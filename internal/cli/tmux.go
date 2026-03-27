package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/dashboard"
	"github.com/iamrajjoshi/willow/internal/errs"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
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
			tmuxSwCmd(),
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

				// Find current session's line position for auto-navigate
				startPos := 0
				if curSess, err := tmux.CurrentSession(); err == nil {
					var curWtPath string
					for _, item := range items {
						sessName := tmux.SessionNameForWorktree(item.RepoName, item.WtDirName)
						if sessName == curSess {
							curWtPath = item.WtPath
							break
						}
					}
					if curWtPath != "" {
						for i, line := range lines {
							if tmux.ExtractPathFromLine(line) == curWtPath {
								startPos = i + 1 // fzf pos is 1-indexed
								break
							}
						}
					}
				}

				previewCmd := fmt.Sprintf("%s tmux preview -- {}", self)
				reloadCmd := fmt.Sprintf("%s tmux list", self)
				if repoFilter != "" {
					reloadCmd += fmt.Sprintf(" --repo %s", repoFilter)
				}

				// Chain reload-sync and pos() in a single bind so both execute
				startBind := fmt.Sprintf("start:reload-sync(%s)", reloadCmd)
				if startPos > 0 {
					startBind += fmt.Sprintf("+pos(%d)", startPos)
				}

				opts := []fzf.Option{
					fzf.WithAnsi(),
					fzf.WithReverse(),
					fzf.WithNoSort(),
					fzf.WithHeader("Enter: Switch | Ctrl-N: New | Ctrl-B: New (stacked) | Ctrl-E: Existing | Ctrl-P: PR | Ctrl-S: Sync | Ctrl-D: Delete"),
					fzf.WithExpectKeys("ctrl-n", "ctrl-b", "ctrl-e", "ctrl-p", "ctrl-s", "ctrl-d"),
					fzf.WithPrintQuery(),
					fzf.WithBind(startBind),
				}

				cfg := config.Load("")
				if cfg.Tmux.SwitcherPreview == nil || *cfg.Tmux.SwitcherPreview {
					opts = append(opts, fzf.WithPreview(previewCmd, "right:50%:wrap:follow"))
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
					if err := tmuxPickNew(self, result.Query, repoFilter, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-b":
					if err := tmuxPickNewWithBase(self, result.Query, repoFilter, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-e":
					if err := tmuxPickExisting(self, repoFilter, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-p":
					if err := tmuxPickPR(self, repoFilter, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-s":
					// Sync selected branch's subtree, or all stacks if no selection
					branch := ""
					if result.Selection != "" {
						wtPath := tmux.ExtractPathFromLine(result.Selection)
						if item := findItemByPath(items, wtPath); item != nil {
							branch = item.Branch
						}
					}
					if err := tmuxPickSync(self, repoFilter, items, branch); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					}
					// Re-enter picker loop to show refreshed state
					fmt.Fprintf(os.Stderr, "\nPress Enter to return to picker...\n")
					fmt.Fscanln(os.Stdin)
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

func tmuxSwCmd() *cli.Command {
	return &cli.Command{
		Name:  "sw",
		Usage: "Switch to a worktree's tmux session (used by shell integration)",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "path",
				UsageText: "<worktree-path>",
			},
		},
		Hidden: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			wtPath := cmd.StringArg("path")
			if wtPath == "" {
				return errs.Userf("worktree path is required")
			}

			wtDir := filepath.Base(wtPath)
			repoName := filepath.Base(filepath.Dir(wtPath))

			sessName := tmux.SessionNameForWorktree(repoName, wtDir)
			if !tmux.SessionExists(sessName) {
				cfg := loadRepoConfig(repoName)
				if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
					return fmt.Errorf("failed to create session: %w", err)
				}
			}

			claude.MarkRead(repoName, wtDir)
			return tmux.SwitchClient(sessName)
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
		cfg := loadRepoConfig(item.RepoName)
		if err := tmux.NewSession(sessName, item.WtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	claude.MarkRead(item.RepoName, item.WtDirName)
	return tmux.SwitchClient(sessName)
}

func tmuxPickNew(self, query, repoFilter string, items []tmux.PickerItem) error {
	if query == "" {
		return errs.Userf("enter a branch name first")
	}

	repo, err := resolveRepo(repoFilter, items)
	if err != nil {
		return err
	}

	args := []string{"new", "--cd", "--repo", repo, "--", query}

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
		cfg := loadRepoConfig(repoName)
		if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	return tmux.SwitchClient(sessName)
}

func tmuxPickNewWithBase(self, query, repoFilter string, items []tmux.PickerItem) error {
	if query == "" {
		return errs.Userf("enter a branch name first")
	}

	repo, err := resolveRepo(repoFilter, items)
	if err != nil {
		return err
	}

	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return err
	}

	repoGit := &git.Git{Dir: bareDir}
	wts, err := worktree.List(repoGit)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	var branches []string
	for _, wt := range wts {
		if !wt.IsBare {
			branches = append(branches, wt.Branch)
		}
	}
	if len(branches) == 0 {
		return errs.Userf("no worktrees to use as base")
	}

	base, err := fzf.Run(branches,
		fzf.WithReverse(),
		fzf.WithHeader("Select base branch"),
	)
	if err != nil {
		return err
	}
	if base == "" {
		return nil
	}

	args := []string{"new", "--base", base, "--cd", "--repo", repo, "--", query}

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

	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))

	sessName := tmux.SessionNameForWorktree(repoName, wtDir)
	if !tmux.SessionExists(sessName) {
		cfg := loadRepoConfig(repoName)
		if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	return tmux.SwitchClient(sessName)
}

func tmuxPickExisting(self, repoFilter string, items []tmux.PickerItem) error {
	repo, err := resolveRepo(repoFilter, items)
	if err != nil {
		return err
	}

	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return err
	}

	repoGit := &git.Git{Dir: bareDir}
	remoteBranches, err := repoGit.RemoteBranches()
	if err != nil {
		return fmt.Errorf("failed to list remote branches: %w", err)
	}
	if len(remoteBranches) == 0 {
		return errs.Userf("no remote branches found")
	}

	// Filter out branches that already have worktrees
	wts, err := worktree.List(repoGit)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}
	wtBranches := make(map[string]bool)
	for _, wt := range wts {
		if !wt.IsBare {
			wtBranches[wt.Branch] = true
		}
	}
	var available []string
	for _, b := range remoteBranches {
		if !wtBranches[b] {
			available = append(available, b)
		}
	}
	if len(available) == 0 {
		return errs.Userf("all remote branches already have worktrees")
	}

	// Pick branch in-process (no nested shell-out)
	branch, err := fzf.Run(available,
		fzf.WithReverse(),
		fzf.WithHeader("Select a branch to check out"),
	)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil // user cancelled
	}

	// Create worktree via `ww new -e --cd`
	args := []string{"new", "-e", "--cd", "--repo", repo, "--", branch}
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

	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))

	sessName := tmux.SessionNameForWorktree(repoName, wtDir)
	if !tmux.SessionExists(sessName) {
		cfg := loadRepoConfig(repoName)
		if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	return tmux.SwitchClient(sessName)
}

