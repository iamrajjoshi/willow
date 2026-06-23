package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/parallel"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/termfmt"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func lsCmd() *cli.Command {
	return &cli.Command{
		Name:    "ls",
		Aliases: []string{"l"},
		Usage:   "List worktrees (or repos when outside a willow repo)",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "repo",
				UsageText: "[repo]",
			},
		},
		ShellComplete: completeRepos,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
			&cli.BoolFlag{
				Name:  "path-only",
				Usage: "Print only worktree paths",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.ls")()
			flags := parseFlags(cmd)
			g := flags.NewGit()

			if repoArg := cmd.StringArg("repo"); repoArg != "" {
				bareDir, err := config.ResolveRepo(repoArg)
				if err != nil {
					return err
				}
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			if bareDir, ok := resolveRepoFromCwd(); ok {
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}
			if !g.Verbose {
				if bareDir, isWillow, foundGit := resolveRepoFromGitMetadataCwd(); isWillow {
					return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
				} else if foundGit {
					return printRepoList(flags)
				}
			}

			bareDir, err := g.BareRepoDir()
			if err == nil && config.IsWillowRepo(bareDir) {
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			return printRepoList(flags)
		},
	}
}

func listWorktrees(ctx context.Context, flags Flags, cmd *cli.Command, repoGit *git.Git) error {
	done := trace.Span(ctx, "worktree.List")
	worktrees, err := worktree.List(repoGit)
	done()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	filtered := filterBareWorktrees(worktrees)

	if cmd.Bool("path-only") {
		for _, wt := range filtered {
			fmt.Println(wt.Path)
		}
		return nil
	}

	if cmd.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	repoName := repoNameFromDir(repoGit.Dir)
	printTable(ctx, flags, filtered, repoName, repoGit)
	return nil
}

type repoListRow struct {
	repo        string
	count       int
	activeCount int
	unreadCount int
	ok          bool
}

func printRepoList(flags Flags) error {
	u := flags.NewUI()
	repos, err := config.ListRepos()
	if err != nil {
		return fmt.Errorf("failed to list repos: %w", err)
	}

	if len(repos) == 0 {
		u.Info("No willow-managed repos. Use 'ww clone <url>' to get started.")
		return nil
	}

	rows := parallel.Map(repos, func(_ int, r string) repoListRow {
		bareDir, err := config.ResolveRepo(r)
		if err != nil {
			return repoListRow{}
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return repoListRow{}
		}
		row := repoListRow{repo: r, ok: true}
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			row.count++
			wtDir := filepath.Base(wt.Path)
			sessions := agent.ReadAllSessions(r, wtDir)
			ws := agent.AggregateStatus(sessions)
			if agent.IsActive(ws.Status) {
				row.activeCount++
			}
			if agent.CountUnreadIn(r, wtDir, sessions) > 0 {
				row.unreadCount++
			}
		}
		return row
	})

	for _, line := range formatRepoListRows(u, rows, termfmt.TerminalWidth()) {
		u.Info(line)
	}
	return nil
}

func formatRepoListRows(u *ui.UI, rows []repoListRow, width int) []string {
	nameW := len("REPO")
	worktreesW := len("WORKTREES")
	activeW := len("ACTIVE")
	unreadW := len("UNREAD")
	for _, row := range rows {
		if !row.ok {
			continue
		}
		nameW = max(nameW, termfmt.VisibleWidth(row.repo))
		worktreesW = max(worktreesW, termfmt.VisibleWidth(fmt.Sprintf("%d", row.count)))
		activeW = max(activeW, termfmt.VisibleWidth(fmt.Sprintf("%d", row.activeCount)))
		unreadW = max(unreadW, termfmt.VisibleWidth(fmt.Sprintf("%d", row.unreadCount)))
	}

	termWidth := termfmt.Width(width)
	fixed := 2 + 2 + worktreesW + 2 + activeW + 2 + unreadW
	if available := termWidth - fixed; available < nameW {
		nameW = max(1, available)
	}

	lines := []string{
		u.Bold(fmt.Sprintf("  %s  %s  %s  %s",
			termfmt.FitRight("REPO", nameW),
			termfmt.FitRight("WORKTREES", worktreesW),
			termfmt.FitRight("ACTIVE", activeW),
			termfmt.FitRight("UNREAD", unreadW),
		)),
	}
	for _, row := range rows {
		if !row.ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s  %s  %s  %s",
			termfmt.FitRight(row.repo, nameW),
			termfmt.FitRight(fmt.Sprintf("%d", row.count), worktreesW),
			termfmt.FitRight(fmt.Sprintf("%d", row.activeCount), activeW),
			termfmt.FitRight(fmt.Sprintf("%d", row.unreadCount), unreadW),
		))
	}
	return lines
}

