package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/parallel"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

type sessionEntry struct {
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch"`
	Harness   string `json:"harness,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp,omitempty"`
	Unread    bool   `json:"unread,omitempty"`
	Path      string `json:"path"`
}

type repoStatus struct {
	Name          string
	Entries       []sessionEntry
	WorktreeCount int
	ActiveCount   int
	UnreadCount   int
}

func collectRepoStatus(repoName string, worktrees []worktree.Worktree) repoStatus {
	rs := repoStatus{Name: repoName, WorktreeCount: len(worktrees)}
	for _, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		sessions := agent.ReadAllSessions(repoName, wtDir)
		if agent.CountUnreadIn(repoName, wtDir, sessions) > 0 {
			rs.UnreadCount++
		}

		if len(sessions) > 0 {
			for _, ss := range sessions {
				effective := agent.EffectiveStatus(ss.Status, ss.Timestamp)
				entry := sessionEntry{
					Repo:      repoName,
					Branch:    wt.DisplayName(),
					Harness:   ss.Harness,
					SessionID: ss.SessionID,
					Status:    string(effective),
					Path:      wt.Path,
				}
				if !ss.Timestamp.IsZero() {
					entry.Timestamp = agent.TimeSince(ss.Timestamp)
				}
				if effective == agent.StatusDone && agent.CountUnreadIn(repoName, wtDir, []*agent.SessionStatus{ss}) > 0 {
					entry.Unread = true
				}
				rs.Entries = append(rs.Entries, entry)

				if agent.IsActive(effective) {
					rs.ActiveCount++
				}
			}
		} else {
			ws := agent.AggregateStatus(sessions)
			entry := sessionEntry{
				Repo:   repoName,
				Branch: wt.DisplayName(),
				Status: string(ws.Status),
				Path:   wt.Path,
			}
			if !ws.Timestamp.IsZero() {
				entry.Timestamp = agent.TimeSince(ws.Timestamp)
			}
			rs.Entries = append(rs.Entries, entry)

			if agent.IsActive(ws.Status) {
				rs.ActiveCount++
			}
		}
	}
	return rs
}

func collectRepoStatuses(repos []repoInfo, verbose bool) []repoStatus {
	type result struct {
		status repoStatus
		ok     bool
	}

	results := parallel.Map(repos, func(_ int, r repoInfo) result {
		repoGit := &git.Git{Dir: r.BareDir, Verbose: verbose}
		wts, err := worktree.ListWithOptions(repoGit, worktree.ListOptions{ResolveHeads: false})
		if err != nil {
			return result{}
		}
		return result{
			status: collectRepoStatus(r.Name, filterBareWorktrees(wts)),
			ok:     true,
		}
	})

	statuses := make([]repoStatus, 0, len(repos))
	for _, result := range results {
		if result.ok {
			statuses = append(statuses, result.status)
		}
	}
	return statuses
}

func statusBranchLabel(branch, harnessID, sessionID string) string {
	if sessionID == "" {
		return branch
	}
	prefix := agent.ShortSessionID(sessionID)
	if harnessID != "" {
		prefix = fmt.Sprintf("%s:%s", harnessID, prefix)
	}
	return fmt.Sprintf("%s [%s]", branch, prefix)
}

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Aliases: []string{"s"},
		Usage:   "Show agent status per worktree",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.status")()
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			repos, err := resolveRepos(g, cmd.String("repo"))
			if err != nil {
				return err
			}

			allStatuses := collectRepoStatuses(repos, g.Verbose)

			if cmd.Bool("json") {
				var allEntries []sessionEntry
				for _, rs := range allStatuses {
					allEntries = append(allEntries, rs.Entries...)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(allEntries)
			}

			for i, rs := range allStatuses {
				if i > 0 {
					u.Info("")
				}
				headerParts := fmt.Sprintf("%s (\U0001F333 %d worktrees, \U0001F916 %d active",
					u.Bold(rs.Name), rs.WorktreeCount, rs.ActiveCount)
				if rs.UnreadCount > 0 {
					headerParts += fmt.Sprintf(", %d unread", rs.UnreadCount)
				}
				headerParts += ")\n"
				u.Info(headerParts)

				branchW := 0
				for _, e := range rs.Entries {
					label := statusBranchLabel(e.Branch, e.Harness, e.SessionID)
					if len(label) > branchW {
						branchW = len(label)
					}
				}

				for _, e := range rs.Entries {
					icon := agent.StatusIcon(agent.Status(e.Status))
					label := e.Status
					if e.Unread {
						label += "\u25CF" // bullet
					}
					branchLabel := statusBranchLabel(e.Branch, e.Harness, e.SessionID)
					var line string
					if e.Timestamp != "" {
						line = fmt.Sprintf("  %s %-*s  %-6s  %s",
							icon, branchW, branchLabel, label, u.Dim(e.Timestamp))
					} else {
						line = fmt.Sprintf("  %s %-*s  %s",
							icon, branchW, branchLabel, label)
					}
					u.Info(line)
				}
			}

			return nil
		},
	}
}
