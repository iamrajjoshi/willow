package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/termfmt"
	"github.com/iamrajjoshi/willow/internal/ui"
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
		{branch: "busy", status: agent.StatusBusy, wt: worktree.Worktree{Branch: "busy"}},
		{branch: "done-unread", status: agent.StatusDone, unread: true, wt: worktree.Worktree{Branch: "done-unread"}},
		{branch: "wait", status: agent.StatusWait, wt: worktree.Worktree{Branch: "wait"}},
		{branch: "done-read", status: agent.StatusDone, wt: worktree.Worktree{Branch: "done-read"}},
		{branch: "idle", status: agent.StatusIdle, wt: worktree.Worktree{Branch: "idle"}},
		{branch: "merged-wait", status: agent.StatusWait, merged: true, wt: worktree.Worktree{Branch: "merged-wait"}},
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
		{branch: "busy", status: agent.StatusBusy, wt: worktree.Worktree{Branch: "busy"}},
		{branch: "stack-a", status: agent.StatusIdle, wt: worktree.Worktree{Branch: "stack-a"}},
		{branch: "stack-b", status: agent.StatusWait, wt: worktree.Worktree{Branch: "stack-b"}},
		{branch: "done-read", status: agent.StatusDone, wt: worktree.Worktree{Branch: "done-read"}},
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

func TestFormatLSTableRowsFitsNarrowWidth(t *testing.T) {
	t.Setenv("HOME", "/Users/raj.joshi")
	u := &ui.UI{}
	long := "raj--tprm-464--backend-validate-review-risk-subtype"
	rows := []lsRow{
		{
			branch: long,
			prefix: "\u2514\u2500 ",
			status: agent.StatusDone,
			age:    "1h",
			wt: worktree.Worktree{
				Branch: long,
				Path:   "/Users/raj.joshi/.willow/worktrees/evergreen/" + long,
			},
		},
	}

	lines := formatLSTableRows(u, rows, 80)
	for _, line := range lines {
		if got := termfmt.VisibleWidth(line); got > 80 {
			t.Fatalf("line width = %d, want <= 80:\n%s", got, termfmt.StripANSI(line))
		}
	}
	plain := termfmt.StripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "AGE") || !strings.Contains(plain, "1h") {
		t.Fatalf("formatted ls table should preserve age column:\n%s", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("formatted ls table should truncate narrow output:\n%s", plain)
	}
}

func TestFormatRepoListRowsFitsNarrowWidth(t *testing.T) {
	u := &ui.UI{}
	rows := []repoListRow{
		{repo: "evergreen-with-a-very-long-name", count: 15, activeCount: 10, unreadCount: 1, ok: true},
	}
	lines := formatRepoListRows(u, rows, 36)
	for _, line := range lines {
		if got := termfmt.VisibleWidth(line); got > 36 {
			t.Fatalf("line width = %d, want <= 36:\n%s", got, termfmt.StripANSI(line))
		}
	}
}
