package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func tmuxCmd() *cli.Command {
	return &cli.Command{
		Name:  "tmux",
		Usage: "Tmux integration for worktree management",
		Commands: []*cli.Command{
			tmuxPickCmd(),
			tmuxPreviewCmd(),
			tmuxListCmd(),
			tmuxStatusBarCmd(),
			tmuxInstallCmd(),
		},
	}
}

func tmuxPickCmd() *cli.Command {
	return &cli.Command{
		Name:  "pick",
		Usage: "Interactive worktree picker for tmux (fzf popup)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Filter to a specific repo",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			repoFilter := cmd.String("repo")
			self, err := os.Executable()
			if err != nil {
				self = "willow"
			}

			for {
				items, err := tmux.BuildPickerItems(repoFilter)
				if err != nil {
					return err
				}
				if len(items) == 0 {
					fmt.Fprintln(os.Stderr, "No worktrees found.")
					return nil
				}

				lines := tmux.FormatPickerLines(items)

				// Find current session index for auto-navigate
				startPos := 0
				if curSess, err := tmux.CurrentSession(); err == nil {
					for i, item := range items {
						sessName := tmux.SessionNameForWorktree(item.RepoName, item.WtDirName)
						if sessName == curSess {
							startPos = i + 1 // fzf pos is 1-indexed
							break
						}
					}
				}

				previewCmd := fmt.Sprintf("%s tmux preview {}", self)
				reloadCmd := fmt.Sprintf("sleep 2 && %s tmux list", self)
				if repoFilter != "" {
					reloadCmd += fmt.Sprintf(" --repo %s", repoFilter)
				}

				opts := []fzf.Option{
					fzf.WithAnsi(),
					fzf.WithReverse(),
					fzf.WithNoSort(),
					fzf.WithHeader("Enter: Switch | Ctrl-N: New | Ctrl-D: Delete"),
					fzf.WithPreview(previewCmd, "right:50%:wrap:follow"),
					fzf.WithExpectKeys("ctrl-n", "ctrl-d"),
					fzf.WithPrintQuery(),
					fzf.WithBind(fmt.Sprintf("start:reload-sync(%s)", reloadCmd)),
				}
				if startPos > 0 {
					opts = append(opts, fzf.WithBind(fmt.Sprintf("start:pos(%d)", startPos)))
				}

				result, err := fzf.RunExpect(lines, opts...)
				if err != nil {
					return err
				}
				if result == nil {
					return nil
				}

				switch result.Key {
				case "ctrl-n":
					if err := tmuxPickNew(self, result.Query, repoFilter); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					}
					continue

				case "ctrl-d":
					if result.Selection == "" {
						continue
					}
					if err := tmuxPickDelete(self, result.Selection, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					}
					continue

				default:
					if result.Selection == "" {
						return nil
					}
					return tmuxPickSwitch(result.Selection, items)
				}
			}
		},
	}
}

