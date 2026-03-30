package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
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
	WtDirName   string
	WtPath      string
	Status      claude.Status
	Unread      bool
	HasSession  bool
	Sessions    []*claude.SessionStatus
	Merged      bool
	StackPrefix string // tree-drawing prefix for stacked branches (e.g., "├─ ")
}

func BuildPickerItems(repoFilter string) ([]PickerItem, error) {
	var repoNames []string
	if repoFilter != "" {
		repoNames = []string{repoFilter}
	} else {
		var err error
		repoNames, err = config.ListRepos()
		if err != nil {
			return nil, fmt.Errorf("failed to list repos: %w", err)
		}
	}

	var items []PickerItem
	for _, repoName := range repoNames {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}

		// Detect merged branches for this repo
		cfg := config.Load(bareDir)
		baseBranch := cfg.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		mergedBranches, _ := repoGit.MergedBranches(baseBranch)
		mergedSet := make(map[string]bool)
		for _, b := range mergedBranches {
			mergedSet[b] = true
		}

		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			wtDir := filepath.Base(wt.Path)
			ws := claude.ReadStatus(repoName, wtDir)
			sessions := claude.ReadAllSessions(repoName, wtDir)
			sessName := SessionNameForWorktree(repoName, wtDir)
			items = append(items, PickerItem{
				RepoName:   repoName,
				Branch:     wt.Branch,
				WtDirName:  wtDir,
				WtPath:     wt.Path,
				Status:     ws.Status,
				Unread:     ws.Status == claude.StatusDone && claude.IsUnread(repoName, wtDir),
				HasSession: SessionExists(sessName),
				Sessions:   sessions,
				Merged:     mergedSet[wt.Branch],
			})
		}
	}

	// Compute stack prefixes and reorder: stacked branches grouped by tree, then non-stacked by status
	items = applyStackOrder(items, repoNames)

	return items, nil
}

func applyStackOrder(items []PickerItem, repoNames []string) []PickerItem {
	// Build branch set for tree line computation
	branchSet := make(map[string]bool)
	for _, item := range items {
		branchSet[item.Branch] = true
	}

	// Build item lookup by repo/branch to avoid collisions across repos
	itemMap := make(map[string]*PickerItem)
	for i := range items {
		key := items[i].RepoName + "/" + items[i].Branch
		itemMap[key] = &items[i]
	}

	// Load stacks for each repo and compute tree lines
	stackedKeys := make(map[string]bool)
	var stackedItems []PickerItem

	for _, repoName := range repoNames {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}
		st := stack.Load(bareDir)
		if st.IsEmpty() {
			continue
		}

		treeLines := st.TreeLines(branchSet)
		for _, tl := range treeLines {
			key := repoName + "/" + tl.Branch
			if item, ok := itemMap[key]; ok {
				item.StackPrefix = tl.Prefix
				stackedKeys[key] = true
				stackedItems = append(stackedItems, *item)
			}
		}
	}

	// Collect non-stacked items
	var nonStacked []PickerItem
	for _, item := range items {
		key := item.RepoName + "/" + item.Branch
		if !stackedKeys[key] {
			nonStacked = append(nonStacked, item)
		}
	}

	// Sort non-stacked by status (merged last)
	sort.SliceStable(nonStacked, func(i, j int) bool {
		if nonStacked[i].Merged != nonStacked[j].Merged {
			return !nonStacked[i].Merged
		}
		return statusOrder(nonStacked[i].Status) < statusOrder(nonStacked[j].Status)
	})

	// Stacked first (in tree order), then non-stacked (by status)
	result := append(stackedItems, nonStacked...)
	return result
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
		// Pad based on plain text width to avoid ANSI miscount
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

		// Show sub-rows for multiple active sessions
		activeSessions := filterActiveSessions(item.Sessions)
		if len(activeSessions) > 1 {
			for _, ss := range activeSessions {
				effStatus := claude.EffectiveStatus(ss.Status, ss.Timestamp)
				subColor := statusColor(effStatus)
				subIcon := claude.StatusIcon(effStatus)
				subLabel := fmt.Sprintf("%-5s", claude.StatusLabel(effStatus))
				toolInfo := ""
				if ss.Tool != "" {
					toolInfo = fmt.Sprintf(" %s(%s)%s", colorDim, ss.Tool, colorReset)
				}
				timeAgo := claude.TimeSince(ss.Timestamp)
				subLine := fmt.Sprintf("  %s%s %s%s | %s\u2514 %s%s %s%s%s | %s%s%s",
					subColor, subIcon, subLabel, colorReset,
					colorDim, truncate(ss.SessionID, 8), toolInfo, timeAgo, colorDim, colorReset,
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
	if multiRepo {
		name = item.RepoName + "/" + item.Branch
	}
	if item.StackPrefix != "" {
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

func statusOrder(s claude.Status) int {
	switch s {
	case claude.StatusBusy:
		return 0
	case claude.StatusWait:
		return 1
	case claude.StatusDone:
		return 2
	case claude.StatusIdle:
		return 3
	default:
		return 4
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
