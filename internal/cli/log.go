package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/termfmt"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.log")()
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

			for _, line := range formatLogLines(u, events, termfmt.TerminalWidth()) {
				u.Info(line)
			}

			return nil
		},
	}
}

func formatLogLines(u *ui.UI, events []log.Event, width int) []string {
	type row struct {
		ts     string
		action string
		repo   string
		branch string
		meta   string
	}
	rows := make([]row, 0, len(events))
	tsW, actionW, repoW, branchW, metaW := 0, 0, 0, 0, 0
	for _, e := range events {
		r := row{
			ts:     e.Timestamp.Local().Format("Jan 02 15:04"),
			action: e.Action,
			repo:   e.Repo,
			branch: e.Branch,
			meta:   formatMetadata(e.Metadata),
		}
		rows = append(rows, r)
		tsW = max(tsW, termfmt.VisibleWidth(r.ts))
		actionW = max(actionW, termfmt.VisibleWidth(r.action))
		repoW = max(repoW, termfmt.VisibleWidth(r.repo))
		branchW = max(branchW, termfmt.VisibleWidth(r.branch))
		metaW = max(metaW, termfmt.VisibleWidth(r.meta))
	}

	termWidth := termfmt.Width(width)
	fixedWithoutBranchMeta := 2 + tsW + 2 + actionW + 2 + repoW
	hasMeta := metaW > 0
	if hasMeta {
		fixedWithoutBranchMeta += 2
	}
	available := termWidth - fixedWithoutBranchMeta
	if hasMeta && branchW+2+metaW > available {
		if metaAvailable := available - branchW - 2; metaAvailable >= 1 {
			metaW = min(metaW, metaAvailable)
		} else {
			metaW = 1
			branchAvailable := available - metaW - 2
			if branchAvailable < 1 {
				branchAvailable = 1
			}
			branchW = min(branchW, branchAvailable)
		}
	} else if !hasMeta && branchW > available {
		branchW = max(1, available)
	}

	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		line := fmt.Sprintf("  %s  %s  %s  %s",
			u.Dim(termfmt.FitRight(r.ts, tsW)),
			termfmt.FitRight(r.action, actionW),
			termfmt.FitRight(r.repo, repoW),
			termfmt.FitRight(r.branch, branchW),
		)
		if hasMeta {
			line += "  " + u.Dim(termfmt.FitRight(r.meta, metaW))
		}
		lines = append(lines, line)
	}
	return lines
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
