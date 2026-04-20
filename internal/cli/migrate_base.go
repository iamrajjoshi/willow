package cli

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	ierrors "github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

type migrateRepo struct {
	Name         string
	OldBareDir   string
	NewBareDir   string
	OldWorktrees []string
	NewWorktrees []string
	HadStack     bool
}

type migrateBasePlan struct {
	SourceBase   string
	DestBase     string
	Repos        []migrateRepo
	HadRepos     bool
	HadWorktrees bool
	HadStatus    bool
	HadLog       bool
	HadTrash     bool
	EnvOverride  string
}

func migrateBaseCmd() *cli.Command {
	return &cli.Command{
		Name:  "migrate-base",
		Usage: "Move willow's base directory to a new path",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "path",
				UsageText: "<new-path>",
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show the planned move without changing anything",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.migrate-base")()
			flags := parseFlags(cmd)
			u := flags.NewUI()

			destArg := cmd.StringArg("path")
			if destArg == "" {
				return ierrors.Userf("destination path is required\n\nUsage: ww migrate-base <new-path>")
			}

			plan, err := buildMigrateBasePlan(config.WillowHome(), config.NormalizeBaseDir(destArg), flags.Verbose)
			if err != nil {
				return err
			}

			printMigrateBasePlan(u, plan)
			if plan.EnvOverride != "" {
				u.Warn("WILLOW_BASE_DIR is set in your shell. Willow will keep using it until you update or unset it.")
			}

			if cmd.Bool("dry-run") {
				return nil
			}

			if !cmd.Bool("yes") && !u.Confirm(fmt.Sprintf("Move willow base from %s to %s?", plan.SourceBase, plan.DestBase)) {
				u.Info("Migration cancelled.")
				return nil
			}

			if err := executeMigrateBasePlan(plan, flags.Verbose); err != nil {
				return err
			}

			u.Success(fmt.Sprintf("Moved willow base to %s", plan.DestBase))
			if plan.EnvOverride != "" {
				u.Warn("Update or unset WILLOW_BASE_DIR before running willow again.")
			}
			return nil
		},
	}
}

func buildMigrateBasePlan(sourceBase, destBase string, verbose bool) (*migrateBasePlan, error) {
	sourceBase = config.NormalizeBaseDir(sourceBase)
	destBase = config.NormalizeBaseDir(destBase)

	if sourceBase == "" {
		return nil, ierrors.Userf("could not resolve the current willow base directory")
	}
	if destBase == "" {
		return nil, ierrors.Userf("destination path is required")
	}
	if sourceBase == destBase {
		return nil, ierrors.Userf("destination matches the current willow base: %s", sourceBase)
	}
	if pathsOverlap(sourceBase, destBase) {
		return nil, ierrors.Userf("source and destination overlap: %s ↔ %s", sourceBase, destBase)
	}

	sourceInfo, err := os.Stat(sourceBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ierrors.Userf("willow base not found at %s", sourceBase)
		}
		return nil, err
	}
	if !sourceInfo.IsDir() {
		return nil, ierrors.Userf("willow base is not a directory: %s", sourceBase)
	}

	if err := rejectCwdInsideBase(sourceBase); err != nil {
		return nil, err
	}
	if err := rejectNonEmptyDestination(destBase); err != nil {
		return nil, err
	}
	if err := rejectActiveSessions(); err != nil {
		return nil, err
	}

	repos, err := discoverMigrateRepos(sourceBase, destBase, verbose)
	if err != nil {
		return nil, err
	}

	return &migrateBasePlan{
		SourceBase:   sourceBase,
		DestBase:     destBase,
		Repos:        repos,
		HadRepos:     pathExists(filepath.Join(sourceBase, "repos")),
		HadWorktrees: pathExists(filepath.Join(sourceBase, "worktrees")),
		HadStatus:    pathExists(filepath.Join(sourceBase, "status")),
		HadLog:       pathExists(filepath.Join(sourceBase, "log")),
		HadTrash:     pathExists(filepath.Join(sourceBase, "trash")),
		EnvOverride:  os.Getenv("WILLOW_BASE_DIR"),
	}, nil
}

