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
	ShowTimeline    bool
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
	Timeline  string
}

type summary struct {
	Repos  int
	Agents int
	Unread int
}

type cachedDiff struct {
	stats string
	at    time.Time
}

var (
	diffCache    = map[string]cachedDiff{}
	diffCacheMu  sync.Mutex
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
	// Use -icanon instead of raw so output post-processing (ONLCR) stays enabled.
	// stty raw also sets -opost which turns \n into bare LF, breaking ANSI cursor
	// positioning that relies on \n returning the cursor to column 0.
	cmd := exec.Command("stty", "-echo", "-icanon", "min", "1", "time", "0")
	cmd.Stdin = tty
	cmd.Run()
}

func restoreMode(tty *os.File) {
	cmd := exec.Command("stty", "echo", "icanon")
	cmd.Stdin = tty
	cmd.Run()
}

func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()

	setRawMode(tty)
	defer restoreMode(tty)

	fmt.Print(ui.AltScreenOn())
	fmt.Print(ui.HideCursor())

	defer func() {
		fmt.Print(ui.ShowCursor())
		fmt.Print(ui.AltScreenOff())
	}()

	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)

	cols := termWidth()
	ticker := time.NewTicker(cfg.RefreshInterval)
	defer ticker.Stop()

	selectedIdx := 0
	showTimeline := cfg.ShowTimeline
	keyCh := readKey(tty)

	frame := 0
	prevStatus := map[string]claude.Status{}
	flashUntil := map[string]time.Time{}

	sessions, sum := collectData()
	updateFlashes(sessions, prevStatus, flashUntil)
	output := render(sessions, sum, cols, selectedIdx, showTimeline, frame, flashUntil)
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
			frame++
			sessions, sum = collectData()
			updateFlashes(sessions, prevStatus, flashUntil)
			if selectedIdx >= len(sessions) && len(sessions) > 0 {
				selectedIdx = len(sessions) - 1
			}
			output = render(sessions, sum, cols, selectedIdx, showTimeline, frame, flashUntil)
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
				updateFlashes(sessions, prevStatus, flashUntil)
				if selectedIdx >= len(sessions) && len(sessions) > 0 {
					selectedIdx = len(sessions) - 1
				}
			case 't': // toggle timeline
				showTimeline = !showTimeline
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
			output = render(sessions, sum, cols, selectedIdx, showTimeline, frame, flashUntil)
			fmt.Print(ui.CursorHome())
			fmt.Print(output)
			fmt.Print(ui.ClearToEnd())
		}
	}
}

// sessionKey uniquely identifies a dashboard row across refreshes. Sessions
// without an ID (bare worktree status) key by repo+worktree so they still
// participate in transition tracking.
func sessionKey(s session) string {
	if s.SessionID != "" {
		return s.Repo + "/" + s.WtDirName + "/" + s.SessionID
	}
	return s.Repo + "/" + s.WtDirName
}