func tmuxPickPR(self, repoFilter string, items []tmux.PickerItem) error {
	repo, err := resolveRepo(repoFilter, items)
	if err != nil {
		return err
	}

	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return err
	}

	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return errs.Userf("gh CLI is required for PR picker")
	}

	cmd := exec.Command(ghPath, "pr", "list", "--json", "number,title,author,headRefName",
		"-q", `.[] | "#\(.number)  \(.title)  (\(.author.login))  [\(.headRefName)]"`)
	cmd.Dir = bareDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list PRs: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return errs.Userf("no open PRs found")
	}
	lines := strings.Split(raw, "\n")

	selected, err := fzf.Run(lines,
		fzf.WithReverse(),
		fzf.WithHeader("Select a PR to check out"),
	)
	if err != nil {
		return err
	}
	if selected == "" {
		return nil
	}

	branch := extractBranchFromPRLine(selected)
	if branch == "" {
		return fmt.Errorf("could not extract branch from selection")
	}

	args := []string{"checkout", "--cd", "--repo", repo, "--", branch}
	coCmd := exec.Command(self, args...)
	coCmd.Stderr = os.Stderr
	coOut, err := coCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to checkout PR branch: %w", err)
	}

	wtPath := strings.TrimSpace(string(coOut))
	if wtPath == "" {
		return fmt.Errorf("no path returned from checkout")
	}

	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))

	sessName := tmux.SessionNameForWorktree(repoName, wtDir)
	if !tmux.SessionExists(sessName) {
		cfg := loadRepoConfig(repoName)
		if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	return tmux.SwitchClient(sessName)
}

