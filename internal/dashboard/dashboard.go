package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

type Config struct {
	RefreshInterval time.Duration
}

type row struct {
	Repo        string
	Branch      string
	Head        string
	Detached    bool
	WtDirName   string
	Path        string
	Status      claude.Status
	Unread      bool
	Merged      bool
	StackPrefix string
}

type summary struct {
	Repos     int
	Worktrees int
	Active    int
	Unread    int
}

func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Print(ui.AltScreenOn())
	fmt.Print(ui.HideCursor())
	defer func() {
		fmt.Print(ui.ShowCursor())
		fmt.Print(ui.AltScreenOff())
	}()

	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)

	cols := termWidth()
	interval := cfg.RefreshInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	frame := 0
	prevStatus := map[string]claude.Status{}
	flashUntil := map[string]time.Time{}

	rows, sum := collectData(ctx)
	updateFlashes(rows, prevStatus, flashUntil)
	output := render(rows, sum, cols, frame, flashUntil)
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
			rows, sum = collectData(ctx)
			updateFlashes(rows, prevStatus, flashUntil)
			output = render(rows, sum, cols, frame, flashUntil)
			fmt.Print(ui.CursorHome())
			fmt.Print(output)
			fmt.Print(ui.ClearToEnd())
		}
	}
}

func rowKey(r row) string {
	return r.Repo + "/" + r.WtDirName
}