// updateFlashes records a 2-second flash whenever a session transitions from
// BUSY → DONE, so the row visibly highlights on the next render.
func updateFlashes(sessions []session, prev map[string]claude.Status, flashUntil map[string]time.Time) {
	seen := map[string]struct{}{}
	now := time.Now()
	for _, s := range sessions {
		key := sessionKey(s)
		seen[key] = struct{}{}
		if p, ok := prev[key]; ok && p == claude.StatusBusy && s.Status == claude.StatusDone {
			flashUntil[key] = now.Add(2 * time.Second)
		}
		prev[key] = s.Status
	}
	for key := range prev {
		if _, ok := seen[key]; !ok {
			delete(prev, key)
			delete(flashUntil, key)
		}
	}
	for key, until := range flashUntil {
		if now.After(until) {
			delete(flashUntil, key)
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
		baseBranch := repoGit.ResolveBaseBranch(cfg.BaseBranch)

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

			diff := getDiffStats(wt.Path, baseBranch)

			timelineSince := time.Now().Add(-60 * time.Minute)

			if len(allSessions) > 0 {
				for _, ss := range allSessions {
					effective := claude.EffectiveStatus(ss.Status, ss.Timestamp)
					if effective == claude.StatusIdle || effective == claude.StatusOffline {
						continue
					}
					timeline, _ := claude.ReadTimeline(repoName, wtDir, ss.SessionID, timelineSince)
					s := session{
						Repo:      repoName,
						Branch:    wt.DisplayName(),
						SessionID: ss.SessionID,
						Status:    effective,
						Tool:      ss.Tool,
						DiffStats: diff,
						Age:       claude.TimeSince(ss.Timestamp),
						Unread:    effective == claude.StatusDone && unread,
						WtDirName: wtDir,
						Timeline:  claude.Sparkline(timeline, 30, 60*time.Minute),
					}
					sessions = append(sessions, s)
					if claude.IsActive(effective) {
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
					Branch:    wt.DisplayName(),
					Status:    ws.Status,
					DiffStats: diff,
					Age:       claude.TimeSince(ws.Timestamp),
					Unread:    ws.Status == claude.StatusDone && unread,
					WtDirName: wtDir,
					Timeline:  strings.Repeat("\033[2m\u00b7\033[0m", 30),
				}
				sessions = append(sessions, s)
				sum.Agents++
			}
		}
	}

	return sessions, sum
}

func render(sessions []session, sum summary, width int, selectedIdx int, showTimeline bool, frame int, flashUntil map[string]time.Time) string {
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
		b.WriteString(u.Dim("  no active sessions yet"))
		b.WriteString("\n\n")
		b.WriteString("  start one with  ")
		b.WriteString(u.Cyan("willow new <branch>"))
		b.WriteString("\n")
		return b.String()
	}

	type rowLabel struct {
		status  string
		session string
	}
	labels := make([]rowLabel, len(sessions))
	statusW := len("STATUS")
	repoW := len("REPO")
	branchW := len("BRANCH")
	sessionW := len("SESSION")
	diffW := len("DIFF")
	for i, s := range sessions {
		statusText := string(s.Status)
		if s.Unread {
			statusText += "\u25CF"
		}
		if s.Status == claude.StatusBusy && s.Tool != "" {
			statusText += " (" + s.Tool + ")"
		}
		sessionText := claude.ShortSessionID(s.SessionID)
		labels[i] = rowLabel{status: statusText, session: sessionText}
		if len(statusText) > statusW {
			statusW = len(statusText)
		}
		if len(s.Repo) > repoW {
			repoW = len(s.Repo)
		}
		if len(s.Branch) > branchW {
			branchW = len(s.Branch)
		}
		if len(sessionText) > sessionW {
			sessionW = len(sessionText)
		}
		if len(s.DiffStats) > diffW {
			diffW = len(s.DiffStats)
		}
	}

	timelineW := 30
	tableW := 2 + 2 + 1 + statusW + 2 + repoW + 2 + branchW + 2 + sessionW + 2 + diffW + 2 + 8
	if showTimeline {
		tableW += 2 + timelineW // 2 for gap + column width
	}

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

	headerLine := fmt.Sprintf("  %-2s %-*s  %-*s  %-*s  %-*s  %-*s",
		"", statusW, "STATUS", repoW, "REPO", branchW, "BRANCH", sessionW, "SESSION", diffW, "DIFF")
	headerLine += "  AGE"
	if showTimeline {
		headerLine += fmt.Sprintf("  %-*s", timelineW, "TIMELINE")
	}
	b.WriteString(u.Bold(headerLine))
	b.WriteString("\n")

	sepLen := tableW - 2
	if sepLen > width-2 && width > 0 {
		sepLen = width - 2
	}
	b.WriteString("  ")
	b.WriteString(u.Dim(strings.Repeat("\u2500", sepLen)))
	b.WriteString("\n")

	now := time.Now()
	for i, s := range sessions {
		icon := claude.StatusIcon(s.Status)
		if s.Status == claude.StatusBusy {
			icon = u.Cyan(u.SpinnerFrame(frame))
		}
		line := fmt.Sprintf("  %s %-*s  %-*s  %-*s  %-*s  %-*s",
			icon, statusW, labels[i].status,
			repoW, s.Repo,
			branchW, s.Branch,
			sessionW, labels[i].session,
			diffW, s.DiffStats)
		line += "  " + u.Dim(s.Age)
		if showTimeline {
			line += "  " + s.Timeline
		}

		flashing := false
		if until, ok := flashUntil[sessionKey(s)]; ok && now.Before(until) {
			flashing = true
		}

		switch {
		case i == selectedIdx:
			b.WriteString("\033[7m") // inverse video
			b.WriteString(line)
			b.WriteString("\033[0m")
		case flashing:
			b.WriteString("\033[48;5;30m") // teal background
			b.WriteString(line)
			b.WriteString("\033[0m")
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(u.Dim("  j/k: navigate | Enter: switch | t: timeline | r: refresh | q: quit"))
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

	stats := git.ParseShortstat(out)

	diffCacheMu.Lock()
	diffCache[wtPath] = cachedDiff{stats: stats, at: time.Now()}
	diffCacheMu.Unlock()

	return stats
}

func termWidth() int {
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
