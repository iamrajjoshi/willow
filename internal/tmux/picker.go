package tmux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

const (
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[0;33m"
	colorBlue   = "\033[0;34m"
	colorRed    = "\033[0;31m"
	colorDim    = "\033[2m"
	colorReset  = "\033[0m"
)

type PickerItem struct {
	RepoName    string
	Branch      string
	Head        string
	Detached    bool
	WtDirName   string
	WtPath      string
	Status      claude.Status
	Unread      bool
	HasSession  bool
	Sessions    []*claude.SessionStatus
	Merged      bool
	StackPrefix string // tree-drawing prefix for stacked branches (e.g., "├─ ")
}

type pickerGroup struct {
	items    []PickerItem
	merged   bool
	priority int
}

type PickerBuildOptions struct {
	RefreshGitHubMerged bool
}

func BuildPickerItems(ctx context.Context, repoFilter string) ([]PickerItem, error) {
	return BuildPickerItemsWithOptions(ctx, repoFilter, PickerBuildOptions{RefreshGitHubMerged: true})
}

func BuildPickerItemsWithOptions(ctx context.Context, repoFilter string, opts PickerBuildOptions) ([]PickerItem, error) {
	defer trace.Span(ctx, "BuildPickerItems")()

	var repoNames []string
	if repoFilter != "" {
		repoNames = []string{repoFilter}
	} else {
		done := trace.Span(ctx, "config.ListRepos")
		var err error
		repoNames, err = config.ListRepos()
		done()
		if err != nil {
			return nil, fmt.Errorf("failed to list repos: %w", err)
		}
	}

	sessionSet := ListSessions()

	var items []PickerItem
	for _, repoName := range repoNames {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bareDir}
		done := trace.Span(ctx, "worktree.List/"+repoName)
		wts, err := worktree.List(repoGit)
		done()
		if err != nil {
			continue
		}

		cfg := config.Load(bareDir)
		baseBranch := repoGit.ResolveBaseBranch(cfg.BaseBranch)
		branches := make([]string, 0, len(wts))
		branchHeads := make(map[string]string, len(wts))
		repoDir := ""
		for _, wt := range wts {
			if !wt.IsBare && !wt.Detached && wt.Branch != "" {
				if repoDir == "" {
					repoDir = wt.Path
				}
				branches = append(branches, wt.Branch)
				branchHeads[wt.Branch] = wt.Head
			}
		}
		done = trace.Span(ctx, "git.MergedBranchSet/"+repoName)
		mergedSet := repoGit.MergedBranchSet(baseBranch, branches)
		done()

		done = trace.Span(ctx, "gh.MergedWorktreeSet/"+repoName)
		var githubMerged map[string]bool
		if opts.RefreshGitHubMerged {
			githubMerged = gh.MergedWorktreeSet(repoDir, baseBranch, branchHeads)
		} else {
			githubMerged = gh.CachedMergedWorktreeSet(repoDir, baseBranch, branchHeads)
		}
		for branch := range githubMerged {
			mergedSet[branch] = true
		}
		done()

		done = trace.Span(ctx, "per-wt-loop/"+repoName)
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			wtDir := filepath.Base(wt.Path)
			sessions := claude.ReadAllSessions(repoName, wtDir)
			ws := claude.AggregateStatus(sessions)
			sessName := SessionNameForWorktree(repoName, wtDir)
			items = append(items, PickerItem{
				RepoName:   repoName,
				Branch:     wt.Branch,
				Head:       wt.Head,
				Detached:   wt.Detached,
				WtDirName:  wtDir,
				WtPath:     wt.Path,
				Status:     ws.Status,
				Unread:     ws.Status == claude.StatusDone && claude.CountUnreadIn(repoName, wtDir, sessions) > 0,
				HasSession: sessionSet[sessName],
				Sessions:   sessions,
				Merged:     !wt.Detached && mergedSet[wt.Branch],
			})
		}
		done()
	}

	done := trace.Span(ctx, "sortPickerItems")
	items = sortPickerItems(items, repoNames, defaultPickerStackLoader)
	done()

	return items, nil
}

type pickerStackLoader func(repoName string) *stack.Stack

