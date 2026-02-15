package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

var configKeys = []string{
	"baseBranch",
	"branchPrefix",
	"postCheckoutHook",
	"setup",
	"teardown",
	"defaults.fetch",
	"defaults.autoSetupRemote",
}

func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "View or edit configuration",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "key",
				UsageText: "[key]",
			},
			&cli.StringArg{
				Name:      "value",
				UsageText: "[value]",
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "global",
				Usage: "Target global config",
			},
			&cli.BoolFlag{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "List all config values with sources",
			},
			&cli.BoolFlag{
				Name:  "edit",
				Usage: "Open config file in $EDITOR",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			if cmd.Bool("list") {
				return configList(g, u)
			}

			if cmd.Bool("edit") {
				return configEdit(cmd, g)
			}

			key := cmd.StringArg("key")
			value := cmd.StringArg("value")

			if key == "" {
				return configList(g, u)
			}

			if value != "" {
				return configSet(cmd, g, u, key, value)
			}

			return configGet(g, u, key)
		},
	}
}

func configList(g *git.Git, u *ui.UI) error {
	bareDir, _ := g.BareRepoDir()
	if bareDir == "" {
		bareDir, _ = resolveRepoFromCwd()
	}
	worktreeRoot, _ := g.WorktreeRoot()

	type tier struct {
		name string
		path string
		cfg  *config.Config
	}

	tiers := []tier{
		{name: "global", path: config.GlobalConfigPath()},
	}

	if worktreeRoot != "" {
		tiers = append(tiers, tier{
			name: "shared",
			path: config.SharedConfigPath(worktreeRoot),
		})
	}

	if bareDir != "" {
		tiers = append(tiers, tier{
			name: "local",
			path: config.LocalConfigPath(bareDir),
		})
	}

	for i := range tiers {
		cfg, _ := config.LoadFile(tiers[i].path)
		tiers[i].cfg = cfg
	}

	for _, key := range configKeys {
		source := "default"
		value := getDefaultValue(key)

		for _, t := range tiers {
			if t.cfg == nil {
				continue
			}
			if v, ok := getFieldValue(t.cfg, key); ok {
				value = v
				source = t.name
			}
		}

		fmt.Printf("%s = %s %s\n", key, value, u.Dim("("+source+")"))
	}
	return nil
}

func configGet(g *git.Git, u *ui.UI, key string) error {
	if !isValidKey(key) {
		return fmt.Errorf("unknown config key: %s\n\nValid keys: %s", key, strings.Join(configKeys, ", "))
	}

	bareDir, _ := g.BareRepoDir()
	if bareDir == "" {
		bareDir, _ = resolveRepoFromCwd()
	}
	worktreeRoot, _ := g.WorktreeRoot()
	cfg := config.Load(bareDir, worktreeRoot)

	value, _ := getFieldValue(cfg, key)
	fmt.Println(value)
	return nil
}

func configSet(cmd *cli.Command, g *git.Git, u *ui.UI, key, value string) error {
	if !isValidKey(key) {
		return fmt.Errorf("unknown config key: %s\n\nValid keys: %s", key, strings.Join(configKeys, ", "))
	}

	var configPath string
	if cmd.Bool("global") {
		configPath = config.GlobalConfigPath()
	} else {
		bareDir, err := requireWillowRepo(g)
		if err != nil {
			return fmt.Errorf("not inside a willow-managed repo (use --global for global config)")
		}
		configPath = config.LocalConfigPath(bareDir)
	}

	cfg, _ := config.LoadFile(configPath)
	if cfg == nil {
		cfg = &config.Config{}
	}

	if err := setFieldValue(cfg, key, value); err != nil {
		return err
	}

	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	u.Success(fmt.Sprintf("%s = %s", key, value))
	return nil
}

func configEdit(cmd *cli.Command, g *git.Git) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	var configPath string
	if cmd.Bool("global") {
		configPath = config.GlobalConfigPath()
	} else {
		bareDir, err := requireWillowRepo(g)
		if err != nil {
			return fmt.Errorf("not inside a willow-managed repo (use --global for global config)")
		}
		configPath = config.LocalConfigPath(bareDir)
	}

	// Create the file with defaults if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := config.Save(&config.Config{}, configPath); err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	}

	e := exec.Command(editor, configPath)
	e.Stdin = os.Stdin
	e.Stdout = os.Stdout
	e.Stderr = os.Stderr
	return e.Run()
}

func getDefaultValue(key string) string {
	switch key {
	case "defaults.fetch":
		return "true"
	case "defaults.autoSetupRemote":
		return "true"
	default:
		return ""
	}
}

func getFieldValue(cfg *config.Config, key string) (string, bool) {
	switch key {
	case "baseBranch":
		if cfg.BaseBranch != "" {
			return cfg.BaseBranch, true
		}
	case "branchPrefix":
		if cfg.BranchPrefix != "" {
			return cfg.BranchPrefix, true
		}
	case "postCheckoutHook":
		if cfg.PostCheckoutHook != "" {
			return cfg.PostCheckoutHook, true
		}
	case "setup":
		if cfg.Setup != nil {
			return strings.Join(cfg.Setup, ", "), true
		}
	case "teardown":
		if cfg.Teardown != nil {
			return strings.Join(cfg.Teardown, ", "), true
		}
	case "defaults.fetch":
		if cfg.Defaults.Fetch != nil {
			return fmt.Sprintf("%t", *cfg.Defaults.Fetch), true
		}
	case "defaults.autoSetupRemote":
		if cfg.Defaults.AutoSetupRemote != nil {
			return fmt.Sprintf("%t", *cfg.Defaults.AutoSetupRemote), true
		}
	}
	return "", false
}

func setFieldValue(cfg *config.Config, key, value string) error {
	switch key {
	case "baseBranch":
		cfg.BaseBranch = value
	case "branchPrefix":
		cfg.BranchPrefix = value
	case "postCheckoutHook":
		cfg.PostCheckoutHook = value
	case "setup":
		cfg.Setup = splitCommands(value)
	case "teardown":
		cfg.Teardown = splitCommands(value)
	case "defaults.fetch":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		cfg.Defaults.Fetch = config.BoolPtr(b)
	case "defaults.autoSetupRemote":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		cfg.Defaults.AutoSetupRemote = config.BoolPtr(b)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func isValidKey(key string) bool {
	for _, k := range configKeys {
		if k == key {
			return true
		}
	}
	return false
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("must be true or false, got %q", s)
	}
}