func completeRepos(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	repos, err := config.ListRepos()
	if err != nil {
		return
	}
	w := cmd.Root().Writer
	for _, r := range repos {
		fmt.Fprintln(w, r)
	}
}

type lsRow struct {
	branch string
	prefix string // tree-drawing prefix
	merged bool
	status agent.Status
	unread bool
	age    string
	wt     worktree.Worktree
}

type lsRowGroup struct {
	rows     []lsRow
	merged   bool
	priority int
}

func printTable(ctx context.Context, flags Flags, worktrees []worktree.Worktree, repoName string, repoGit *git.Git) {
	u := flags.NewUI()

	if len(worktrees) == 0 {
		u.Info(u.Dim("  no worktrees yet"))
		u.Info("")
		u.Info("  create one with  " + u.Cyan("willow new <branch>"))
		return
	}

	cfg := config.Load(repoGit.Dir)
	baseBranch := repoGit.ResolveBaseBranch(cfg.BaseBranch)

	st := stack.Load(repoGit.Dir)
	branchHeads := make(map[string]string, len(worktrees))
	branchBases := make(map[string]string, len(worktrees))
	repoDir := ""
	for _, wt := range worktrees {
		if repoDir == "" {
			repoDir = wt.Path
		}
		if wt.Branch != "" && !wt.Detached {
			branchHeads[wt.Branch] = wt.Head
			if parent := st.Parent(wt.Branch); parent != "" {
				branchBases[wt.Branch] = parent
			}
		}
	}
	done := trace.Span(ctx, "gh.CachedMergedWorktreeSet")
	mergedSet := gh.CachedMergedWorktreeSet(repoDir, baseBranch, branchHeads, branchBases)
	done()

	var rows []lsRow
	for _, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		sessions := agent.ReadAllSessions(repoName, wtDir)
		ws := agent.AggregateStatus(sessions)
		rows = append(rows, lsRow{
			branch: wt.Branch,
			merged: !wt.Detached && mergedSet[wt.Branch],
			status: ws.Status,
			unread: ws.Status == agent.StatusDone && agent.CountUnreadIn(repoName, wtDir, sessions) > 0,
			age:    worktreeAge(wt.Path),
			wt:     wt,
		})
	}
	rows = sortLSRows(rows, st)

	for _, line := range formatLSTableRows(u, rows, termfmt.TerminalWidth()) {
		u.Info(line)
	}
}

type lsDisplayRow struct {
	branchPlain   string
	branchDisplay string
	status        string
	path          string
	age           string
}

