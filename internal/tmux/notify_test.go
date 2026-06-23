package tmux

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/config"
)

// ensureWillowDir creates the ~/.willow directory so saveState can write the state file.
func ensureWillowDir(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".willow"), 0o755); err != nil {
		t.Fatalf("creating .willow dir: %v", err)
	}
}

func TestCheckTransitions_FirstCallNoPriorState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	// First call with no prior state file — should return no transitions.
	got := CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusBusy,
	})
	if len(got) != 0 {
		t.Fatalf("expected 0 transitions on first call, got %d", len(got))
	}

	// Second call changes status — now there IS prior state, so we get a transition.
	got = CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusDone,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].Key != "repo/wt" {
		t.Errorf("expected key %q, got %q", "repo/wt", got[0].Key)
	}
	if got[0].FromStatus != agent.StatusBusy {
		t.Errorf("expected FromStatus %q, got %q", agent.StatusBusy, got[0].FromStatus)
	}
	if got[0].ToStatus != agent.StatusDone {
		t.Errorf("expected ToStatus %q, got %q", agent.StatusDone, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToDone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	// Seed state with BUSY.
	CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusBusy,
	})

	got := CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusDone,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].FromStatus != agent.StatusBusy {
		t.Errorf("expected FromStatus %q, got %q", agent.StatusBusy, got[0].FromStatus)
	}
	if got[0].ToStatus != agent.StatusDone {
		t.Errorf("expected ToStatus %q, got %q", agent.StatusDone, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToWait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusBusy,
	})

	got := CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusWait,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].ToStatus != agent.StatusWait {
		t.Errorf("expected ToStatus %q, got %q", agent.StatusWait, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToBusy_NoTransition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusBusy,
	})

	got := CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusBusy,
	})
	if len(got) != 0 {
		t.Fatalf("expected 0 transitions for BUSY->BUSY, got %d", len(got))
	}
}

func TestCheckTransitions_DoneToIdle_NoTransition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusDone,
	})

	got := CheckTransitions(map[string]agent.Status{
		"repo/wt": agent.StatusIdle,
	})
	if len(got) != 0 {
		t.Fatalf("expected 0 transitions for DONE->IDLE, got %d", len(got))
	}
}

func TestCheckTransitions_MultipleTransitions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	// Seed both entries as BUSY.
	CheckTransitions(map[string]agent.Status{
		"repo/wt1": agent.StatusBusy,
		"repo/wt2": agent.StatusBusy,
	})

	// Transition both to different non-BUSY statuses.
	got := CheckTransitions(map[string]agent.Status{
		"repo/wt1": agent.StatusDone,
		"repo/wt2": agent.StatusWait,
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(got))
	}

	// Sort by key for deterministic assertions.
	sort.Slice(got, func(i, j int) bool { return got[i].Key < got[j].Key })

	if got[0].Key != "repo/wt1" || got[0].ToStatus != agent.StatusDone {
		t.Errorf("transition[0]: expected repo/wt1->DONE, got %s->%s", got[0].Key, got[0].ToStatus)
	}
	if got[1].Key != "repo/wt2" || got[1].ToStatus != agent.StatusWait {
		t.Errorf("transition[1]: expected repo/wt2->WAIT, got %s->%s", got[1].Key, got[1].ToStatus)
	}
}

func TestNotifyUsesDefaultAndCustomCommands(t *testing.T) {
	var commands []string
	oldStart := startNotify
	startNotify = func(command string) {
		commands = append(commands, command)
	}
	t.Cleanup(func() { startNotify = oldStart })

	Notify("")
	Notify("printf custom")

	want := []string{defaultNotifyCommand, "printf custom"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestNotifyWithContextSkipsCurrentSessionAndChoosesCommands(t *testing.T) {
	oldCurrent := currentSession
	oldStart := startNotify
	currentSession = func() (string, error) { return "repo/current", nil }
	var commands []string
	startNotify = func(command string) {
		commands = append(commands, command)
	}
	t.Cleanup(func() {
		currentSession = oldCurrent
		startNotify = oldStart
	})

	NotifyWithContext([]agent.Transition{
		{Key: "repo/current", ToStatus: agent.StatusDone},
		{Key: "repo/wait", ToStatus: agent.StatusWait},
		{Key: "repo/done", ToStatus: agent.StatusDone},
		{Key: "repo/busy", ToStatus: agent.StatusBusy},
	}, &config.Config{
		Tmux: config.TmuxConfig{
			NotifyCommand:     "done-command",
			NotifyWaitCommand: "wait-command",
		},
	})

	want := []string{"wait-command", "done-command"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestNotifyWithContextDefaultsWaitCommand(t *testing.T) {
	oldCurrent := currentSession
	oldStart := startNotify
	currentSession = func() (string, error) { return "", nil }
	var commands []string
	startNotify = func(command string) {
		commands = append(commands, command)
	}
	t.Cleanup(func() {
		currentSession = oldCurrent
		startNotify = oldStart
	})

	NotifyWithContext([]agent.Transition{
		{Key: "repo/wait", ToStatus: agent.StatusWait},
	}, &config.Config{})

	want := []string{defaultNotifyWaitCommand}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}