// extractBranchFromPRLine parses "[branch-name]" from the end of a PR picker line.
func extractBranchFromPRLine(line string) string {
	start := strings.LastIndex(line, "[")
	end := strings.LastIndex(line, "]")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return line[start+1 : end]
}

func tmuxPickSync(self, repoFilter string, items []tmux.PickerItem, branch string) error {
	repo, err := resolveRepo(repoFilter, items)
	if err != nil {
		return err
	}

	args := []string{"sync", "--repo", repo}
	if branch != "" {
		args = append(args, branch)
	}

	cmd := exec.Command(self, args...)
	cmd.Stdout = os.Stderr // Show output in the popup
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveRepo(repoFilter string, items []tmux.PickerItem) (string, error) {
	if repoFilter != "" {
		return repoFilter, nil
	}

	repos, err := config.ListRepos()
	if err != nil || len(repos) == 0 {
		return "", errs.Userf("no repos found — run 'ww clone' first")
	}

	if len(repos) == 1 {
		return repos[0], nil
	}

	currentRepo := ""
	if sess, err := tmux.CurrentSession(); err == nil {
		if parts := strings.SplitN(sess, "/", 2); len(parts) == 2 {
			currentRepo = parts[0]
		}
	}
	activeRepos := make(map[string]bool)
	for _, item := range items {
		if item.Status == claude.StatusBusy || item.Status == claude.StatusWait || item.Status == claude.StatusDone {
			activeRepos[item.RepoName] = true
		}
	}
	sort.SliceStable(repos, func(i, j int) bool {
		ci := repos[i] == currentRepo
		cj := repos[j] == currentRepo
		if ci != cj {
			return ci
		}
		ai := activeRepos[repos[i]]
		aj := activeRepos[repos[j]]
		if ai != aj {
			return ai
		}
		return repos[i] < repos[j]
	})

	selected, err := fzf.Run(repos, fzf.WithHeader("Pick a repo"), fzf.WithReverse())
	if err != nil || selected == "" {
		return "", errs.Userf("no repo selected")
	}
	return selected, nil
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

func loadRepoConfig(repoName string) *config.Config {
	bareDir, err := config.ResolveRepo(repoName)
	if err != nil {
		return config.DefaultConfig()
	}
	return config.Load(bareDir)
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

			// Metadata header
			fmt.Printf("\033[0;34m\u2500\u2500 %s/%s \u2500\u2500\033[0m\n\n", repoName, wtDir)
			printPreviewMetadata(wtPath, repoName)

			if !tmux.SessionExists(sessName) {
				fmt.Printf("\033[2mSession '%s' is offline.\033[0m\n\n", sessName)
				fmt.Println("Press Enter to start the session.")
				return nil
			}

			fmt.Printf("\033[0;34m\u2500\u2500 tmux pane \u2500\u2500\033[0m\n\n")

			content, err := tmux.CapturePane(sessName)
			if err != nil {
				fmt.Printf("\033[2mCould not capture pane: %v\033[0m\n", err)
				return nil
			}
			fmt.Print(content)
			return nil
		},
	}
}

