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

			type statusEntry struct {
				Branch    string `json:"branch"`
				Status    string `json:"status"`
				Timestamp string `json:"timestamp,omitempty"`
				Path      string `json:"path"`
			}

			var entries []statusEntry
			activeCount := 0

			for _, wt := range filtered {
				wtDir := filepath.Base(wt.Path)
				ws := claude.ReadStatus(repoName, wtDir)

				entry := statusEntry{
					Branch: wt.Branch,
					Status: string(ws.Status),
					Path:   wt.Path,
				}
				if !ws.Timestamp.IsZero() {
					entry.Timestamp = claude.TimeSince(ws.Timestamp)
				}
				entries = append(entries, entry)

				if ws.Status == claude.StatusBusy || ws.Status == claude.StatusWait {
					activeCount++
				}
			}

			if cmd.Bool("json") {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			activeLabel := "agent active"
			if activeCount != 1 {
				activeLabel = "agents active"
			}
			u.Info(fmt.Sprintf("%s (%d worktrees, %d %s)\n",
				u.Bold(repoName), len(filtered), activeCount, activeLabel))

			branchW := 0
			for _, e := range entries {
				if len(e.Branch) > branchW {
					branchW = len(e.Branch)
				}
			}

			for _, e := range entries {
				icon := claude.StatusIcon(claude.Status(e.Status))
				var line string
				if e.Timestamp != "" {
					line = fmt.Sprintf("  %s %-*s  %-4s  %s",
						icon, branchW, e.Branch, e.Status, u.Dim(e.Timestamp))
				} else {
					line = fmt.Sprintf("  %s %-*s  %s",
						icon, branchW, e.Branch, e.Status)
				}
				u.Info(line)
			}

			return nil
		},
	}
}
