package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

var errNotWillowRepo = errors.Userf("not inside a willow-managed repo\n\nRun this command from a willow-managed worktree, or use 'ww ls' to see your repos.")

func requireWillowRepo(g *git.Git) (string, error) {
	bareDir, err := g.BareRepoDir()
	if err == nil && config.IsWillowRepo(bareDir) {
		return bareDir, nil
	}

	if bareDir, ok := resolveRepoFromCwd(); ok {
		return bareDir, nil
	}

	return "", errNotWillowRepo
}

func resolveRepoFromCwd() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return config.ResolveRepoFromDir(cwd)
}

func resolveRepoFromGitMetadataCwd() (bareDir string, isWillow bool, foundGit bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, false
	}

	for {
		if commonDir, ok := commonDirFromGitMetadata(cwd); ok {
			return commonDir, config.IsWillowRepo(commonDir), true
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", false, false
		}
		cwd = parent
	}
}

func commonDirFromGitMetadata(dir string) (string, bool) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}

	gitDir := gitPath
	if !info.IsDir() {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return "", false
		}
		line := strings.TrimSpace(string(data))
		rawGitDir, ok := strings.CutPrefix(line, "gitdir:")
		if !ok {
			return "", false
		}
		gitDir = strings.TrimSpace(rawGitDir)
		if gitDir == "" {
			return "", false
		}
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
	}

	commonDir := gitDir
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			commonDir = value
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(gitDir, commonDir)
			}
		}
	}
	return filepath.Clean(commonDir), true
}

var version = "dev"

func Version() string { return version }

type Flags struct {
	Verbose bool
	Trace   bool
}

func parseFlags(cmd *cli.Command) Flags {
	return Flags{
		Verbose: cmd.Root().Bool("verbose"),
		Trace:   cmd.Root().Bool("trace"),
	}
}

func (f Flags) NewGit() *git.Git {
	return &git.Git{Verbose: f.Verbose}
}

func (f Flags) NewUI() *ui.UI {
	return &ui.UI{}
}

func NewApp() *cli.Command {
	return &cli.Command{
		Name:                  "willow",
		Usage:                 "A simple, opinionated git worktree manager",
		Version:               version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "C",
				Usage:   "Run as if willow was started in `PATH`",
				Sources: cli.EnvVars("WILLOW_DIR"),
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Show git commands being executed",
			},
			&cli.BoolFlag{
				Name:    "trace",
				Usage:   "Print timing trace to stderr for performance debugging",
				Sources: cli.EnvVars("WILLOW_TRACE"),
			},
		},
		Commands: []*cli.Command{
			cloneCmd(),
			newCmd(),
			promoteCmd(),
			renameCmd(),
			checkoutCmd(),
			syncCmd(),
			prCmd(),
			swCmd(),
			rmCmd(),
			lsCmd(),
			statusCmd(),
			dashboardCmd(),
			logCmd(),
			dispatchCmd(),
			tmuxCmd(),
			agentCmd(),
			setupCmd(),
			codexSetupCmd(),
			cursorSetupCmd(),
			hookCmd(),
			focusCmd(),
			shellInitCmd(),
			gcCmd(),
			migrateBaseCmd(),
			doctorCmd(),
			configCmd(),
			stackCmd(),
			refreshStatusCmd(),
		},
	}
}