func printPreviewMetadata(wtPath, repoName string) {
	g := &git.Git{Dir: wtPath}
	repoCfg := loadRepoConfig(repoName)
	baseBranch := repoCfg.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	branch := ""
	if b, err := g.Run("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		branch = b
		fmt.Printf("  \033[1mBranch:\033[0m  %s\n", branch)
	}

	// Show stack chain if branch is stacked
	bareDir, _ := config.ResolveRepo(repoName)
	if bareDir != "" {
		st := stack.Load(bareDir)
		if st.IsTracked(branch) {
			chain := buildStackChain(st, branch)
			fmt.Printf("  \033[1mStack:\033[0m   %s\n", chain)
		}
	}

	if diffOut, err := g.Run("diff", "--shortstat", fmt.Sprintf("origin/%s...HEAD", baseBranch)); err == nil {
		stats := dashboard.ParseShortstat(diffOut)
		fmt.Printf("  \033[1mDiff:\033[0m    %s\n", stats)
	}

	if lastCommit, err := g.Run("log", "-1", "--format=%s (%cr)", "--no-walk"); err == nil {
		fmt.Printf("  \033[1mCommit:\033[0m  %s\n", lastCommit)
	}

	if ghPath, err := exec.LookPath("gh"); err == nil {
		prCmd := exec.Command(ghPath, "pr", "view", "--json", "state,title", "-q", ".state + \" \\u2014 \" + .title")
		prCmd.Dir = wtPath
		if prOut, err := prCmd.Output(); err == nil {
			prInfo := strings.TrimSpace(string(prOut))
			if prInfo != "" {
				fmt.Printf("  \033[1mPR:\033[0m      %s\n", prInfo)
			}
		}
	}
	fmt.Println()
}

// buildStackChain returns a display string like "main → feature-a → [feature-b] → feature-c"
func buildStackChain(st *stack.Stack, current string) string {
	// Walk up to find the root
	var ancestors []string
	b := current
	for {
		parent := st.Parent(b)
		if parent == "" {
			break
		}
		ancestors = append([]string{parent}, ancestors...)
		if !st.IsTracked(parent) {
			break
		}
		b = parent
	}

	// Walk down to find descendants
	var descendants []string
	queue := st.Children(current)
	for len(queue) > 0 {
		child := queue[0]
		queue = queue[1:]
		descendants = append(descendants, child)
		queue = append(queue, st.Children(child)...)
	}

	var parts []string
	for _, a := range ancestors {
		parts = append(parts, a)
	}
	parts = append(parts, fmt.Sprintf("\033[1m[%s]\033[0m", current))
	for _, d := range descendants {
		parts = append(parts, d)
	}

	return strings.Join(parts, " → ")
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
			currentStatuses := make(map[string]claude.Status)

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
					sessions := claude.ReadAllSessions(repoName, wtDir)
					ws := claude.AggregateStatus(sessions)

					// Self-heal: clean orphaned BUSY/WAIT sessions when tmux session is gone
					sessName := tmux.SessionNameForWorktree(repoName, wtDir)
					if (ws.Status == claude.StatusBusy || ws.Status == claude.StatusWait) && !tmux.SessionExists(sessName) {
						for _, ss := range sessions {
							if ss.Status == claude.StatusBusy || ss.Status == claude.StatusWait {
								claude.RemoveSessionFile(repoName, wtDir, ss.SessionID)
							}
						}
						ws = claude.AggregateStatus(claude.ReadAllSessions(repoName, wtDir))
					}

					currentStatuses[repoName+"/"+wtDir] = ws.Status
					if ws.Status == claude.StatusBusy || ws.Status == claude.StatusWait || ws.Status == claude.StatusDone {
						activeAgents++
					}
				}
			}

			transitions := tmux.CheckTransitions(currentStatuses)
			if len(transitions) > 0 {
				cfg := config.Load("")
				if cfg.Tmux.Notification == nil || *cfg.Tmux.Notification {
					tmux.NotifyWithContext(transitions, cfg)
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
