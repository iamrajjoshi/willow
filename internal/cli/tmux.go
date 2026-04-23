package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func tmuxCmd() *cli.Command {
	return &cli.Command{
		Name:  "tmux",
		Usage: "Tmux integration for worktree management",
		Commands: []*cli.Command{
			tmuxPickCmd(),
			tmuxExistingBranchesCmd(),
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
			&cli.StringFlag{
				Name:    "session",
				Aliases: []string{"s"},
				Usage:   "Current tmux session name (passed by run-shell)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.pick")()
			repoFilter := cmd.String("repo")
			sessionName := cmd.String("session")
			self, err := os.Executable()
			if err != nil {
				self = "willow"
			}

			for {
				items, err := tmux.BuildPickerItemsWithOptions(ctx, repoFilter, tmux.PickerBuildOptions{})
				if err != nil {
					return err
				}
				if len(items) == 0 {
					fmt.Fprintln(os.Stderr, "No worktrees found.")
					return nil
				}

				lines := tmux.FormatPickerLines(items)

				curSess := sessionName
				if curSess == "" {
					curSess, _ = tmux.CurrentSession()
				}
				if curSess != "" {
					items = moveToFront(items, curSess)
					lines = tmux.FormatPickerLines(items)
				}

				previewCmd := fmt.Sprintf("%s tmux preview -- {}", shellQuote(self))
				opts := []fzf.Option{
					fzf.WithAnsi(),
					fzf.WithReverse(),
					fzf.WithNoSort(),
					fzf.WithDelimiter("\\|"),
					fzf.WithNth("1,2"),
					fzf.WithHeader("Enter: Switch | Ctrl-N: New | Ctrl-B: Stacked | Ctrl-E: Existing | Ctrl-P: PR | Ctrl-G: Dispatch | Ctrl-S: Sync | Ctrl-D: Delete | Ctrl-X: Prune merged"),
					fzf.WithExpectKeys("ctrl-n", "ctrl-b", "ctrl-e", "ctrl-p", "ctrl-g", "ctrl-s", "ctrl-d", "ctrl-x"),
					fzf.WithPrintQuery(),
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
					if err := tmuxPickNew(self, result.Query, repoFilter, sessionName, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-b":
					if err := tmuxPickNewWithBase(self, result.Query, repoFilter, sessionName, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-e":
					if err := tmuxPickExisting(self, repoFilter, sessionName, items, result.Query); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-p":
					if err := tmuxPickPR(self, repoFilter, sessionName, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-g":
					if err := tmuxPickDispatch(self, result.Query, repoFilter, sessionName, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
					return nil

				case "ctrl-s":
					branch := ""
					if result.Selection != "" {
						wtPath := tmux.ExtractPathFromLine(result.Selection)
						if item := findItemByPath(items, wtPath); item != nil {
							branch = item.Branch
						}
					}
					if err := tmuxPickSync(self, repoFilter, sessionName, items, branch); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					}
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

				case "ctrl-x":
					if err := tmuxPickDeleteMerged(self, curSess, items); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					}
					fmt.Fprintf(os.Stderr, "\nPress Enter to return to picker...\n")
					fmt.Fscanln(os.Stdin)
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.sw")()
			wtPath := cmd.StringArg("path")
			if wtPath == "" {
				return errors.Userf("worktree path is required")
			}

			wtDir := filepath.Base(wtPath)
			repoName := filepath.Base(filepath.Dir(wtPath))

			claude.MarkRead(repoName, wtDir)
			return ensureTmuxSession(repoName, wtDir, wtPath)
		},
	}
}

func tmuxExistingBranchesCmd() *cli.Command {
	return &cli.Command{
		Name:   "existing-branches",
		Usage:  "Print existing remote branches for tmux picker reloads",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "refresh",
				Usage: "Fetch origin before listing branches",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.existing-branches")()
			repo := cmd.String("repo")
			if repo == "" {
				return errors.Userf("repo is required")
			}

			branches, err := existingBranchesForRepo(repo, cmd.Bool("refresh"))
			if err != nil {
				return err
			}
			for _, branch := range branches {
				fmt.Println(branch)
			}
			return nil
		},
	}
}

func tmuxPickSwitch(selection string, items []tmux.PickerItem) error {
	wtPath := tmux.ExtractPathFromLine(selection)
	item := findItemByPath(items, wtPath)
	if item == nil {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	claude.MarkRead(item.RepoName, item.WtDirName)
	return ensureTmuxSession(item.RepoName, item.WtDirName, item.WtPath)
}

func tmuxPickNew(self, query, repoFilter, sessionName string, items []tmux.PickerItem) error {
	if query == "" {
		return errors.Userf("enter a branch name first")
	}

	repo, err := resolveRepo(repoFilter, sessionName, items)
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

	return ensureTmuxSessionFromPath(wtPath)
}

func tmuxPickNewWithBase(self, query, repoFilter, sessionName string, items []tmux.PickerItem) error {
	if query == "" {
		return errors.Userf("enter a branch name first")
	}

	repo, err := resolveRepo(repoFilter, sessionName, items)
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
		return errors.Userf("no worktrees to use as base")
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

	return ensureTmuxSessionFromPath(wtPath)
}

func tmuxPickExisting(self, repoFilter, sessionName string, items []tmux.PickerItem, query string) error {
	repo, err := resolveRepo(repoFilter, sessionName, items)
	if err != nil {
		return err
	}

	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return err
	}

	repoGit := &git.Git{Dir: bareDir}
	cfg := config.Load(bareDir)
	remoteBranches, err := loadExistingBranchCache(repo)
	if err != nil {
		remoteBranches = nil
	}
	if len(remoteBranches) == 0 {
		remoteBranches, err = repoGit.RemoteBranches()
		if err != nil {
			return fmt.Errorf("failed to list remote branches: %w", err)
		}
		_ = saveExistingBranchCache(repo, remoteBranches)
	}
	if len(remoteBranches) == 0 {
		return errors.Userf("no remote branches found")
	}

	available := availableExistingBranches(remoteBranches, repo, items)
	if len(available) == 0 && *cfg.Defaults.Fetch {
		available, err = existingBranchesForRepo(repo, true)
		if err != nil {
			return err
		}
	}
	if len(available) == 0 {
		return errors.Userf("all remote branches already have worktrees")
	}

	opts := []fzf.Option{
		fzf.WithReverse(),
		fzf.WithHeader("Select a branch to check out"),
	}
	if strings.TrimSpace(query) != "" {
		opts = append(opts, fzf.WithQuery(query))
	}
	if *cfg.Defaults.Fetch {
		refreshCmd := fmt.Sprintf("%s tmux existing-branches --repo %s --refresh", shellQuote(self), shellQuote(repo))
		opts = append(opts,
			fzf.WithHeader("Select a branch to check out (refreshing in background; Ctrl-R to refresh)"),
			fzf.WithBind(fmt.Sprintf("start:reload(%s)", refreshCmd)),
			fzf.WithBind(fmt.Sprintf("ctrl-r:reload-sync(%s)", refreshCmd)),
		)
	}

	branch, err := fzf.Run(available, opts...)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil // user cancelled
	}

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

	return ensureTmuxSessionFromPath(wtPath)
}

func existingBranchesForRepo(repo string, refresh bool) ([]string, error) {
	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return nil, err
	}

	repoGit := &git.Git{Dir: bareDir}
	if refresh {
		_, _ = repoGit.Run("fetch", "origin")
	}

	remoteBranches, err := repoGit.RemoteBranches()
	if err != nil {
		cached, cacheErr := loadExistingBranchCache(repo)
		if cacheErr == nil {
			remoteBranches = cached
		} else {
			return nil, fmt.Errorf("failed to list remote branches: %w", err)
		}
	}
	_ = saveExistingBranchCache(repo, remoteBranches)

	wtBranches, err := currentWorktreeBranchSet(repoGit)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	return filterAvailableBranches(remoteBranches, wtBranches), nil
}

func availableExistingBranches(remoteBranches []string, repo string, items []tmux.PickerItem) []string {
	wtBranches := make(map[string]bool)
	for _, item := range items {
		if item.RepoName != repo {
			continue
		}
		wtBranches[item.Branch] = true
	}
	return filterAvailableBranches(remoteBranches, wtBranches)
}

func currentWorktreeBranchSet(repoGit *git.Git) (map[string]bool, error) {
	wts, err := worktree.List(repoGit)
	if err != nil {
		return nil, err
	}

	wtBranches := make(map[string]bool)
	for _, wt := range wts {
		if wt.IsBare {
			continue
		}
		wtBranches[wt.Branch] = true
	}
	return wtBranches, nil
}

func filterAvailableBranches(remoteBranches []string, wtBranches map[string]bool) []string {
	available := make([]string, 0, len(remoteBranches))
	for _, branch := range remoteBranches {
		if !wtBranches[branch] {
			available = append(available, branch)
		}
	}
	return available
}

func existingBranchesCachePath(repo string) string {
	return filepath.Join(config.WillowHome(), "cache", "existing-branches", repo+".txt")
}

func loadExistingBranchCache(repo string) ([]string, error) {
	data, err := os.ReadFile(existingBranchesCachePath(repo))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func saveExistingBranchCache(repo string, branches []string) error {
	path := existingBranchesCachePath(repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data := strings.Join(branches, "\n")
	if data != "" {
		data += "\n"
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func tmuxPickPR(self, repoFilter, sessionName string, items []tmux.PickerItem) error {
	repo, err := resolveRepo(repoFilter, sessionName, items)
	if err != nil {
		return err
	}

	bareDir, err := config.ResolveRepo(repo)
	if err != nil {
		return err
	}

	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return errors.Userf("gh CLI is required for PR picker")
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
		return errors.Userf("no open PRs found")
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

	return ensureTmuxSessionFromPath(wtPath)
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

func tmuxPickDispatch(self, query, repoFilter, sessionName string, items []tmux.PickerItem) error {
	if query == "" {
		return errors.Userf("type a prompt first")
	}

	repo, err := resolveRepo(repoFilter, sessionName, items)
	if err != nil {
		return err
	}

	branch := "dispatch--" + slugify(query)
	args := []string{"new", "--cd", "--repo", repo, "--", branch}

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

	meta := map[string]string{"prompt": truncatePrompt(query)}
	_ = log.Append(log.Event{Action: "dispatch", Repo: repo, Branch: branch, Metadata: meta})

	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))

	sessName := tmux.SessionNameForWorktree(repoName, wtDir)

	promptFile := filepath.Join(config.WillowHome(), "prompts", repoName, wtDir+".prompt")
	if err := os.MkdirAll(filepath.Dir(promptFile), 0o755); err != nil {
		return fmt.Errorf("failed to create prompts dir: %w", err)
	}
	if err := os.WriteFile(promptFile, []byte(query), 0o644); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	cfg := loadRepoConfig(repoName)
	if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	claudeCmd := fmt.Sprintf(`claude "$(cat %s)"; rm -f %s`, shellQuote(promptFile), shellQuote(promptFile))
	if err := tmux.SendKeys(sessName, claudeCmd, "Enter"); err != nil {
		return fmt.Errorf("failed to send claude command: %w", err)
	}

	return tmux.SwitchClient(sessName)
}

func tmuxPickSync(self, repoFilter, sessionName string, items []tmux.PickerItem, branch string) error {
	repo, err := resolveRepo(repoFilter, sessionName, items)
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

func resolveRepo(repoFilter, sessionName string, items []tmux.PickerItem) (string, error) {
	if repoFilter != "" {
		return repoFilter, nil
	}

	repos, err := config.ListRepos()
	if err != nil || len(repos) == 0 {
		return "", errors.Userf("no repos found — run 'ww clone' first")
	}

	if len(repos) == 1 {
		return repos[0], nil
	}

	curSess := sessionName
	if curSess == "" {
		curSess, _ = tmux.CurrentSession()
	}
	currentRepo := ""
	if parts := strings.SplitN(curSess, "/", 2); len(parts) == 2 {
		currentRepo = parts[0]
	}
	activeRepos := make(map[string]bool)
	for _, item := range items {
		if claude.IsActive(item.Status) {
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
		return "", errors.Userf("no repo selected")
	}
	return selected, nil
}

func tmuxPickDelete(self, selection string, items []tmux.PickerItem) error {
	wtPath := tmux.ExtractPathFromLine(selection)
	item := findItemByPath(items, wtPath)
	if item == nil {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	return tmuxPickDeleteItem(self, *item)
}

func tmuxPickDeleteMerged(self, currentSession string, items []tmux.PickerItem) error {
	candidates, skippedCurrent := mergedDeleteCandidates(items, currentSession)
	if len(candidates) == 0 {
		if skippedCurrent {
			return errors.Userf("no merged worktrees available to delete (current session skipped)")
		}
		return errors.Userf("no merged worktrees available to delete")
	}

	safe, skipped, err := filterMergedDeleteCandidates(candidates)
	if err != nil {
		return err
	}

	multiRepo := mergedDeleteHasMultipleRepos(items)
	if len(safe) == 0 {
		if skippedCurrent {
			fmt.Fprintln(os.Stderr, "Skipping the active tmux session's worktree.")
		}
		if len(skipped) > 0 {
			fmt.Fprintln(os.Stderr, "Skipping unsafe merged worktrees:")
			for _, skip := range skipped {
				fmt.Fprintf(os.Stderr, "  %s (%s)\n", mergedDeleteLabel(skip.Item, multiRepo), skip.Reason)
			}
		}
		return errors.Userf("no safe merged worktrees available to delete")
	}

	fmt.Fprintf(os.Stderr, "Remove %d merged worktree(s)?\n", len(safe))
	for _, item := range safe {
		fmt.Fprintf(os.Stderr, "  %s\n", mergedDeleteLabel(item, multiRepo))
	}
	if skippedCurrent {
		fmt.Fprintln(os.Stderr, "\nSkipping the active tmux session's worktree.")
	}
	if len(skipped) > 0 {
		fmt.Fprintln(os.Stderr, "\nSkipping unsafe merged worktrees:")
		for _, skip := range skipped {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", mergedDeleteLabel(skip.Item, multiRepo), skip.Reason)
		}
	}

	fmt.Fprint(os.Stderr, "\nProceed? [y/N] ")
	answer := readTrimmedStdinLine()
	if answer != "y" && answer != "Y" {
		fmt.Fprintln(os.Stderr, "Aborted.")
		return nil
	}

	failures := 0
	for _, item := range safe {
		fmt.Fprintf(os.Stderr, "Removing %s...\n", mergedDeleteLabel(item, multiRepo))
		if err := tmuxPickDeleteItem(self, item); err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", mergedDeleteLabel(item, multiRepo), err)
		}
	}
	if failures > 0 {
		return fmt.Errorf("failed to remove %d merged worktree(s)", failures)
	}

	fmt.Fprintf(os.Stderr, "Removed %d merged worktree(s).\n", len(safe))
	return nil
}

type mergedDeleteSkip struct {
	Item   tmux.PickerItem
	Reason string
}

func filterMergedDeleteCandidates(items []tmux.PickerItem) ([]tmux.PickerItem, []mergedDeleteSkip, error) {
	bareDirs := make(map[string]string)
	stacks := make(map[string]*stack.Stack)
	var safe []tmux.PickerItem
	var skipped []mergedDeleteSkip

	for _, item := range items {
		bareDir, ok := bareDirs[item.RepoName]
		if !ok {
			resolved, err := config.ResolveRepo(item.RepoName)
			if err != nil {
				return nil, nil, err
			}
			bareDir = resolved
			bareDirs[item.RepoName] = bareDir
			stacks[item.RepoName] = stack.Load(bareDir)
		}

		reason, err := mergedDeleteSkipReason(item, stacks[item.RepoName])
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			skipped = append(skipped, mergedDeleteSkip{Item: item, Reason: reason})
			continue
		}
		safe = append(safe, item)
	}

	return safe, skipped, nil
}

func mergedDeleteCandidates(items []tmux.PickerItem, currentSession string) ([]tmux.PickerItem, bool) {
	var candidates []tmux.PickerItem
	skippedCurrent := false
	for _, item := range items {
		if !item.Merged {
			continue
		}
		if currentSession != "" && tmux.SessionNameForWorktree(item.RepoName, item.WtDirName) == currentSession {
			skippedCurrent = true
			continue
		}
		candidates = append(candidates, item)
	}
	return candidates, skippedCurrent
}

func mergedDeleteSkipReason(item tmux.PickerItem, st *stack.Stack) (string, error) {
	var children []string
	if st != nil {
		children = st.Children(item.Branch)
	}

	wtGit := &git.Git{Dir: item.WtPath}
	dirty, err := wtGit.IsDirty()
	if err != nil {
		return "", err
	}

	unpushed, err := wtGit.HasUnpushedCommits()
	if err != nil {
		return "", err
	}

	return mergedDeleteSkipReasonFromState(children, dirty, unpushed), nil
}

func mergedDeleteSkipReasonFromState(children []string, dirty, unpushed bool) string {
	var reasons []string
	if len(children) > 0 {
		reasons = append(reasons, "stacked children: "+strings.Join(children, ", "))
	}
	if dirty {
		reasons = append(reasons, "uncommitted changes")
	}
	if unpushed {
		reasons = append(reasons, "unpushed commits")
	}
	return strings.Join(reasons, "; ")
}

func mergedDeleteLabel(item tmux.PickerItem, multiRepo bool) string {
	if multiRepo {
		return item.RepoName + "/" + item.Branch
	}
	return item.Branch
}

func mergedDeleteHasMultipleRepos(items []tmux.PickerItem) bool {
	repos := make(map[string]bool)
	for _, item := range items {
		repos[item.RepoName] = true
		if len(repos) > 1 {
			return true
		}
	}
	return false
}

func readTrimmedStdinLine() string {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(line)
}

func tmuxPickDeleteItem(self string, item tmux.PickerItem) error {
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

// ensureTmuxSession creates a tmux session for the worktree if it doesn't exist,
// then switches to it.
func ensureTmuxSession(repoName, wtDir, wtPath string) error {
	sessName := tmux.SessionNameForWorktree(repoName, wtDir)
	if !tmux.SessionExists(sessName) {
		cfg := loadRepoConfig(repoName)
		if err := tmux.NewSession(sessName, wtPath, cfg.Tmux.Layout, cfg.Tmux.Panes); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}
	return tmux.SwitchClient(sessName)
}

// ensureTmuxSessionFromPath derives repo/worktree names from a worktree path
// and calls ensureTmuxSession.
func ensureTmuxSessionFromPath(wtPath string) error {
	wtDir := filepath.Base(wtPath)
	repoName := filepath.Base(filepath.Dir(wtPath))
	return ensureTmuxSession(repoName, wtDir, wtPath)
}

// stackLoaderFunc loads a stack for a given repo name. Injected for testing.
type stackLoaderFunc func(repoName string) *stack.Stack

func defaultStackLoader(repoName string) *stack.Stack {
	bareDir, err := config.ResolveRepo(repoName)
	if err != nil {
		return nil
	}
	return stack.Load(bareDir)
}

func moveToFront(items []tmux.PickerItem, sessName string) []tmux.PickerItem {
	return moveToFrontWithStack(items, sessName, defaultStackLoader)
}

func moveToFrontWithStack(items []tmux.PickerItem, sessName string, loadStack stackLoaderFunc) []tmux.PickerItem {
	matchIdx := -1
	for i, item := range items {
		if tmux.SessionNameForWorktree(item.RepoName, item.WtDirName) == sessName {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return items
	}

	matched := items[matchIdx]

	st := loadStack(matched.RepoName)
	if st == nil || !st.IsTracked(matched.Branch) {
		return moveItemToFront(items, matchIdx)
	}

	root := matched.Branch
	for {
		parent := st.Parent(root)
		if parent == "" || !st.IsTracked(parent) {
			break
		}
		root = parent
	}

	treeBranches := make(map[string]bool)
	treeBranches[root] = true
	for _, d := range st.Descendants(root) {
		treeBranches[d] = true
	}

	var treeItems, rest []tmux.PickerItem
	for _, item := range items {
		if item.RepoName == matched.RepoName && treeBranches[item.Branch] {
			treeItems = append(treeItems, item)
		} else {
			rest = append(rest, item)
		}
	}

	result := make([]tmux.PickerItem, 0, len(items))
	result = append(result, treeItems...)
	result = append(result, rest...)
	return result
}

func moveItemToFront(items []tmux.PickerItem, idx int) []tmux.PickerItem {
	reordered := make([]tmux.PickerItem, 0, len(items))
	reordered = append(reordered, items[idx])
	reordered = append(reordered, items[:idx]...)
	reordered = append(reordered, items[idx+1:]...)
	return reordered
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.preview")()
			line := strings.Join(cmd.Args().Slice(), " ")
			if line == "" {
				return nil
			}

			wtPath := tmux.ExtractPathFromLine(line)
			wtDir := filepath.Base(wtPath)
			repoName := filepath.Base(filepath.Dir(wtPath))

			sessName := tmux.SessionNameForWorktree(repoName, wtDir)

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
	bareDir, _ := config.ResolveRepo(repoName)
	baseBranch := (&git.Git{Dir: bareDir}).ResolveBaseBranch(repoCfg.BaseBranch)

	branch := ""
	if b, err := g.Run("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		branch = b
		fmt.Printf("  \033[1mBranch:\033[0m  %s\n", branch)
	}

	if bareDir != "" {
		st := stack.Load(bareDir)
		if st.IsTracked(branch) {
			chain := buildStackChain(st, branch)
			fmt.Printf("  \033[1mStack:\033[0m   %s\n", chain)
		}
	}

	if diffOut, err := g.Run("diff", "--shortstat", fmt.Sprintf("origin/%s...HEAD", baseBranch)); err == nil {
		stats := git.ParseShortstat(diffOut)
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
			&cli.StringFlag{
				Name:    "session",
				Aliases: []string{"s"},
				Usage:   "Current tmux session (moves matching worktree to top)",
			},
			&cli.BoolFlag{
				Name:   "refresh-github-merged",
				Usage:  "Refresh GitHub-backed merged status instead of using cached results",
				Hidden: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.list")()
			items, err := tmux.BuildPickerItemsWithOptions(ctx, cmd.String("repo"), tmux.PickerBuildOptions{
				RefreshGitHubMerged: cmd.Bool("refresh-github-merged"),
			})
			if err != nil {
				return err
			}
			if sess := cmd.String("session"); sess != "" {
				items = moveToFront(items, sess)
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.status-bar")()
			repos, err := config.ListRepos()
			if err != nil {
				return nil
			}

			totalWt := 0
			activeAgents := 0
			currentStatuses := make(map[string]claude.Status)
			done := trace.Span(ctx, "tmux.ListSessions")
			sessionSet := tmux.ListSessions()
			done()

			for _, repoName := range repos {
				bareDir, err := config.ResolveRepo(repoName)
				if err != nil {
					continue
				}
				repoGit := &git.Git{Dir: bareDir}
				done := trace.Span(ctx, "worktree.List/"+repoName)
				wts, err := worktree.List(repoGit)
				done()
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

					// Clean orphaned sessions whose tmux session no longer exists
					sessName := tmux.SessionNameForWorktree(repoName, wtDir)
					if (ws.Status == claude.StatusBusy || ws.Status == claude.StatusWait) && !sessionSet[sessName] {
						for _, ss := range sessions {
							if ss.Status == claude.StatusBusy || ss.Status == claude.StatusWait {
								claude.RemoveSessionFile(repoName, wtDir, ss.SessionID)
							}
						}
						ws = claude.AggregateStatus(claude.ReadAllSessions(repoName, wtDir))
					}

					currentStatuses[repoName+"/"+wtDir] = ws.Status
					if claude.IsActive(ws.Status) {
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "tmux.install")()
			self, err := os.Executable()
			if err != nil {
				self = "willow"
			}

			fmt.Println("# Willow tmux integration")
			fmt.Println("# Add these lines to your tmux.conf:")
			fmt.Println()
			fmt.Printf("bind w run-shell -b 'tmux display-popup -E -w 90%% -h 80%% \"%s tmux pick --session #S\"'\n", self)
			fmt.Printf("set -g status-right '#(%s tmux status-bar) %%l:%%M %%a'\n", self)
			fmt.Println("set -g status-interval 3")
			return nil
		},
	}
}