func updateFlashes(rows []row, prev map[string]claude.Status, flashUntil map[string]time.Time) {
	seen := map[string]struct{}{}
	now := time.Now()
	for _, r := range rows {
		key := rowKey(r)
		seen[key] = struct{}{}
		if p, ok := prev[key]; ok && p == claude.StatusBusy && r.Status == claude.StatusDone {
			flashUntil[key] = now.Add(2 * time.Second)
		}
		prev[key] = r.Status
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

func collectData(ctx context.Context) ([]row, summary) {
	repos, err := config.ListRepos()
	if err != nil {
		return nil, summary{}
	}

	sum := summary{Repos: len(repos)}
	items, err := tmux.BuildPickerItemsWithOptions(ctx, "", tmux.PickerBuildOptions{RefreshGitHubMerged: false})
	if err != nil {
		return nil, sum
	}

	rows := make([]row, 0, len(items))
	for _, item := range items {
		unread := claude.CountUnreadIn(item.RepoName, item.WtDirName, item.Sessions) > 0
		r := row{
			Repo:        item.RepoName,
			Branch:      item.Branch,
			Head:        item.Head,
			Detached:    item.Detached,
			WtDirName:   item.WtDirName,
			Path:        item.WtPath,
			Status:      item.Status,
			Unread:      unread,
			Merged:      item.Merged,
			StackPrefix: item.StackPrefix,
		}
		rows = append(rows, r)
		sum.Worktrees++
		if claude.IsActive(item.Status) {
			sum.Active++
		}
		if unread {
			sum.Unread++
		}
	}

	return rows, sum
}

func render(rows []row, sum summary, width int, frame int, flashUntil map[string]time.Time) string {
	var b strings.Builder
	u := &ui.UI{}

	if len(rows) == 0 {
		title := "willow dashboard"
		stats := fmt.Sprintf("%d repos | %d worktrees | %d active | %d unread", sum.Repos, sum.Worktrees, sum.Active, sum.Unread)
		headerText := title + "  " + stats
		pad := 0
		if width > len(headerText) {
			pad = (width - len(headerText)) / 2
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(u.Bold(headerText))
		b.WriteString("\n\n")
		b.WriteString(u.Dim("  no worktrees yet"))
		b.WriteString("\n\n")
		b.WriteString("  create one with  ")
		b.WriteString(u.Cyan("willow new <branch>"))
		b.WriteString("\n")
		return b.String()
	}

	multiRepo := hasMultipleRepos(rows)
	home, _ := os.UserHomeDir()

	type labels struct {
		status string
		name   string
		path   string
	}
	rowLabels := make([]labels, len(rows))
	statusW := len("STATUS")
	nameW := len("WORKTREE")
	pathW := len("PATH")

	for i, r := range rows {
		statusText := string(r.Status)
		if r.Unread {
			statusText += " \u25cf"
		}
		name := displayName(r, multiRepo)
		namePlain := name
		if r.Merged {
			namePlain += " [merged]"
		}
		path := shortenPathWithHome(r.Path, home)
		rowLabels[i] = labels{status: statusText, name: name, path: path}

		if utf8.RuneCountInString(statusText) > statusW {
			statusW = utf8.RuneCountInString(statusText)
		}
		if utf8.RuneCountInString(namePlain) > nameW {
			nameW = utf8.RuneCountInString(namePlain)
		}
		if utf8.RuneCountInString(path) > pathW {
			pathW = utf8.RuneCountInString(path)
		}
	}

	tableW := 2 + 2 + 1 + statusW + 2 + nameW + 2 + pathW
	title := "willow dashboard"
	stats := fmt.Sprintf("%d repos | %d worktrees | %d active | %d unread", sum.Repos, sum.Worktrees, sum.Active, sum.Unread)
	headerText := title + "  " + stats
	pad := 0
	if tableW > len(headerText) {
		pad = (tableW - len(headerText)) / 2
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(u.Bold(headerText))
	b.WriteString("\n\n")

	headerLine := fmt.Sprintf("  %-2s %-*s  %-*s  %-*s",
		"", statusW, "STATUS", nameW, "WORKTREE", pathW, "PATH")
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
	for i, r := range rows {
		icon := claude.StatusIcon(r.Status)
		if r.Status == claude.StatusBusy {
			icon = u.Cyan(u.SpinnerFrame(frame))
		}

		statusCol := fmt.Sprintf("%-*s", statusW, rowLabels[i].status)
		statusCol = colorStatus(u, r.Status, statusCol)

		nameDisplay := rowLabels[i].name
		namePlain := nameDisplay
		if r.Merged {
			namePlain += " [merged]"
			nameDisplay += " " + u.Dim("[merged]")
		}
		namePadding := nameW - utf8.RuneCountInString(namePlain)
		if namePadding < 0 {
			namePadding = 0
		}
		nameCol := nameDisplay + strings.Repeat(" ", namePadding)

		pathCol := fmt.Sprintf("%-*s", pathW, rowLabels[i].path)
		line := fmt.Sprintf("  %s %s  %s  %s", icon, statusCol, nameCol, u.Dim(pathCol))

		if until, ok := flashUntil[rowKey(r)]; ok && now.Before(until) {
			b.WriteString("\033[48;5;30m")
			b.WriteString(line)
			b.WriteString("\033[0m")
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func displayName(r row, multiRepo bool) string {
	name := r.Branch
	if r.Detached {
		if r.Head == "" {
			name = fmt.Sprintf("%s [detached]", r.WtDirName)
		} else {
			name = fmt.Sprintf("%s [detached %s]", r.WtDirName, worktree.ShortHead(r.Head))
		}
	}
	if multiRepo {
		name = r.Repo + "/" + name
	}
	if r.StackPrefix != "" && !r.Detached {
		name = r.StackPrefix + name
	}
	return name
}

func hasMultipleRepos(rows []row) bool {
	if len(rows) <= 1 {
		return false
	}
	first := rows[0].Repo
	for _, r := range rows[1:] {
		if r.Repo != first {
			return true
		}
	}
	return false
}

func colorStatus(u *ui.UI, status claude.Status, text string) string {
	switch status {
	case claude.StatusBusy:
		return u.Green(text)
	case claude.StatusWait:
		return u.Red(text)
	case claude.StatusDone:
		return u.Cyan(text)
	case claude.StatusIdle:
		return u.Yellow(text)
	default:
		return u.Dim(text)
	}
}

func shortenPathWithHome(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
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