func printMigrateBasePlan(u interface{ Info(string) }, plan *migrateBasePlan) {
	worktreeCount := 0
	for _, repo := range plan.Repos {
		worktreeCount += len(repo.NewWorktrees)
	}

	u.Info(fmt.Sprintf("Current base: %s", plan.SourceBase))
	u.Info(fmt.Sprintf("New base:     %s", plan.DestBase))
	u.Info(fmt.Sprintf("Repos:        %d", len(plan.Repos)))
	u.Info(fmt.Sprintf("Worktrees:    %d", worktreeCount))
	if len(plan.Repos) > 0 {
		u.Info("")
		for _, repo := range plan.Repos {
			u.Info(fmt.Sprintf("  %s", repo.Name))
			u.Info(fmt.Sprintf("    bare: %s", repo.NewBareDir))
			for _, wt := range repo.NewWorktrees {
				u.Info(fmt.Sprintf("    wt:   %s", wt))
			}
		}
	}
}

func rejectCwdInsideBase(sourceBase string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if pathWithin(sourceBase, cwd) {
		return ierrors.Userf("cannot migrate while your current directory is inside the willow base: %s", cwd)
	}
	return nil
}

func rejectNonEmptyDestination(destBase string) error {
	info, err := os.Stat(destBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return ierrors.Userf("destination exists and is not a directory: %s", destBase)
	}
	empty, err := dirEmpty(destBase)
	if err != nil {
		return err
	}
	if !empty {
		return ierrors.Userf("destination already exists and is not empty: %s", destBase)
	}
	return nil
}

func rejectActiveSessions() error {
	sessions, err := claude.ScanAllSessions()
	if err != nil {
		return err
	}

	var active []string
	for _, session := range sessions {
		if !claude.IsActive(session.Session.Status) {
			continue
		}
		active = append(active, fmt.Sprintf("  %s/%s %s (%s)",
			session.RepoName,
			session.WorktreeDir,
			session.Session.SessionID,
			session.Session.Status,
		))
	}
	if len(active) == 0 {
		return nil
	}

	sort.Strings(active)
	return ierrors.Userf("cannot migrate while Claude status files are active:\n%s", strings.Join(active, "\n"))
}

func discoverMigrateRepos(sourceBase, destBase string, verbose bool) ([]migrateRepo, error) {
	reposDir := filepath.Join(sourceBase, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var repos []migrateRepo
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".git") {
			continue
		}

		oldBareDir := filepath.Join(reposDir, entry.Name())
		name := strings.TrimSuffix(entry.Name(), ".git")
		repoGit := &git.Git{Dir: oldBareDir, Verbose: verbose}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return nil, fmt.Errorf("list worktrees for %s: %w", name, err)
		}

		repo := migrateRepo{
			Name:       name,
			OldBareDir: oldBareDir,
			NewBareDir: relocatePath(oldBareDir, sourceBase, destBase),
			HadStack:   pathExists(filepath.Join(oldBareDir, "branches.json")),
		}
		for _, wt := range filterBareWorktrees(wts) {
			repo.OldWorktrees = append(repo.OldWorktrees, wt.Path)
			repo.NewWorktrees = append(repo.NewWorktrees, relocatePath(wt.Path, sourceBase, destBase))
		}
		repos = append(repos, repo)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}

func executeMigrateBasePlan(plan *migrateBasePlan, verbose bool) error {
	if err := moveWillowBase(plan.SourceBase, plan.DestBase); err != nil {
		return err
	}

	if err := saveMigratedBaseDir(plan.DestBase); err != nil {
		return fmt.Errorf("willow base moved to %s, but updating global config failed: %w", plan.DestBase, err)
	}

	var failures []string
	for _, repo := range plan.Repos {
		if len(repo.NewWorktrees) == 0 {
			continue
		}
		repoGit := &git.Git{Dir: repo.NewBareDir, Verbose: verbose}
		args := append([]string{"worktree", "repair"}, repo.NewWorktrees...)
		if _, err := repoGit.Run(args...); err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: git worktree repair failed: %v", repo.Name, err))
		}
	}

	failures = append(failures, validateMigrateBasePlan(plan, verbose)...)
	if len(failures) == 0 {
		return nil
	}

	return fmt.Errorf("moved willow base to %s, but some repos/worktrees need manual repair:\n%s",
		plan.DestBase,
		strings.Join(prefixLines(failures, "  "), "\n"),
	)
}

