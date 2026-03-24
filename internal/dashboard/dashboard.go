package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

type Config struct {
	RefreshInterval time.Duration
}

type session struct {
	Repo      string
	Branch    string
	SessionID string
	Status    claude.Status
	Tool      string
	DiffStats string
	Age       string
	Unread    bool
	WtDirName string
}

type summary struct {
	Repos   int
	Agents  int
	Unread  int
}

type cachedDiff struct {
	stats string
	at    time.Time
}

var (
	diffCache   = map[string]cachedDiff{}
	diffCacheMu sync.Mutex
	diffCacheTTL = 10 * time.Second
)

func readKey(tty *os.File) chan byte {
	ch := make(chan byte, 1)
	go func() {
		buf := make([]byte, 3)
		for {
			n, err := tty.Read(buf)
			if err != nil {
				return
			}
			if n == 1 {
				ch <- buf[0]
			} else if n == 3 && buf[0] == 27 && buf[1] == 91 {
				// Arrow keys: ESC [ A/B
				ch <- buf[2]
			}
		}
	}()
	return ch
}

func setRawMode(tty *os.File) {
	cmd := exec.Command("stty", "raw", "-echo")
	cmd.Stdin = tty
	cmd.Run()
}

func restoreMode(tty *os.File) {
	cmd := exec.Command("stty", "-raw", "echo")
	cmd.Stdin = tty
	cmd.Run()
}

func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Open /dev/tty for keyboard input
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()

	setRawMode(tty)
	defer restoreMode(tty)

	// Enter alternate screen, hide cursor
	fmt.Print(ui.AltScreenOn())
	fmt.Print(ui.HideCursor())

	defer func() {
		fmt.Print(ui.ShowCursor())
		fmt.Print(ui.AltScreenOff())
	}()

	// Handle SIGWINCH for terminal resize
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)

	cols := termWidth()
	ticker := time.NewTicker(cfg.RefreshInterval)
	defer ticker.Stop()

	selectedIdx := 0
	keyCh := readKey(tty)

	// Initial render
	sessions, sum := collectData()
	output := render(sessions, sum, cols, selectedIdx)
	fmt.Print(ui.CursorHome())
	fmt.Print(output)
	fmt.Print(ui.ClearToEnd())

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case <-winchCh:
			cols = termWidth()
		case <-ticker.C:
			sessions, sum = collectData()
			if selectedIdx >= len(sessions) && len(sessions) > 0 {
				selectedIdx = len(sessions) - 1
			}
			output = render(sessions, sum, cols, selectedIdx)
			fmt.Print(ui.CursorHome())
			fmt.Print(output)
			fmt.Print(ui.ClearToEnd())
		case key := <-keyCh:
			switch key {
			case 'j', 'B': // down: j or arrow-down (ESC[B)
				if selectedIdx < len(sessions)-1 {
					selectedIdx++
				}
			case 'k', 'A': // up: k or arrow-up (ESC[A)
				if selectedIdx > 0 {
					selectedIdx--
				}
			case 'q', 3: // q or Ctrl+C
				return nil
			case 'r': // refresh
				sessions, sum = collectData()
				if selectedIdx >= len(sessions) && len(sessions) > 0 {
					selectedIdx = len(sessions) - 1
				}
			case 13: // Enter — switch to tmux session
				if selectedIdx < len(sessions) {
					s := sessions[selectedIdx]
					sessionName := tmux.SessionNameForWorktree(s.Repo, s.WtDirName)
					restoreMode(tty)
					fmt.Print(ui.ShowCursor())
					fmt.Print(ui.AltScreenOff())
					tmux.SwitchClient(sessionName)
					return nil
				}
			}
			output = render(sessions, sum, cols, selectedIdx)
			fmt.Print(ui.CursorHome())
			fmt.Print(output)
			fmt.Print(ui.ClearToEnd())
		}
	}
}

func collectData() ([]session, summary) {
	repos, err := config.ListRepos()
	if err != nil {
		return nil, summary{}
	}

	var sessions []session
	sum := summary{Repos: len(repos)}

	for _, repoName := range repos {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}

		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}

		cfg := config.Load(bareDir)

		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			wtDir := filepath.Base(wt.Path)
			allSessions := claude.ReadAllSessions(repoName, wtDir)
			unread := claude.IsUnread(repoName, wtDir)

			if unread {
				sum.Unread++
			}

			baseBranch := cfg.BaseBranch
			if baseBranch == "" {
				baseBranch = "main"
			}

			diff := getDiffStats(wt.Path, baseBranch)

			if len(allSessions) > 0 {
				for _, ss := range allSessions {
					effective := claude.EffectiveStatus(ss.Status, ss.Timestamp)
					s := session{
						Repo:      repoName,
						Branch:    wt.Branch,
						SessionID: ss.SessionID,
						Status:    effective,
						Tool:      ss.Tool,
						DiffStats: diff,
						Age:       claude.TimeSince(ss.Timestamp),
						Unread:    effective == claude.StatusDone && unread,
						WtDirName: wtDir,
					}
					sessions = append(sessions, s)
					if effective == claude.StatusBusy || effective == claude.StatusWait || effective == claude.StatusDone {
						sum.Agents++
					}
				}
			} else {
				ws := claude.ReadStatus(repoName, wtDir)
				if ws.Status == claude.StatusOffline || ws.Status == claude.StatusIdle {
					continue
				}
				s := session{
					Repo:      repoName,
					Branch:    wt.Branch,
					Status:    ws.Status,
					DiffStats: diff,
					Age:       claude.TimeSince(ws.Timestamp),
					Unread:    ws.Status == claude.StatusDone && unread,
					WtDirName: wtDir,
				}
				sessions = append(sessions, s)
				sum.Agents++
			}
		}
	}

	return sessions, sum
}

