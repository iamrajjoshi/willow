package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

type sessionEntry struct {
	Repo      string              `json:"repo,omitempty"`
	Branch    string              `json:"branch"`
	SessionID string              `json:"session_id,omitempty"`
	Status    string              `json:"status"`
	Timestamp string              `json:"timestamp,omitempty"`
	Unread    bool                `json:"unread,omitempty"`
	Path      string              `json:"path"`
	Cost      *claude.CostEstimate `json:"cost,omitempty"`
}

type repoStatus struct {
	Name          string
	Entries       []sessionEntry
	WorktreeCount int
	ActiveCount   int
	UnreadCount   int
}

func collectRepoStatus(repoName string, worktrees []worktree.Worktree, costCfg *config.CostConfig) repoStatus {
	rs := repoStatus{Name: repoName, WorktreeCount: len(worktrees)}
	for _, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		sessions := claude.ReadAllSessions(repoName, wtDir)
		unread := claude.IsUnread(repoName, wtDir)
		if unread {
			rs.UnreadCount++
		}

		if len(sessions) > 0 {
			for _, ss := range sessions {
				effective := claude.EffectiveStatus(ss.Status, ss.Timestamp)
				entry := sessionEntry{
					Repo:      repoName,
					Branch:    wt.Branch,
					SessionID: ss.SessionID,
					Status:    string(effective),
					Path:      wt.Path,
				}
				if !ss.Timestamp.IsZero() {
					entry.Timestamp = claude.TimeSince(ss.Timestamp)
				}
				if effective == claude.StatusDone && unread {
					entry.Unread = true
				}
				if costCfg != nil {
					entry.Cost = claude.EstimateFromSession(ss, costCfg.InputRate, costCfg.OutputRate)
				}
				rs.Entries = append(rs.Entries, entry)

				if claude.IsActive(effective) {
					rs.ActiveCount++
				}
			}
		} else {
			ws := claude.ReadStatus(repoName, wtDir)
			entry := sessionEntry{
				Repo:   repoName,
				Branch: wt.Branch,
				Status: string(ws.Status),
				Path:   wt.Path,
			}
			if !ws.Timestamp.IsZero() {
				entry.Timestamp = claude.TimeSince(ws.Timestamp)
			}
			rs.Entries = append(rs.Entries, entry)

			if claude.IsActive(ws.Status) {
				rs.ActiveCount++
			}
		}
	}
	return rs
}

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
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "cost",
				Usage: "Show estimated token cost per session",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()
			showCost := cmd.Bool("cost")

			repos, err := resolveRepos(g, cmd.String("repo"))
			if err != nil {
				return err
			}

			var costCfg *config.CostConfig
			if showCost {
				cfg := config.Load("")
				costCfg = &cfg.Cost
			}

			var allStatuses []repoStatus
			for _, r := range repos {
				repoGit := &git.Git{Dir: r.BareDir, Verbose: g.Verbose}
				wts, err := worktree.List(repoGit)
				if err != nil {
					continue
				}
				filtered := filterBareWorktrees(wts)
				allStatuses = append(allStatuses, collectRepoStatus(r.Name, filtered, costCfg))
			}

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
					if len(e.Branch) > branchW {
						branchW = len(e.Branch)
					}
				}

				var totalCost float64
				for _, e := range rs.Entries {
					icon := claude.StatusIcon(claude.Status(e.Status))
					label := e.Status
					if e.Unread {
						label += "\u25CF" // bullet
					}
					var line string
					if e.Timestamp != "" {
						line = fmt.Sprintf("  %s %-*s  %-6s  %s",
							icon, branchW, e.Branch, label, u.Dim(e.Timestamp))
					} else {
						line = fmt.Sprintf("  %s %-*s  %s",
							icon, branchW, e.Branch, label)
					}
					if showCost && e.Cost != nil {
						line += "  " + u.Dim(claude.FormatCost(e.Cost))
						totalCost += e.Cost.TotalUSD
					}
					u.Info(line)
				}

				if showCost {
					u.Info("")
					u.Info(fmt.Sprintf("  Total: ~$%.2f", totalCost))
				}
			}

			return nil
		},
	}
}
