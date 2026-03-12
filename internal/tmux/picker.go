package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
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
	RepoName   string
	Branch     string
	WtDirName  string
	WtPath     string
	Status     claude.Status
	Unread     bool
	HasSession bool
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
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			wtDir := filepath.Base(wt.Path)
			ws := claude.ReadStatus(repoName, wtDir)
			sessName := SessionNameForWorktree(repoName, wtDir)
			items = append(items, PickerItem{
				RepoName:   repoName,
				Branch:     wt.Branch,
				WtDirName:  wtDir,
				WtPath:     wt.Path,
				Status:     ws.Status,
				Unread:     ws.Status == claude.StatusDone && claude.IsUnread(repoName, wtDir),
				HasSession: SessionExists(sessName),
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return statusOrder(items[i].Status) < statusOrder(items[j].Status)
	})

	return items, nil
}

// FormatPickerLines produces ANSI-colored lines for the fzf picker.
//
// Format: [color]icon STATUS dot[reset] | name             | [dim]~/path[reset]
// Example: \033[0;32m🤖 BUSY   ●\033[0m | auth-refactor     | \033[2m~/.willow/worktrees/evergreen/auth-refactor\033[0m
//
// When multiple repos are present, repo name is prepended to the name column.
func FormatPickerLines(items []PickerItem) []string {
	multiRepo := hasMultipleRepos(items)

	nameW := 0
	for _, item := range items {
		label := displayName(item, multiRepo)
		if len(label) > nameW {
			nameW = len(label)
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

		name := displayName(item, multiRepo)

		line := fmt.Sprintf("%s%s %s%s%s | %-*s | %s%s%s",
			color, icon, label, dot, colorReset,
			nameW, name,
			colorDim, shortenPath(item.WtPath), colorReset,
		)
		lines = append(lines, line)
	}
	return lines
}

func displayName(item PickerItem, multiRepo bool) string {
	if multiRepo {
		return item.RepoName + "/" + item.Branch
	}
	return item.Branch
}

// ExtractPathFromLine pulls the worktree path from the last pipe-delimited field,
// expanding ~ to the home directory.
func ExtractPathFromLine(line string) string {
	parts := strings.Split(line, "|")
	if len(parts) == 0 {
		return ""
	}
	raw := strings.TrimSpace(parts[len(parts)-1])
	// Strip ANSI escape codes
	raw = stripAnsi(raw)
	return expandHome(raw)
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