func defaultPickerStackLoader(repoName string) *stack.Stack {
	bareDir, err := config.ResolveRepo(repoName)
	if err != nil {
		return nil
	}
	return stack.Load(bareDir)
}

func sortPickerItems(items []PickerItem, repoNames []string, loadStack pickerStackLoader) []PickerItem {
	groups := buildPickerGroups(items, repoNames, loadStack)
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].merged != groups[j].merged {
			return !groups[i].merged
		}
		return groups[i].priority < groups[j].priority
	})

	var result []PickerItem
	for _, group := range groups {
		result = append(result, group.items...)
	}
	return result
}

func buildPickerGroups(items []PickerItem, repoNames []string, loadStack pickerStackLoader) []pickerGroup {
	if loadStack == nil {
		loadStack = func(string) *stack.Stack { return nil }
	}

	repoBranchSets := make(map[string]map[string]bool)
	for _, item := range items {
		if item.Detached {
			continue
		}
		branchSet := repoBranchSets[item.RepoName]
		if branchSet == nil {
			branchSet = make(map[string]bool)
			repoBranchSets[item.RepoName] = branchSet
		}
		branchSet[item.Branch] = true
	}

	itemMap := make(map[string]*PickerItem)
	for i := range items {
		if items[i].Detached {
			continue
		}
		key := pickerItemKey(items[i].RepoName, items[i].Branch)
		itemMap[key] = &items[i]
	}

	groupedKeys := make(map[string]bool)
	var groups []pickerGroup

	for _, repoName := range repoNames {
		branchSet := repoBranchSets[repoName]
		if len(branchSet) == 0 {
			continue
		}
		st := loadStack(repoName)
		if st == nil || st.IsEmpty() {
			continue
		}

		treeLines := st.TreeLines(branchSet)
		prefixByBranch := make(map[string]string, len(treeLines))
		for _, tl := range treeLines {
			prefixByBranch[tl.Branch] = tl.Prefix
		}

		for _, root := range st.Roots() {
			var groupItems []PickerItem
			for _, branch := range st.SubtreeSort(root) {
				key := pickerItemKey(repoName, branch)
				item, ok := itemMap[key]
				if !ok {
					continue
				}
				item.StackPrefix = prefixByBranch[branch]
				groupedKeys[pickerStableItemKey(*item)] = true
				groupItems = append(groupItems, *item)
			}
			if len(groupItems) > 0 {
				groups = append(groups, newPickerGroup(groupItems))
			}
		}
	}

	for _, item := range items {
		key := pickerStableItemKey(item)
		if !groupedKeys[key] {
			groups = append(groups, newPickerGroup([]PickerItem{item}))
		}
	}

	return groups
}

func newPickerGroup(items []PickerItem) pickerGroup {
	group := pickerGroup{
		items:    items,
		merged:   true,
		priority: claude.WorktreeUrgencyOrder(claude.StatusOffline, false),
	}
	for _, item := range items {
		if !item.Merged {
			group.merged = false
		}
		priority := claude.WorktreeUrgencyOrder(item.Status, item.Unread)
		if priority < group.priority {
			group.priority = priority
		}
	}
	return group
}

func pickerItemKey(repoName, branch string) string {
	return repoName + "/" + branch
}

func pickerStableItemKey(item PickerItem) string {
	if item.WtPath == "" {
		return pickerItemKey(item.RepoName, item.Branch)
	}
	return item.RepoName + "/" + item.WtPath
}