func formatLSTableRows(u *ui.UI, rows []lsRow, width int) []string {
	home, _ := os.UserHomeDir()
	displayRows := make([]lsDisplayRow, 0, len(rows))
	branchW := len("BRANCH")
	statusW := len("STATUS")
	pathW := len("PATH")
	ageW := len("AGE")
	for _, row := range rows {
		statusLabel := agent.StatusLabel(row.status)
		if row.unread {
			statusLabel += "\u25CF" // ●
		}
		branchPlain := row.prefix + row.wt.DisplayName()
		branchDisplay := branchPlain
		if row.merged {
			branchPlain += " [merged]"
			branchDisplay += " " + u.Dim("[merged]")
		}
		path := termfmt.ShortenHome(row.wt.Path, home)
		displayRows = append(displayRows, lsDisplayRow{
			branchPlain:   branchPlain,
			branchDisplay: branchDisplay,
			status:        statusLabel,
			path:          path,
			age:           row.age,
		})
		branchW = max(branchW, termfmt.VisibleWidth(branchPlain))
		statusW = max(statusW, termfmt.VisibleWidth(statusLabel))
		pathW = max(pathW, termfmt.VisibleWidth(path))
		ageW = max(ageW, termfmt.VisibleWidth(row.age))
	}

	branchW, pathW = fitNameAndPathWidths(termfmt.Width(width), branchW, pathW, statusW, ageW)
	lines := []string{
		u.Bold(fmt.Sprintf("  %s  %s  %s  %s",
			termfmt.FitRight("BRANCH", branchW),
			termfmt.FitRight("STATUS", statusW),
			termfmt.FitRight("PATH", pathW),
			termfmt.FitLeft("AGE", ageW),
		)),
	}
	for _, row := range displayRows {
		lines = append(lines, fmt.Sprintf("  %s  %s  %s  %s",
			fitDisplayRight(row.branchDisplay, row.branchPlain, branchW),
			termfmt.FitRight(row.status, statusW),
			u.Dim(termfmt.FitRight(row.path, pathW)),
			u.Dim(termfmt.FitLeft(row.age, ageW)),
		))
	}
	return lines
}

func fitNameAndPathWidths(width, nameW, pathW, statusW, tailW int) (int, int) {
	minPathW := len("PATH")
	fixedWithoutNamePath := 2 + 2 + statusW + 2 + 2 + tailW
	available := width - fixedWithoutNamePath
	if available <= 2 {
		return 1, 1
	}
	if nameW+2+pathW <= available {
		return nameW, pathW
	}
	if availablePath := available - nameW - 2; availablePath >= minPathW {
		return nameW, min(pathW, availablePath)
	}
	pathW = minPathW
	nameAvailable := available - pathW - 2
	if nameAvailable < 1 {
		nameAvailable = 1
	}
	return min(nameW, nameAvailable), pathW
}

func fitDisplayRight(display, plain string, width int) string {
	if termfmt.VisibleWidth(plain) > width {
		return termfmt.FitRight(plain, width)
	}
	return termfmt.PadRight(display, width)
}

func sortLSRows(rows []lsRow, st *stack.Stack) []lsRow {
	rowMap := make(map[string]*lsRow, len(rows))
	branchSet := make(map[string]bool, len(rows))
	for i := range rows {
		if rows[i].wt.Detached {
			continue
		}
		rowMap[rows[i].branch] = &rows[i]
		branchSet[rows[i].branch] = true
	}

	groupedBranches := make(map[string]bool)
	var groups []lsRowGroup

	if st != nil && !st.IsEmpty() {
		treeLines := st.TreeLines(branchSet)
		prefixByBranch := make(map[string]string, len(treeLines))
		for _, tl := range treeLines {
			prefixByBranch[tl.Branch] = tl.Prefix
		}

		for _, root := range st.Roots() {
			var groupRows []lsRow
			for _, branch := range st.SubtreeSort(root) {
				row, ok := rowMap[branch]
				if !ok {
					continue
				}
				row.prefix = prefixByBranch[branch]
				groupedBranches[branch] = true
				groupRows = append(groupRows, *row)
			}
			if len(groupRows) > 0 {
				groups = append(groups, newLSRowGroup(groupRows))
			}
		}
	}

	for _, row := range rows {
		if !groupedBranches[row.branch] {
			groups = append(groups, newLSRowGroup([]lsRow{row}))
		}
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].merged != groups[j].merged {
			return !groups[i].merged
		}
		return groups[i].priority < groups[j].priority
	})

	var sorted []lsRow
	for _, group := range groups {
		sorted = append(sorted, group.rows...)
	}
	return sorted
}

func newLSRowGroup(rows []lsRow) lsRowGroup {
	group := lsRowGroup{
		rows:     rows,
		merged:   true,
		priority: agent.WorktreeUrgencyOrder(agent.StatusOffline, false),
	}
	for _, row := range rows {
		if !row.merged {
			group.merged = false
		}
		priority := agent.WorktreeUrgencyOrder(row.status, row.unread)
		if priority < group.priority {
			group.priority = priority
		}
	}
	return group
}

func worktreeAge(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "?"
	}
	return formatAge(time.Since(info.ModTime()))
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}
