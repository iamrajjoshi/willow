package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "View, edit, and initialize willow configuration",
		Commands: []*cli.Command{
			configShowCmd(),
			configEditCmd(),
			configInitCmd(),
		},
	}
}

func configShowCmd() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Show merged config with source annotations",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output raw JSON",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target repo by name",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.config.show")()
			flags := parseFlags(cmd)
			u := flags.NewUI()

			bareDir := ""
			if repoName := cmd.String("repo"); repoName != "" {
				dir, err := config.ResolveRepo(repoName)
				if err != nil {
					return err
				}
				bareDir = dir
			} else {
				g := flags.NewGit()
				dir, err := requireWillowRepo(g)
				if err == nil {
					bareDir = dir
				}
			}

			merged := config.Load(bareDir)

			if cmd.Bool("json") {
				data, err := json.MarshalIndent(merged, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal config: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			def := config.DefaultConfig()
			global, _ := config.LoadFile(config.GlobalConfigPath())
			if global == nil {
				global = &config.Config{}
			}
			var local *config.Config
			if bareDir != "" {
				local, _ = config.LoadFile(config.LocalConfigPath(bareDir))
			}
			if local == nil {
				local = &config.Config{}
			}

			printField("baseDir", formatStringValue(merged.BaseDir), baseDirSource(global.BaseDir))
			printField("baseBranch", formatStringValue(merged.BaseBranch), fieldSource(local.BaseBranch, global.BaseBranch, def.BaseBranch))
			printField("branchPrefix", formatStringValue(merged.BranchPrefix), fieldSource(local.BranchPrefix, global.BranchPrefix, def.BranchPrefix))
			printField("postCheckoutHook", formatStringValue(merged.PostCheckoutHook), fieldSource(local.PostCheckoutHook, global.PostCheckoutHook, def.PostCheckoutHook))
			printField("setup", formatStringSliceValue(merged.Setup), fieldSourceSlice(local.Setup, global.Setup, def.Setup))
			printField("teardown", formatStringSliceValue(merged.Teardown), fieldSourceSlice(local.Teardown, global.Teardown, def.Teardown))
			printField("defaults.fetch", formatBoolPtrValue(merged.Defaults.Fetch), fieldSourceBoolPtr(local.Defaults.Fetch, global.Defaults.Fetch, def.Defaults.Fetch))
			printField("defaults.autoSetupRemote", formatBoolPtrValue(merged.Defaults.AutoSetupRemote), fieldSourceBoolPtr(local.Defaults.AutoSetupRemote, global.Defaults.AutoSetupRemote, def.Defaults.AutoSetupRemote))
			printField("notify.desktop", formatBoolPtrValue(merged.Notify.Desktop), fieldSourceBoolPtr(local.Notify.Desktop, global.Notify.Desktop, def.Notify.Desktop))
			printField("notify.command", formatStringValue(merged.Notify.Command), fieldSource(local.Notify.Command, global.Notify.Command, def.Notify.Command))
			printField("agent.default", formatStringValue(merged.Agent.Default), fieldSource(local.Agent.Default, global.Agent.Default, def.Agent.Default))
			printField("tmux.reloadInterval", formatIntValue(merged.Tmux.ReloadInterval), fieldSource(local.Tmux.ReloadInterval, global.Tmux.ReloadInterval, def.Tmux.ReloadInterval))
			printField("tmux.notification", formatBoolPtrValue(merged.Tmux.Notification), fieldSourceBoolPtr(local.Tmux.Notification, global.Tmux.Notification, def.Tmux.Notification))
			printField("tmux.notifyCommand", formatStringValue(merged.Tmux.NotifyCommand), fieldSource(local.Tmux.NotifyCommand, global.Tmux.NotifyCommand, def.Tmux.NotifyCommand))
			printField("tmux.notifyWaitCommand", formatStringValue(merged.Tmux.NotifyWaitCommand), fieldSource(local.Tmux.NotifyWaitCommand, global.Tmux.NotifyWaitCommand, def.Tmux.NotifyWaitCommand))
			printField("tmux.switcherPreview", formatBoolPtrValue(merged.Tmux.SwitcherPreview), fieldSourceBoolPtr(local.Tmux.SwitcherPreview, global.Tmux.SwitcherPreview, def.Tmux.SwitcherPreview))
			printField("tmux.layout", formatStringSliceValue(merged.Tmux.Layout), fieldSourceSlice(local.Tmux.Layout, global.Tmux.Layout, def.Tmux.Layout))
			printField("tmux.panes", formatPaneSliceValue(merged.Tmux.Panes), fieldSourceSlice(local.Tmux.Panes, global.Tmux.Panes, def.Tmux.Panes))
			printField("telemetry", formatBoolPtrValue(merged.Telemetry), fieldSourceBoolPtr(local.Telemetry, global.Telemetry, def.Telemetry))

			if warnings := merged.Validate(); len(warnings) > 0 {
				fmt.Println()
				for _, w := range warnings {
					u.Warn(w)
				}
			}

			return nil
		},
	}
}