// FormatPickerLines produces ANSI-colored lines for the fzf picker.
// When a worktree has multiple active Claude sessions, sub-rows are shown
// indented below the parent row.
func FormatPickerLines(items []PickerItem) []string {
	multiRepo := hasMultipleRepos(items)

	nameW := 0
	for _, item := range items {
		plain := displayName(item, multiRepo)
		if item.Merged {
			plain += " [merged]"
		}
		if utf8.RuneCountInString(plain) > nameW {
			nameW = utf8.RuneCountInString(plain)
		}
	}

	var lines []string
	for _, item := range items {
		color := statusColor(item.Status)
		icon := claude.StatusIcon(item.Status)
		label := fmt.Sprintf("%-7s", claude.StatusLabel(item.Status))

		dot := " "
		if item.Unread {
			dot = "\u25CF"
		}

		namePlain := displayName(item, multiRepo)
		name := namePlain
		if item.Merged {
			namePlain += " [merged]"
			name += fmt.Sprintf(" %s[merged]%s", colorDim, colorReset)
		}
		padding := nameW - utf8.RuneCountInString(namePlain)
		if padding < 0 {
			padding = 0
		}
		nameCol := name + strings.Repeat(" ", padding)

		line := fmt.Sprintf("%s%s %s%s%s | %s | %s%s%s",
			color, icon, label, dot, colorReset,
			nameCol,
			colorDim, shortenPath(item.WtPath), colorReset,
		)
		lines = append(lines, line)

		activeSessions := filterActiveSessions(item.Sessions)
		if len(activeSessions) > 1 {
			for _, ss := range activeSessions {
				effStatus := claude.EffectiveStatus(ss.Status, ss.Timestamp)
				subColor := statusColor(effStatus)
				subIcon := claude.StatusIcon(effStatus)
				subLabel := fmt.Sprintf("%-6s", claude.StatusLabel(effStatus))

				prefix := "\u2514 "
				sid := truncate(ss.SessionID, 8)
				timeAgo := claude.TimeSince(ss.Timestamp)

				plainLen := utf8.RuneCountInString(prefix) + len(sid)
				infoAnsi := colorDim + prefix + sid
				if ss.Tool != "" {
					infoAnsi += fmt.Sprintf(" %s(%s)%s", colorDim, ss.Tool, colorReset)
					plainLen += 2 + len(ss.Tool) + 1 // " (" + tool + ")"
				}
				infoAnsi += " " + timeAgo
				plainLen += 1 + utf8.RuneCountInString(timeAgo)

				pad := nameW - plainLen
				if pad < 0 {
					pad = 0
				}
				infoCol := infoAnsi + strings.Repeat(" ", pad) + colorReset

				subLine := fmt.Sprintf("  %s%s %s%s | %s | %s%s%s",
					subColor, subIcon, subLabel, colorReset,
					infoCol,
					colorDim, shortenPath(item.WtPath), colorReset,
				)
				lines = append(lines, subLine)
			}
		}
	}
	return lines
}

func filterActiveSessions(sessions []*claude.SessionStatus) []*claude.SessionStatus {
	var active []*claude.SessionStatus
	for _, ss := range sessions {
		eff := claude.EffectiveStatus(ss.Status, ss.Timestamp)
		if eff != claude.StatusIdle && eff != claude.StatusOffline {
			active = append(active, ss)
		}
	}
	return active
}

func displayName(item PickerItem, multiRepo bool) string {
	name := item.Branch
	if item.Detached {
		if item.Head == "" {
			name = fmt.Sprintf("%s [detached]", item.WtDirName)
		} else {
			name = fmt.Sprintf("%s [detached %s]", item.WtDirName, worktree.ShortHead(item.Head))
		}
	}
	if multiRepo {
		name = item.RepoName + "/" + name
	}
	if item.StackPrefix != "" && !item.Detached {
		name = item.StackPrefix + name
	}
	return name
}

// ExtractPathFromLine pulls the worktree path from the last pipe-delimited field,
// expanding ~ to the home directory.
func ExtractPathFromLine(line string) string {
	parts := strings.Split(line, "|")
	if len(parts) == 0 {
		return ""
	}
	raw := strings.TrimSpace(parts[len(parts)-1])
	raw = stripAnsi(raw)
	return expandHome(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func stripAnsi(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func statusColor(s claude.Status) string {
	switch s {
	case claude.StatusBusy:
		return colorGreen
	case claude.StatusWait:
		return colorRed
	case claude.StatusDone:
		return colorBlue
	case claude.StatusIdle:
		return colorYellow
	default:
		return colorDim
	}
}

func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func hasMultipleRepos(items []PickerItem) bool {
	if len(items) <= 1 {
		return false
	}
	first := items[0].RepoName
	for _, item := range items[1:] {
		if item.RepoName != first {
			return true
		}
	}
	return false
}
