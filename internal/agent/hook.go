package agent

import (
	"fmt"
	"os"

	"github.com/iamrajjoshi/willow/internal/agent/harness"
)

func InstallHarness(id string) (bool, error) {
	if err := os.MkdirAll(StatusDir(), 0o755); err != nil {
		return false, fmt.Errorf("failed to create status directory: %w", err)
	}
	h, err := harness.MustGet(id)
	if err != nil {
		return false, err
	}
	cmd, err := harness.CurrentHookCommand(h)
	if err != nil {
		return false, err
	}
	return h.InstallHooks(cmd)
}

func IsHarnessInstalled(id string) bool {
	h, ok := harness.Get(id)
	if !ok {
		return false
	}
	cmd, err := harness.CurrentHookCommand(h)
	if err != nil {
		return false
	}
	return h.HooksInstalled(cmd)
}

func HookCommandForHarness(id string) (string, error) {
	h, err := harness.MustGet(id)
	if err != nil {
		return "", err
	}
	return harness.CurrentHookCommand(h)
}

func UnmarkedLegacyHooks() []harness.LegacyHook {
	h, ok := harness.Get(harness.ClaudeID)
	if !ok {
		return nil
	}
	return h.LegacyHooks()
}

func RemoveLegacyWillowHooks() ([]string, bool, error) {
	h, err := harness.MustGet(harness.ClaudeID)
	if err != nil {
		return nil, false, err
	}
	return h.RemoveLegacyHooks()
}