func validateMigrateBasePlan(plan *migrateBasePlan, verbose bool) []string {
	var failures []string

	requiredDirs := []struct {
		had  bool
		path string
	}{
		{had: plan.HadRepos, path: filepath.Join(plan.DestBase, "repos")},
		{had: plan.HadWorktrees, path: filepath.Join(plan.DestBase, "worktrees")},
	}
	for _, required := range requiredDirs {
		if !required.had {
			continue
		}
		if !pathExists(required.path) {
			failures = append(failures, fmt.Sprintf("missing directory: %s", required.path))
		}
	}

	if plan.HadStatus && !pathExists(filepath.Join(plan.DestBase, "status")) {
		failures = append(failures, fmt.Sprintf("missing status directory: %s", filepath.Join(plan.DestBase, "status")))
	}
	if plan.HadLog && !pathExists(filepath.Join(plan.DestBase, "log")) {
		failures = append(failures, fmt.Sprintf("missing log directory: %s", filepath.Join(plan.DestBase, "log")))
	}
	if plan.HadTrash && !pathExists(filepath.Join(plan.DestBase, "trash")) {
		failures = append(failures, fmt.Sprintf("missing trash directory: %s", filepath.Join(plan.DestBase, "trash")))
	}

	for _, repo := range plan.Repos {
		repoGit := &git.Git{Dir: repo.NewBareDir, Verbose: verbose}
		if _, err := repoGit.Run("worktree", "list", "--porcelain"); err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: git worktree list failed: %v", repo.Name, err))
		}
		if repo.HadStack && !pathExists(filepath.Join(repo.NewBareDir, "branches.json")) {
			failures = append(failures, fmt.Sprintf("repo %s: missing branches.json", repo.Name))
		}
		for _, wtPath := range repo.NewWorktrees {
			wtGit := &git.Git{Dir: wtPath, Verbose: verbose}
			top, err := wtGit.Run("rev-parse", "--show-toplevel")
			if err != nil {
				failures = append(failures, fmt.Sprintf("worktree %s: git rev-parse failed: %v", wtPath, err))
				continue
			}
			if comparablePath(top) != comparablePath(wtPath) {
				failures = append(failures, fmt.Sprintf("worktree %s: git top-level resolved to %s", wtPath, top))
			}
		}
	}

	return failures
}

func saveMigratedBaseDir(destBase string) error {
	cfg, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cfg = &config.Config{}
	}

	if destBase == config.DefaultWillowHome() {
		cfg.BaseDir = ""
	} else {
		cfg.BaseDir = destBase
	}

	return config.Save(cfg, config.GlobalConfigPath())
}

func moveWillowBase(sourceBase, destBase string) error {
	if err := os.MkdirAll(filepath.Dir(destBase), 0o755); err != nil {
		return err
	}

	destExists := pathExists(destBase)
	if !destExists {
		if err := os.Rename(sourceBase, destBase); err == nil {
			return nil
		} else if !isCrossDeviceError(err) {
			return err
		}
	}

	if err := copyDir(sourceBase, destBase); err != nil {
		return err
	}
	return os.RemoveAll(sourceBase)
}

func isCrossDeviceError(err error) bool {
	var linkErr *os.LinkError
	return stderrors.Is(err, syscall.EXDEV) || (stderrors.As(err, &linkErr) && stderrors.Is(linkErr.Err, syscall.EXDEV))
}

func copyDir(sourceBase, destBase string) error {
	info, err := os.Stat(sourceBase)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", sourceBase)
	}
	if err := os.MkdirAll(destBase, info.Mode().Perm()); err != nil {
		return err
	}

	return filepath.Walk(sourceBase, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceBase, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(destBase, rel)
		mode := info.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
				return err
			}
			return os.Chmod(target, mode.Perm())
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		default:
			return copyFile(path, target, mode.Perm())
		}
	})
}

func copyFile(sourcePath, destPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dest, source); err != nil {
		dest.Close()
		return err
	}
	return dest.Close()
}

func relocatePath(path, sourceBase, destBase string) string {
	rel, err := filepath.Rel(comparablePath(sourceBase), comparablePath(path))
	if err != nil {
		return path
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.Join(destBase, rel)
}

func pathWithin(base, target string) bool {
	base = comparablePath(base)
	target = comparablePath(target)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "")
}

func comparablePath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func prefixLines(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, prefix+line)
	}
	return out
}
