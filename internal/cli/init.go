package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/urfave/cli/v3"
)

func initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize config for a willow-managed repo",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "global",
				Usage: "Create global config at ~/.config/willow/config.json",
			},
			&cli.BoolFlag{
				Name:  "shared",
				Usage: "Create shared config tracked in the repo",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			var configPath string
			var bareDir string
			var err error

			if cmd.Bool("global") {
				configPath = config.GlobalConfigPath()
			} else {
				bareDir, err = g.BareRepoDir()
				if err != nil {
					return fmt.Errorf("not inside a willow-managed repo (use --global for global config)")
				}
				if cmd.Bool("shared") {
					wtRoot, err := g.WorktreeRoot()
					if err != nil {
						return fmt.Errorf("not inside a worktree: %w", err)
					}
					configPath = config.SharedConfigPath(wtRoot)
				} else {
					configPath = config.LocalConfigPath(bareDir)
				}
			}

			// Pre-fill from existing config file
			existing, _ := config.LoadFile(configPath)
			if existing == nil {
				existing = &config.Config{}
			}

			// Auto-detect default branch for the prompt default
			detectedBranch := ""
			if bareDir != "" {
				repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
				detectedBranch, _ = repoGit.DefaultBranch()
			}

			baseBranchDefault := existing.BaseBranch
			if baseBranchDefault == "" {
				baseBranchDefault = detectedBranch
			}

			cfg := &config.Config{}
			cfg.BaseBranch = prompt("Base branch", baseBranchDefault)
			cfg.BranchPrefix = prompt("Branch prefix (e.g. your-username)", existing.BranchPrefix)

			setupDefault := ""
			if len(existing.Setup) > 0 {
				setupDefault = strings.Join(existing.Setup, ", ")
			}
			if s := prompt("Setup command (run after creating worktree)", setupDefault); s != "" {
				cfg.Setup = splitCommands(s)
			}

			teardownDefault := ""
			if len(existing.Teardown) > 0 {
				teardownDefault = strings.Join(existing.Teardown, ", ")
			}
			if s := prompt("Teardown command (run before removing worktree)", teardownDefault); s != "" {
				cfg.Teardown = splitCommands(s)
			}

			if err := config.Save(cfg, configPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			u.Success(fmt.Sprintf("Config saved to %s", u.Dim(configPath)))
			return nil
		},
	}
}

func prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		val := strings.TrimSpace(scanner.Text())
		if val != "" {
			return val
		}
	}
	return defaultVal
}

func splitCommands(s string) []string {
	var cmds []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			cmds = append(cmds, c)
		}
	}
	return cmds
}
