package harness

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
)

const (
	ClaudeID = "claude"
	CodexID  = "codex"
)

type Capabilities struct {
	SupportsSessionEnd     bool
	SupportsPermissionWait bool
	SupportsFilesTouched   bool
	RequiresHookTrust      bool
	SupportsNotification   bool
	SupportsPermissionMode bool
	SupportsTurnID         bool
}

type LegacyHook struct {
	Event   string
	Command string
}

type NormalizedHook struct {
	HarnessID      string
	SessionID      string
	EventName      string
	ToolName       string
	FilePath       string
	FilesTouched   []string
	Model          string
	TurnID         string
	PermissionMode string
}

type LaunchCommand struct {
	Command string
	Args    []string
}

type LaunchOptions struct {
	Prompt    string
	Yolo      bool
	Overrides config.AgentHarnessConfig
}

type ShellLaunchOptions struct {
	PromptArg    string
	PromptArgRaw bool
	Yolo         bool
	Overrides    config.AgentHarnessConfig
}

type Harness interface {
	ID() string
	DisplayName() string
	ExecutableName() string
	SetupCommandLabel() string
	DocsHint() string
	HookEvents() []string
	Capabilities() Capabilities
	HookCommand(exe string) string
	InstallHooks(command string) (changed bool, err error)
	HooksInstalled(command string) bool
	LegacyHooks() []LegacyHook
	RemoveLegacyHooks() (removed []string, changed bool, err error)
	NormalizeHook(raw []byte) (NormalizedHook, bool)
	BuildLaunch(LaunchOptions) LaunchCommand
	BuildShellLaunch(ShellLaunchOptions) string
}

var registry = map[string]Harness{}

func Register(h Harness) {
	registry[h.ID()] = h
}

func Get(id string) (Harness, bool) {
	id = NormalizeID(id)
	h, ok := registry[id]
	return h, ok
}

func MustGet(id string) (Harness, error) {
	h, ok := Get(id)
	if !ok {
		return nil, fmt.Errorf("unknown agent harness %q (known: %s)", id, strings.Join(IDs(), ", "))
	}
	return h, nil
}

func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func NormalizeID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return ClaudeID
	}
	return id
}

func DefaultID(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Agent.Default) != "" {
		return NormalizeID(cfg.Agent.Default)
	}
	return ClaudeID
}

func CurrentHookCommand(h Harness) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve willow executable: %w", err)
	}
	return h.HookCommand(exe), nil
}

func OverridesFor(cfg *config.Config, id string) config.AgentHarnessConfig {
	if cfg == nil || cfg.Agent.Harnesses == nil {
		return config.AgentHarnessConfig{}
	}
	return cfg.Agent.Harnesses[NormalizeID(id)]
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func appendYoloArgs(args []string, opts LaunchOptions, defaults []string) []string {
	if !opts.Yolo {
		return args
	}
	yoloArgs := defaults
	if opts.Overrides.YoloArgs != nil {
		yoloArgs = opts.Overrides.YoloArgs
	}
	return append(args, yoloArgs...)
}

func launchCommand(defaultCommand string, defaultArgs []string, opts LaunchOptions, promptFirst bool, yoloDefaults []string) LaunchCommand {
	command := defaultCommand
	if opts.Overrides.Command != "" {
		command = opts.Overrides.Command
	}
	args := append([]string{}, opts.Overrides.Args...)
	if len(args) == 0 {
		args = append(args, defaultArgs...)
	}
	if promptFirst {
		if opts.Prompt != "" {
			args = append(args, opts.Prompt)
		}
		args = appendYoloArgs(args, opts, yoloDefaults)
	} else {
		args = appendYoloArgs(args, opts, yoloDefaults)
		if opts.Prompt != "" {
			args = append(args, opts.Prompt)
		}
	}
	return LaunchCommand{Command: command, Args: args}
}

func shellLaunch(defaultCommand string, defaultArgs []string, opts ShellLaunchOptions, promptFirst bool, yoloDefaults []string) string {
	command := defaultCommand
	if opts.Overrides.Command != "" {
		command = opts.Overrides.Command
	}
	args := append([]string{}, opts.Overrides.Args...)
	if len(args) == 0 {
		args = append(args, defaultArgs...)
	}
	promptArg := opts.PromptArg
	if promptArg != "" && !opts.PromptArgRaw {
		promptArg = shellQuote(promptArg)
	}
	yoloArgs := []string{}
	if opts.Yolo {
		yoloArgs = yoloDefaults
		if opts.Overrides.YoloArgs != nil {
			yoloArgs = opts.Overrides.YoloArgs
		}
	}
	if promptFirst {
		if promptArg != "" {
			args = append(args, promptArg)
		}
		args = append(args, yoloArgs...)
	} else {
		args = append(args, yoloArgs...)
		if promptArg != "" {
			args = append(args, promptArg)
		}
	}
	parts := []string{shellQuote(command)}
	for _, arg := range args {
		if arg == promptArg && opts.PromptArgRaw {
			parts = append(parts, arg)
			continue
		}
		if strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'") {
			parts = append(parts, arg)
		} else {
			parts = append(parts, shellQuote(arg))
		}
	}
	return strings.Join(parts, " ")
}