func configEditCmd() *cli.Command {
	return &cli.Command{
		Name:  "edit",
		Usage: "Open config file in $EDITOR",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "global",
				Usage: "Edit global config (default)",
			},
			&cli.BoolFlag{
				Name:  "local",
				Usage: "Edit local (per-repo) config",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target repo by name",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.config.edit")()
			flags := parseFlags(cmd)
			path := config.GlobalConfigPath()

			if cmd.Bool("local") {
				bareDir := ""
				if repoName := cmd.String("repo"); repoName != "" {
					dir, err := config.ResolveRepo(repoName)
					if err != nil {
						return err
					}
					bareDir = dir
				} else {
					g := flags.NewGit()
					dir, err := requireWillowRepo(g)
					if err != nil {
						return err
					}
					bareDir = dir
				}
				path = config.LocalConfigPath(bareDir)
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
					return fmt.Errorf("failed to create config file: %w", err)
				}
			}

			editor := os.Getenv("VISUAL")
			if editor == "" {
				editor = os.Getenv("EDITOR")
			}
			if editor == "" {
				editor = "vi"
			}

			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func configInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create a default config file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "local",
				Usage: "Create local (per-repo) config",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target repo by name",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite existing config",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.config.init")()
			flags := parseFlags(cmd)
			u := flags.NewUI()
			path := config.GlobalConfigPath()

			if cmd.Bool("local") {
				bareDir := ""
				if repoName := cmd.String("repo"); repoName != "" {
					dir, err := config.ResolveRepo(repoName)
					if err != nil {
						return err
					}
					bareDir = dir
				} else {
					g := flags.NewGit()
					dir, err := requireWillowRepo(g)
					if err != nil {
						return err
					}
					bareDir = dir
				}
				path = config.LocalConfigPath(bareDir)
			}

			if _, err := os.Stat(path); err == nil && !cmd.Bool("force") {
				return errors.Userf("config already exists at %s (use --force to overwrite)", path)
			}

			cfg := config.DefaultConfig()

			if u.Confirm("Enable anonymous error telemetry (crash reports)?") {
				cfg.Telemetry = config.BoolPtr(true)
			} else {
				cfg.Telemetry = config.BoolPtr(false)
			}

			if err := config.Save(cfg, path); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			u.Success(fmt.Sprintf("Created config at %s", path))
			return nil
		},
	}
}

func baseDirSource(globalVal string) string {
	if os.Getenv("WILLOW_BASE_DIR") != "" {
		return "env"
	}
	if globalVal != "" {
		return "global"
	}
	return "default"
}

func printField(name, value, source string) {
	fmt.Printf("%-30s %s  # %s\n", name+":", value, source)
}

func formatStringValue(v string) string {
	return fmt.Sprintf("%q", v)
}

func formatBoolPtrValue(v *bool) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *v)
}

func formatStringSliceValue(v []string) string {
	if v == nil {
		return "[]"
	}
	return fmt.Sprintf("%v", v)
}

func formatPaneSliceValue(v []config.PaneConfig) string {
	if len(v) == 0 {
		return "[]"
	}
	return fmt.Sprintf("[%d panes]", len(v))
}

func formatIntValue(v int) string {
	return fmt.Sprintf("%d", v)
}

// fieldSource determines the source of a string or int field value.
func fieldSource[T comparable](localVal, globalVal, defaultVal T) string {
	var zero T
	if localVal != zero {
		return "local"
	}
	if globalVal != zero {
		return "global"
	}
	return "default"
}

func fieldSourceBoolPtr(localVal, globalVal, defaultVal *bool) string {
	if localVal != nil {
		return "local"
	}
	if globalVal != nil {
		return "global"
	}
	if defaultVal != nil {
		return "default"
	}
	return "default"
}

type configSliceElem interface {
	string | config.PaneConfig
}

func fieldSourceSlice[T configSliceElem](localVal, globalVal, _ []T) string {
	if localVal != nil {
		return "local"
	}
	if globalVal != nil {
		return "global"
	}
	return "default"
}
