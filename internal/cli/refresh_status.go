package cli

import (
	"context"
	"fmt"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func refreshStatusCmd() *cli.Command {
	return &cli.Command{
		Name:  "refresh-status",
		Usage: "Remove orphaned session files whose tmux sessions no longer exist",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Print what would be removed without deleting",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.refresh-status")()
			u := parseFlags(cmd).NewUI()
			dryRun := cmd.Bool("dry-run")

			sessions, err := claude.ScanAllSessions()
			if err != nil {
				return fmt.Errorf("failed to scan sessions: %w", err)
			}

			removed := 0
			for _, si := range sessions {
				if si.Session.Status != claude.StatusBusy && si.Session.Status != claude.StatusWait {
					continue
				}

				sessName := tmux.SessionNameForWorktree(si.RepoName, si.WorktreeDir)
				if tmux.SessionExists(sessName) {
					continue
				}

				if dryRun {
					u.Info(fmt.Sprintf("Would remove %s/%s session %s (%s)",
						si.RepoName, si.WorktreeDir, si.Session.SessionID, si.Session.Status))
				} else {
					if err := claude.RemoveSessionFile(si.RepoName, si.WorktreeDir, si.Session.SessionID); err != nil {
						u.Warn(fmt.Sprintf("Failed to remove %s/%s session %s: %v",
							si.RepoName, si.WorktreeDir, si.Session.SessionID, err))
						continue
					}
				}
				removed++
			}

			if !dryRun {
				if err := claude.CleanEmptyStatusDirs(); err != nil {
					u.Warn(fmt.Sprintf("Failed to clean empty status dirs: %v", err))
				}
			}

			if removed == 0 {
				u.Info("No orphaned sessions found.")
			} else if dryRun {
				u.Info(fmt.Sprintf("Would remove %d orphaned sessions", removed))
			} else {
				u.Success(fmt.Sprintf("Removed %d orphaned sessions", removed))
			}

			return nil
		},
	}
}
