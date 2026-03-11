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

func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

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

	// Initial render
	renderTick(cols)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case <-winchCh:
			cols = termWidth()
		case <-ticker.C:
			renderTick(cols)
		}
	}
}

func renderTick(cols int) {
	sessions, sum := collectData()
	output := render(sessions, sum, cols)
	fmt.Print(ui.CursorHome())
	fmt.Print(output)
	fmt.Print(ui.ClearToEnd())
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
					s := session{
						Repo:      repoName,
						Branch:    wt.Branch,
						SessionID: ss.SessionID,
						Status:    ss.Status,
						Tool:      ss.Tool,
						DiffStats: diff,
						Age:       claude.TimeSince(ss.Timestamp),
						Unread:    ss.Status == claude.StatusDone && unread,
					}
					sessions = append(sessions, s)
					if ss.Status == claude.StatusBusy || ss.Status == claude.StatusWait || ss.Status == claude.StatusDone {
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
				}
				sessions = append(sessions, s)
				sum.Agents++
			}
		}
	}

	return sessions, sum
}

func render(sessions []session, sum summary, width int) string {
	var b strings.Builder
	u := &ui.UI{}

	header := fmt.Sprintf("willow dashboard          %d repos | %d agents | %d unread",
		sum.Repos, sum.Agents, sum.Unread)
	b.WriteString(u.Bold(header))
	b.WriteString("\n\n")

	if len(sessions) == 0 {
		b.WriteString(u.Dim("  No active sessions. Claude agents will appear here when running."))
		b.WriteString("\n")
		return b.String()
	}

	// Calculate column widths
	repoW := len("REPO")
	branchW := len("BRANCH")
	diffW := len("DIFF")
	for _, s := range sessions {
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

	// Header row
	headerLine := fmt.Sprintf("  %-8s  %-*s  %-*s  %-*s  %s",
		"STATUS", repoW, "REPO", branchW, "BRANCH", diffW, "DIFF", "AGE")
	b.WriteString(u.Bold(headerLine))
	b.WriteString("\n")

	// Separator
	sepLen := 8 + repoW + branchW + diffW + 20
	if sepLen > width && width > 0 {
		sepLen = width - 2
	}
	b.WriteString("  ")
	b.WriteString(u.Dim(strings.Repeat("\u2500", sepLen)))
	b.WriteString("\n")

	// Rows
	for _, s := range sessions {
		icon := claude.StatusIcon(s.Status)
		label := string(s.Status)
		if s.Unread {
			label += "\u25CF" // ●
		}

		activity := ""
		if s.Status == claude.StatusBusy && s.Tool != "" {
			activity = u.Dim(fmt.Sprintf(" (%s)", s.Tool))
		}

		line := fmt.Sprintf("  %s %-6s%-*s  %-*s  %-*s  %s",
			icon, label+activity,
			repoW, s.Repo,
			branchW, s.Branch,
			diffW, s.DiffStats,
			u.Dim(s.Age))
		b.WriteString(line)
		b.WriteString("\n")
	}

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

	stats := parseShortstat(out)

	diffCacheMu.Lock()
	diffCache[wtPath] = cachedDiff{stats: stats, at: time.Now()}
	diffCacheMu.Unlock()

	return stats
}

func parseShortstat(out string) string {
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
