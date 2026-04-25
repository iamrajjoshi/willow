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
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
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

			bareDir, err := g.BareRepoDir()
			if err == nil && config.IsWillowRepo(bareDir) {
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}
			if bareDir, ok := resolveRepoFromCwd(); ok {
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

	nameW := len("REPO")
	for _, r := range repos {
		if len(r) > nameW {
			nameW = len(r)
		}
	}

	header := fmt.Sprintf("  %-*s  %-9s  %-6s  %s", nameW, "REPO", "WORKTREES", "ACTIVE", "UNREAD")
	u.Info(u.Bold(header))

	for _, r := range repos {
		bareDir, err := config.ResolveRepo(r)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}
		count := 0
		activeCount := 0
		unreadCount := 0
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			count++
			wtDir := filepath.Base(wt.Path)
			ws := claude.ReadStatus(r, wtDir)
			if claude.IsActive(ws.Status) {
				activeCount++
			}
			if claude.IsUnread(r, wtDir) {
				unreadCount++
			}
		}
		line := fmt.Sprintf("  %-*s  %-9d  %-6d  %d", nameW, r, count, activeCount, unreadCount)
		u.Info(line)
	}
	return nil
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
	status claude.Status
	unread bool
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
	branches := make([]string, 0, len(worktrees))
	branchHeads := make(map[string]string, len(worktrees))
	repoDir := ""
	for _, wt := range worktrees {
		if repoDir == "" {
			repoDir = wt.Path
		}
		if wt.Branch != "" && !wt.Detached {
			branches = append(branches, wt.Branch)
			branchHeads[wt.Branch] = wt.Head
		}
	}
	done := trace.Span(ctx, "MergedBranchSet")
	mergedSet := repoGit.MergedBranchSet(baseBranch, branches)
	for branch := range gh.MergedWorktreeSet(repoDir, baseBranch, branchHeads) {
		mergedSet[branch] = true
	}
	done()

	var rows []lsRow
	for _, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		ws := claude.ReadStatus(repoName, wtDir)
		rows = append(rows, lsRow{
			branch: wt.Branch,
			merged: !wt.Detached && mergedSet[wt.Branch],
			status: ws.Status,
			unread: ws.Status == claude.StatusDone && claude.IsUnread(repoName, wtDir),
			wt:     wt,
		})
	}
	rows = sortLSRows(rows, st)

	branchW := len("BRANCH")
	statusW := len("STATUS")
	pathW := len("PATH")
	ageW := len("AGE")
	for _, row := range rows {
		display := row.prefix + row.wt.DisplayName()
		if row.merged {
			display += " [merged]"
		}
		if utf8.RuneCountInString(display) > branchW {
			branchW = utf8.RuneCountInString(display)
		}
		if len(row.wt.Path) > pathW {
			pathW = len(row.wt.Path)
		}
		age := worktreeAge(row.wt.Path)
		if len(age) > ageW {
			ageW = len(age)
		}
	}

	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %*s", branchW, "BRANCH", statusW, "STATUS", pathW, "PATH", ageW, "AGE")
	u.Info(u.Bold(header))

	for _, row := range rows {
		age := worktreeAge(row.wt.Path)
		statusLabel := claude.StatusLabel(row.status)
		if row.unread {
			statusLabel += "\u25CF" // ●
		}
		branchPlain := row.prefix + row.wt.DisplayName()
		branchDisplay := branchPlain
		if row.merged {
			branchPlain += " [merged]"
			branchDisplay += " " + u.Dim("[merged]")
		}
		// Pad based on plain text width to avoid ANSI code miscount
		padding := branchW - utf8.RuneCountInString(branchPlain)
		if padding < 0 {
			padding = 0
		}
		branchCol := branchDisplay + strings.Repeat(" ", padding)
		pathPadded := fmt.Sprintf("%-*s", pathW, row.wt.Path)
		agePadded := fmt.Sprintf("%*s", ageW, age)
		line := fmt.Sprintf("  %s  %-*s  %s  %s", branchCol, statusW, statusLabel, u.Dim(pathPadded), u.Dim(agePadded))
		u.Info(line)
	}
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
		priority: claude.WorktreeUrgencyOrder(claude.StatusOffline, false),
	}
	for _, row := range rows {
		if !row.merged {
			group.merged = false
		}
		priority := claude.WorktreeUrgencyOrder(row.status, row.unread)
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
