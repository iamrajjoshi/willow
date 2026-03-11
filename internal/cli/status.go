package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Aliases: []string{"s"},
		Usage:   "Show Claude Code agent status per worktree",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			bareDir, err := requireWillowRepo(g)
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			filtered := filterBareWorktrees(worktrees)
			repoName := repoNameFromDir(bareDir)

			type sessionEntry struct {
				Branch    string `json:"branch"`
				SessionID string `json:"session_id,omitempty"`
				Status    string `json:"status"`
				Timestamp string `json:"timestamp,omitempty"`
				Unread    bool   `json:"unread,omitempty"`
				Path      string `json:"path"`
			}

			var entries []sessionEntry
			activeCount := 0
			totalUnread := 0

			for _, wt := range filtered {
				wtDir := filepath.Base(wt.Path)
				sessions := claude.ReadAllSessions(repoName, wtDir)
				unread := claude.IsUnread(repoName, wtDir)
				if unread {
					totalUnread++
				}

				if len(sessions) > 0 {
					for _, ss := range sessions {
						entry := sessionEntry{
							Branch:    wt.Branch,
							SessionID: ss.SessionID,
							Status:    string(ss.Status),
							Path:      wt.Path,
						}
						if !ss.Timestamp.IsZero() {
							entry.Timestamp = claude.TimeSince(ss.Timestamp)
						}
						if ss.Status == claude.StatusDone && unread {
							entry.Unread = true
						}
						entries = append(entries, entry)

						if ss.Status == claude.StatusBusy || ss.Status == claude.StatusDone || ss.Status == claude.StatusWait {
							activeCount++
						}
					}
				} else {
					ws := claude.ReadStatus(repoName, wtDir)
					entry := sessionEntry{
						Branch: wt.Branch,
						Status: string(ws.Status),
						Path:   wt.Path,
					}
					if !ws.Timestamp.IsZero() {
						entry.Timestamp = claude.TimeSince(ws.Timestamp)
					}
					entries = append(entries, entry)

					if ws.Status == claude.StatusBusy || ws.Status == claude.StatusDone || ws.Status == claude.StatusWait {
						activeCount++
					}
				}
			}

			if cmd.Bool("json") {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			headerParts := fmt.Sprintf("%s (\U0001F333 %d worktrees, \U0001F916 %d active",
				u.Bold(repoName), len(filtered), activeCount)
			if totalUnread > 0 {
				headerParts += fmt.Sprintf(", %d unread", totalUnread)
			}
			headerParts += ")\n"
			u.Info(headerParts)

			branchW := 0
			for _, e := range entries {
				if len(e.Branch) > branchW {
					branchW = len(e.Branch)
				}
			}

			for _, e := range entries {
				icon := claude.StatusIcon(claude.Status(e.Status))
				label := e.Status
				if e.Unread {
					label += "\u25CF" // ●
				}
				var line string
				if e.Timestamp != "" {
					line = fmt.Sprintf("  %s %-*s  %-6s  %s",
						icon, branchW, e.Branch, label, u.Dim(e.Timestamp))
				} else {
					line = fmt.Sprintf("  %s %-*s  %s",
						icon, branchW, e.Branch, label)
				}
				u.Info(line)
			}

			return nil
		},
	}
}