func tmuxPickSwitch(selection string, items []tmux.PickerItem) error {
	wtPath := tmux.ExtractPathFromLine(selection)
	item := findItemByPath(items, wtPath)
	if item == nil {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	sessName := tmux.SessionNameForWorktree(item.RepoName, item.WtDirName)
	if !tmux.SessionExists(sessName) {
		if err := tmux.NewSession(sessName, item.WtPath); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	claude.MarkRead(item.RepoName, item.WtDirName)
	return tmux.SwitchClient(sessName)
}

func tmuxPickNew(self, query, repoFilter string) error {
	if query == "" {
		return fmt.Errorf("enter a branch name first")
	}

	args := []string{"new", query, "--cd"}
	if repoFilter != "" {
		args = append(args, "--repo", repoFilter)
	}

	cmd := exec.Command(self, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	wtPath := strings.TrimSpace(string(out))
	if wtPath == "" {
		return fmt.Errorf("no path returned from willow new")
	}

	// Path format: ~/.willow/worktrees/<repo>/<dir>
	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))

	sessName := tmux.SessionNameForWorktree(repoName, wtDir)
	if !tmux.SessionExists(sessName) {
		if err := tmux.NewSession(sessName, wtPath); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	return tmux.SwitchClient(sessName)
}

func tmuxPickDelete(self, selection string, items []tmux.PickerItem) error {
	wtPath := tmux.ExtractPathFromLine(selection)
	item := findItemByPath(items, wtPath)
	if item == nil {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	sessName := tmux.SessionNameForWorktree(item.RepoName, item.WtDirName)
	if tmux.SessionExists(sessName) {
		tmux.KillSession(sessName)
	}

	cmd := exec.Command(self, "rm", item.Branch, "--force", "--repo", item.RepoName)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findItemByPath(items []tmux.PickerItem, path string) *tmux.PickerItem {
	for i := range items {
		if items[i].WtPath == path {
			return &items[i]
		}
	}
	return nil
}

func tmuxPreviewCmd() *cli.Command {
	return &cli.Command{
		Name:   "preview",
		Usage:  "Preview helper for fzf (shows tmux pane content)",
		Hidden: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			line := strings.Join(cmd.Args().Slice(), " ")
			if line == "" {
				return nil
			}

			wtPath := tmux.ExtractPathFromLine(line)
			wtDir := filepath.Base(wtPath)
			repoName := filepath.Base(filepath.Dir(wtPath))

			sessName := tmux.SessionNameForWorktree(repoName, wtDir)
			if !tmux.SessionExists(sessName) {
				fmt.Printf("\033[2mSession '%s' is offline.\033[0m\n\n", sessName)
				fmt.Println("Press Enter to start the session.")
				return nil
			}

			content, err := tmux.CapturePane(sessName)
			if err != nil {
				fmt.Printf("\033[2mCould not capture pane: %v\033[0m\n", err)
				return nil
			}

			fmt.Printf("\033[0;34m\u2500\u2500 %s \u2500\u2500\033[0m\n\n", sessName)
			fmt.Print(content)
			return nil
		},
	}
}

func tmuxListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "Print formatted picker lines (for fzf reload)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Filter to a specific repo",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			items, err := tmux.BuildPickerItems(cmd.String("repo"))
			if err != nil {
				return err
			}
			for _, line := range tmux.FormatPickerLines(items) {
				fmt.Println(line)
			}
			return nil
		},
	}
}

func tmuxStatusBarCmd() *cli.Command {
	return &cli.Command{
		Name:  "status-bar",
		Usage: "Tmux status-right widget showing worktree and agent counts",
		Action: func(_ context.Context, cmd *cli.Command) error {
			repos, err := config.ListRepos()
			if err != nil {
				return nil
			}

			totalWt := 0
			activeAgents := 0
			for _, repoName := range repos {
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
					totalWt++
					wtDir := filepath.Base(wt.Path)
					ws := claude.ReadStatus(repoName, wtDir)
					if ws.Status == claude.StatusBusy || ws.Status == claude.StatusWait || ws.Status == claude.StatusDone {
						activeAgents++
					}
				}
			}

			fmt.Printf("#[fg=#98be65]\U0001F333 %d #[fg=#51afef]\U0001F916 %d", totalWt, activeAgents)
			return nil
		},
	}
}

func tmuxInstallCmd() *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "Print tmux.conf lines to add for willow integration",
		Action: func(_ context.Context, cmd *cli.Command) error {
			self, err := os.Executable()
			if err != nil {
				self = "willow"
			}

			fmt.Println("# Willow tmux integration")
			fmt.Println("# Add these lines to your tmux.conf:")
			fmt.Println()
			fmt.Printf("bind w display-popup -E -w 90%% -h 80%% \"%s tmux pick\"\n", self)
			fmt.Printf("set -g status-right '#(%s tmux status-bar) %%l:%%M %%a'\n", self)
			fmt.Println("set -g status-interval 3")
			return nil
		},
	}
}