func render(sessions []session, sum summary, width int, selectedIdx int) string {
	var b strings.Builder
	u := &ui.UI{}

	if len(sessions) == 0 {
		title := "willow dashboard"
		stats := fmt.Sprintf("%d repos | %d agents | %d unread", sum.Repos, sum.Agents, sum.Unread)
		headerText := title + "  " + stats
		pad := 0
		if width > len(headerText) {
			pad = (width - len(headerText)) / 2
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(u.Bold(headerText))
		b.WriteString("\n\n")
		b.WriteString(u.Dim("  No active sessions. Claude agents will appear here when running."))
		b.WriteString("\n")
		return b.String()
	}

	// Build plain-text status labels to compute column widths
	type rowLabel struct {
		text string
	}
	labels := make([]rowLabel, len(sessions))
	statusW := len("STATUS")
	repoW := len("REPO")
	branchW := len("BRANCH")
	diffW := len("DIFF")
	for i, s := range sessions {
		text := string(s.Status)
		if s.Unread {
			text += "\u25CF"
		}
		if s.Status == claude.StatusBusy && s.Tool != "" {
			text += " (" + s.Tool + ")"
		}
		labels[i] = rowLabel{text: text}
		if len(text) > statusW {
			statusW = len(text)
		}
		if len(s.Repo) > repoW {
			repoW = len(s.Repo)
		}
		if len(s.Branch) > branchW {
			branchW = len(s.Branch)
		}
		if len(s.DiffStats) > diffW {
			diffW = len(s.DiffStats)
		}
	}

	// Table width: 2 (indent) + 2 (icon) + 1 (space) + statusW + 2 + repoW + 2 + branchW + 2 + diffW + 2 + 8 (AGE)
	tableW := 2 + 2 + 1 + statusW + 2 + repoW + 2 + branchW + 2 + diffW + 2 + 8

	// Title centered over the table
	title := "willow dashboard"
	stats := fmt.Sprintf("%d repos | %d agents | %d unread", sum.Repos, sum.Agents, sum.Unread)
	headerText := title + "  " + stats
	pad := 0
	if tableW > len(headerText) {
		pad = (tableW - len(headerText)) / 2
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(u.Bold(headerText))
	b.WriteString("\n\n")

	// Header row — icon placeholder is 2 spaces to match emoji visual width (2 columns)
	headerLine := fmt.Sprintf("  %-2s %-*s  %-*s  %-*s  %-*s  %s",
		"", statusW, "STATUS", repoW, "REPO", branchW, "BRANCH", diffW, "DIFF", "AGE")
	b.WriteString(u.Bold(headerLine))
	b.WriteString("\n")

	// Separator
	sepLen := tableW - 2
	if sepLen > width-2 && width > 0 {
		sepLen = width - 2
	}
	b.WriteString("  ")
	b.WriteString(u.Dim(strings.Repeat("\u2500", sepLen)))
	b.WriteString("\n")

	// Rows — status is padded as plain text, no ANSI inside padded fields
	for i, s := range sessions {
		icon := claude.StatusIcon(s.Status)
		line := fmt.Sprintf("  %s %-*s  %-*s  %-*s  %-*s  %s",
			icon, statusW, labels[i].text,
			repoW, s.Repo,
			branchW, s.Branch,
			diffW, s.DiffStats,
			u.Dim(s.Age))
		if i == selectedIdx {
			b.WriteString("\033[7m") // inverse video
			b.WriteString(line)
			b.WriteString("\033[0m")
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(u.Dim("  j/k: navigate | Enter: switch | r: refresh | q: quit"))
	b.WriteString("\n")

	return b.String()
}

func getDiffStats(wtPath, baseBranch string) string {
	diffCacheMu.Lock()
	if cached, ok := diffCache[wtPath]; ok && time.Since(cached.at) < diffCacheTTL {
		diffCacheMu.Unlock()
		return cached.stats
	}
	diffCacheMu.Unlock()

	g := &git.Git{Dir: wtPath}
	out, err := g.Run("diff", "--shortstat", fmt.Sprintf("origin/%s...HEAD", baseBranch))
	if err != nil {
		return "--"
	}

	stats := ParseShortstat(out)

	diffCacheMu.Lock()
	diffCache[wtPath] = cachedDiff{stats: stats, at: time.Now()}
	diffCacheMu.Unlock()

	return stats
}

// ParseShortstat converts git diff --shortstat output into a compact summary.
func ParseShortstat(out string) string {
	if out == "" {
		return "--"
	}
	// "3 files changed, 42 insertions(+), 12 deletions(-)"
	parts := strings.Split(out, ",")
	files := ""
	ins := ""
	del := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		fields := strings.Fields(p)
		if len(fields) >= 2 {
			switch {
			case strings.Contains(p, "file"):
				files = fields[0] + "f"
			case strings.Contains(p, "insertion"):
				ins = "+" + fields[0]
			case strings.Contains(p, "deletion"):
				del = "-" + fields[0]
			}
		}
	}
	result := files
	if ins != "" {
		result += " " + ins
	}
	if del != "" {
		result += " " + del
	}
	if result == "" {
		return "--"
	}
	return result
}

func termWidth() int {
	// Use /dev/tty so stty works even when stdin is piped
	cmd := exec.Command("stty", "size")
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return 120
	}
	defer tty.Close()
	cmd.Stdin = tty
	out, err := cmd.Output()
	if err != nil {
		return 120
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 2 {
		if w, err := strconv.Atoi(fields[1]); err == nil {
			return w
		}
	}
	return 120
}
