package cli

import (
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h"},
		{3 * time.Hour, "3h"},
		{36 * time.Hour, "1d"},
		{5 * 24 * time.Hour, "5d"},
		{14 * 24 * time.Hour, "2w"},
		{30 * 24 * time.Hour, "4w"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatAge(tt.d)
			if got != tt.want {
				t.Errorf("formatAge(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func lsTestStack(pairs ...string) *stack.Stack {
	st := &stack.Stack{Parents: make(map[string]string)}
	for i := 0; i+1 < len(pairs); i += 2 {
		st.Parents[pairs[i]] = pairs[i+1]
	}
	return st
}

func lsBranches(rows []lsRow) []string {
	branches := make([]string, len(rows))
	for i, row := range rows {
		branches[i] = row.branch
	}
	return branches
}

func TestSortLSRows_UrgencyAndMergedOrder(t *testing.T) {
	rows := []lsRow{
		{branch: "busy", status: claude.StatusBusy, wt: worktree.Worktree{Branch: "busy"}},
		{branch: "done-unread", status: claude.StatusDone, unread: true, wt: worktree.Worktree{Branch: "done-unread"}},
		{branch: "wait", status: claude.StatusWait, wt: worktree.Worktree{Branch: "wait"}},
		{branch: "done-read", status: claude.StatusDone, wt: worktree.Worktree{Branch: "done-read"}},
		{branch: "idle", status: claude.StatusIdle, wt: worktree.Worktree{Branch: "idle"}},
		{branch: "merged-wait", status: claude.StatusWait, merged: true, wt: worktree.Worktree{Branch: "merged-wait"}},
	}

	got := lsBranches(sortLSRows(rows, nil))
	want := []string{"wait", "done-unread", "busy", "done-read", "idle", "merged-wait"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

func TestSortLSRows_StackStaysContiguousAndRanksByUrgency(t *testing.T) {
	rows := []lsRow{
		{branch: "busy", status: claude.StatusBusy, wt: worktree.Worktree{Branch: "busy"}},
		{branch: "stack-a", status: claude.StatusIdle, wt: worktree.Worktree{Branch: "stack-a"}},
		{branch: "stack-b", status: claude.StatusWait, wt: worktree.Worktree{Branch: "stack-b"}},
		{branch: "done-read", status: claude.StatusDone, wt: worktree.Worktree{Branch: "done-read"}},
	}

	got := sortLSRows(rows, lsTestStack("stack-a", "main", "stack-b", "stack-a"))
	want := []string{"stack-a", "stack-b", "busy", "done-read"}
	branches := lsBranches(got)
	for i := range want {
		if branches[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, branches[i], want[i], branches)
		}
	}
	if got[0].prefix != "" {
		t.Fatalf("root prefix = %q, want empty", got[0].prefix)
	}
	if got[1].prefix == "" {
		t.Fatal("child prefix should be preserved")
	}
}
