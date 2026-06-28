package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func doctorCmd() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Check your willow setup for common issues",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "fix",
				Usage: "Remove unmarked legacy willow hooks from ~/.claude/settings.json after confirmation",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.doctor")()
			flags := parseFlags(cmd)
			u := flags.NewUI()

			checkGitVersion(u)
			checkBinary(u, "gh", "gh CLI", "https://cli.github.com")
			checkBinary(u, "tmux", "tmux", "https://github.com/tmux/tmux")
			checkClickableNotifications(u)
			checkAgentHarnesses(u, cmd.Bool("fix"))
			checkWillowDirs(u)
			checkStaleSessions(u)
			checkConfig(u)

			return nil
		},
	}
}

func checkGitVersion(u interface {
	Success(string)
	Warn(string)
	Red(string) string
}) {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s git not found\n", u.Red("\u2717"))
		return
	}

	major, minor, patch, err := parseGitVersion(strings.TrimSpace(string(out)))
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s git version could not be parsed\n", u.Red("\u2717"))
		return
	}

	version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if major < 2 || (major == 2 && minor < 30) {
		u.Warn(fmt.Sprintf("git %s (recommend >= 2.30)", version))
		return
	}
	u.Success(fmt.Sprintf("git %s", version))
}

func parseGitVersion(output string) (major, minor, patch int, err error) {
	// "git version 2.45.0" or "git version 2.39.3 (Apple Git-146)"
	// Extract the version token right after "version", or fall back to the
	// first token that looks like a dotted number.
	parts := strings.Fields(output)
	if len(parts) == 0 {
		return 0, 0, 0, fmt.Errorf("empty version string")
	}

	versionStr := ""
	for i, p := range parts {
		if p == "version" && i+1 < len(parts) {
			versionStr = parts[i+1]
			break
		}
	}
	if versionStr == "" {
		versionStr = parts[len(parts)-1]
	}

	segments := strings.SplitN(versionStr, ".", 4) // at most major.minor.patch, ignore rest
	if len(segments) < 2 {
		return 0, 0, 0, fmt.Errorf("unexpected version format: %s", versionStr)
	}

	major, err = strconv.Atoi(segments[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %s", segments[0])
	}
	minor, err = strconv.Atoi(segments[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %s", segments[1])
	}
	if len(segments) >= 3 {
		patch, err = strconv.Atoi(segments[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid patch version: %s", segments[2])
		}
	}

	return major, minor, patch, nil
}

type binaryChecker interface {
	Success(string)
	Warn(string)
	Red(string) string
}

func checkBinary(u binaryChecker, name, label, installURL string) {
	if _, err := exec.LookPath(name); err != nil {
		u.Warn(fmt.Sprintf("%s not found (install: %s)", label, installURL))
		return
	}
	u.Success(fmt.Sprintf("%s installed", label))
}

// checkClickableNotifications reports whether terminal-notifier is available.
// Without it, desktop notifications still fire via osascript but can't focus
// the agent's session on click. macOS-only; silent elsewhere.
func checkClickableNotifications(u binaryChecker) {
	if runtime.GOOS != "darwin" {
		return
	}
	if _, err := exec.LookPath("terminal-notifier"); err != nil {
		u.Warn("terminal-notifier not found; notification clicks won't focus the session (install: brew install terminal-notifier)")
		return
	}
	u.Success("terminal-notifier installed (clickable notifications)")
}

type agentHooksUI interface {
	Success(string)
	Warn(string)
	Info(string)
	Confirm(string) bool
}

func checkAgentHarnesses(u agentHooksUI, fix bool) {
	cfg := config.Load("")
	defaultID := harness.DefaultID(cfg)
	for _, id := range harness.IDs() {
		h, err := harness.MustGet(id)
		if err != nil {
			continue
		}
		commandName := h.ExecutableName()
		if override := harness.OverridesFor(cfg, id); override.Command != "" {
			commandName = override.Command
		}
		label := agentBinaryLabel(h.DisplayName(), id == defaultID)
		if _, err := exec.LookPath(commandName); err != nil {
			u.Warn(fmt.Sprintf("%s not found (run: %s after installing)", label, h.SetupCommandLabel()))
		} else {
			u.Success(fmt.Sprintf("%s installed", label))
		}

		if !agent.IsHarnessInstalled(id) {
			u.Warn(fmt.Sprintf("%s hooks not installed (run: %s)", h.DisplayName(), h.SetupCommandLabel()))
		} else {
			u.Success(fmt.Sprintf("%s hooks installed", h.DisplayName()))
		}
	}

	legacy := agent.UnmarkedLegacyHooks()
	if len(legacy) == 0 {
		return
	}

	// Collapse duplicate commands across events for display; one warn per unique command.
	seen := map[string]bool{}
	for _, h := range legacy {
		if seen[h.Command] {
			continue
		}
		seen[h.Command] = true
		u.Warn(fmt.Sprintf("legacy willow hook in ~/.claude/settings.json: %q", h.Command))
	}

	if !fix {
		u.Info("  run 'ww doctor --fix' to remove these")
		return
	}

	if !u.Confirm(fmt.Sprintf("Remove %d legacy willow hook rule(s) across %d event(s)?", len(legacy), len(seen))) {
		u.Info("  skipped")
		return
	}

	removed, _, err := agent.RemoveLegacyWillowHooks()
	if err != nil {
		u.Warn(fmt.Sprintf("could not remove legacy hooks: %v", err))
		return
	}
	u.Success(fmt.Sprintf("Removed %d legacy hook(s)", len(removed)))
}

func checkWillowDirs(u interface {
	Success(string)
	Red(string) string
}) {
	dirs := []struct {
		path string
	}{
		{config.WillowHome()},
		{config.ReposDir()},
		{config.WorktreesDir()},
	}

	for _, d := range dirs {
		if _, err := os.Stat(d.path); err != nil {
			fmt.Fprintf(os.Stdout, "%s %s missing (run: ww clone)\n", u.Red("\u2717"), d.path)
			continue
		}
		u.Success(fmt.Sprintf("%s exists", d.path))
	}
}

func agentBinaryLabel(displayName string, isDefault bool) string {
	label := displayName
	if isDefault {
		label += " (default)"
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(displayName)), "cli") {
		return label
	}
	return label + " CLI"
}

func checkStaleSessions(u interface {
	Success(string)
	Warn(string)
}) {
	sessions, err := agent.ScanAllSessions()
	if err != nil {
		u.Warn(fmt.Sprintf("could not scan sessions: %v", err))
		return
	}

	staleCount := 0
	for _, s := range sessions {
		if time.Since(s.Session.Timestamp) > 30*time.Minute {
			staleCount++
		}
	}

	if staleCount > 0 {
		u.Warn(fmt.Sprintf("%d stale session file(s) (run: ww refresh-status)", staleCount))
		return
	}
	u.Success("no stale session files")
}

func checkConfig(u interface {
	Success(string)
	Warn(string)
}) {
	cfg := config.Load("")
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		u.Success("config valid")
		return
	}
	for _, w := range warnings {
		u.Warn(fmt.Sprintf("config: %s", w))
	}
}
