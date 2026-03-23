package tmux

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
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
	got := CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusBusy,
	})
	if len(got) != 0 {
		t.Fatalf("expected 0 transitions on first call, got %d", len(got))
	}

	// Second call changes status — now there IS prior state, so we get a transition.
	got = CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusDone,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].Key != "repo/wt" {
		t.Errorf("expected key %q, got %q", "repo/wt", got[0].Key)
	}
	if got[0].FromStatus != claude.StatusBusy {
		t.Errorf("expected FromStatus %q, got %q", claude.StatusBusy, got[0].FromStatus)
	}
	if got[0].ToStatus != claude.StatusDone {
		t.Errorf("expected ToStatus %q, got %q", claude.StatusDone, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToDone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	// Seed state with BUSY.
	CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusBusy,
	})

	got := CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusDone,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].FromStatus != claude.StatusBusy {
		t.Errorf("expected FromStatus %q, got %q", claude.StatusBusy, got[0].FromStatus)
	}
	if got[0].ToStatus != claude.StatusDone {
		t.Errorf("expected ToStatus %q, got %q", claude.StatusDone, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToWait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusBusy,
	})

	got := CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusWait,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].ToStatus != claude.StatusWait {
		t.Errorf("expected ToStatus %q, got %q", claude.StatusWait, got[0].ToStatus)
	}
}

func TestCheckTransitions_BusyToBusy_NoTransition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusBusy,
	})

	got := CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusBusy,
	})
	if len(got) != 0 {
		t.Fatalf("expected 0 transitions for BUSY->BUSY, got %d", len(got))
	}
}

func TestCheckTransitions_DoneToIdle_NoTransition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureWillowDir(t, home)

	CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusDone,
	})

	got := CheckTransitions(map[string]claude.Status{
		"repo/wt": claude.StatusIdle,
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
	CheckTransitions(map[string]claude.Status{
		"repo/wt1": claude.StatusBusy,
		"repo/wt2": claude.StatusBusy,
	})

	// Transition both to different non-BUSY statuses.
	got := CheckTransitions(map[string]claude.Status{
		"repo/wt1": claude.StatusDone,
		"repo/wt2": claude.StatusWait,
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(got))
	}

	// Sort by key for deterministic assertions.
	sort.Slice(got, func(i, j int) bool { return got[i].Key < got[j].Key })

	if got[0].Key != "repo/wt1" || got[0].ToStatus != claude.StatusDone {
		t.Errorf("transition[0]: expected repo/wt1->DONE, got %s->%s", got[0].Key, got[0].ToStatus)
	}
	if got[1].Key != "repo/wt2" || got[1].ToStatus != claude.StatusWait {
		t.Errorf("transition[1]: expected repo/wt2->WAIT, got %s->%s", got[1].Key, got[1].ToStatus)
	}
}
