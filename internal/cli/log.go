package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/urfave/cli/v3"
)

func logCmd() *cli.Command {
	return &cli.Command{
		Name:  "log",
		Usage: "Show activity log of worktree events",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "branch",
				Usage: "Filter by branch name",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Filter by repo name",
			},
			&cli.StringFlag{
				Name:  "since",
				Usage: "Show events after duration (e.g. 7d, 24h, 30m)",
			},
			&cli.IntFlag{
				Name:    "limit",
				Aliases: []string{"n"},
				Usage:   "Max events to show",
				Value:   20,
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()

			opts := log.ReadOpts{
				Repo:   cmd.String("repo"),
				Branch: cmd.String("branch"),
				Limit:  int(cmd.Int("limit")),
			}

			if since := cmd.String("since"); since != "" {
				d, err := parseDuration(since)
				if err != nil {
					return err
				}
				opts.Since = time.Now().Add(-d)
			}

			events, err := log.Read(opts)
			if err != nil {
				return fmt.Errorf("read log: %w", err)
			}

			if len(events) == 0 {
				u.Info("No log events found.")
				return nil
			}

			if cmd.Bool("json") {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(events)
			}

			// Column widths
			actionW, repoW, branchW := 0, 0, 0
			for _, e := range events {
				if len(e.Action) > actionW {
					actionW = len(e.Action)
				}
				if len(e.Repo) > repoW {
					repoW = len(e.Repo)
				}
				if len(e.Branch) > branchW {
					branchW = len(e.Branch)
				}
			}

			for _, e := range events {
				ts := e.Timestamp.Local().Format("Jan 02 15:04")
				meta := formatMetadata(e.Metadata)
				line := fmt.Sprintf("  %s  %-*s  %-*s  %-*s",
					u.Dim(ts), actionW, e.Action, repoW, e.Repo, branchW, e.Branch)
				if meta != "" {
					line += "  " + u.Dim(meta)
				}
				u.Info(line)
			}

			return nil
		},
	}
}

func formatMetadata(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for k, v := range m {
		if v == "" {
			continue
		}
		// Truncate long values (e.g. prompts)
		if len(v) > 60 {
			v = v[:57] + "..."
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// parseDuration parses durations like "7d", "24h", "30m".
// Extends Go's time.ParseDuration with support for "d" (days).
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid duration %q", s+"d")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use e.g. 7d, 24h, 30m)", s)
	}
	return d, nil
}
