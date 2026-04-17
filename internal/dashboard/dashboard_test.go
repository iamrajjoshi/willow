package dashboard

import (
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/git"
)

func TestParseShortstatFromGit(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "full output",
			out:  " 3 files changed, 42 insertions(+), 12 deletions(-)",
			want: "3f +42 -12",
		},
		{
			name: "insertions only",
			out:  " 1 file changed, 10 insertions(+)",
			want: "1f +10",
		},
		{
			name: "deletions only",
			out:  " 2 files changed, 5 deletions(-)",
			want: "2f -5",
		},
		{
			name: "empty string",
			out:  "",
			want: "--",
		},
		{
			name: "single file single insertion and deletion",
			out:  " 1 file changed, 1 insertion(+), 1 deletion(-)",
			want: "1f +1 -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := git.ParseShortstat(tt.out)
			if got != tt.want {
				t.Errorf("git.ParseShortstat(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

func TestRenderNoSessions(t *testing.T) {
	out := render(nil, summary{Repos: 2, Agents: 0, Unread: 0}, 120, 0, false)
	if !strings.Contains(out, "No active sessions") {
		t.Errorf("expected 'No active sessions' in output, got:\n%s", out)
	}
}

func TestRenderSingleSession(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--thing",
			SessionID: "abc123",
			Status:    claude.StatusBusy,
			Tool:      "",
			DiffStats: "2f +10 -3",
			Age:       "5m",
			Unread:    false,
			WtDirName: "feat--thing",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false)

	if !strings.Contains(out, "myrepo") {
		t.Errorf("expected 'myrepo' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "feat--thing") {
		t.Errorf("expected 'feat--thing' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "2f +10 -3") {
		t.Errorf("expected diff stats in output, got:\n%s", out)
	}
}

func TestRenderUnreadMarker(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--done",
			SessionID: "abc123",
			Status:    claude.StatusDone,
			DiffStats: "--",
			Age:       "2m",
			Unread:    true,
			WtDirName: "feat--done",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 1}, 120, 0, false)

	if !strings.Contains(out, "\u25CF") {
		t.Errorf("expected unread marker (bullet) in output, got:\n%s", out)
	}
}

func TestRenderBusyWithTool(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--edit",
			SessionID: "abc123",
			Status:    claude.StatusBusy,
			Tool:      "Edit",
			DiffStats: "1f +5",
			Age:       "1m",
			Unread:    false,
			WtDirName: "feat--edit",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false)

	if !strings.Contains(out, "(Edit)") {
		t.Errorf("expected '(Edit)' tool label in output, got:\n%s", out)
	}
}

func TestRenderSelectedRow(t *testing.T) {
	sessions := []session{
		{
			Repo:      "repo-a",
			Branch:    "branch-a",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "3m",
			WtDirName: "branch-a",
		},
		{
			Repo:      "repo-b",
			Branch:    "branch-b",
			Status:    claude.StatusDone,
			DiffStats: "1f +2",
			Age:       "8m",
			WtDirName: "branch-b",
		},
	}
	out := render(sessions, summary{Repos: 2, Agents: 2, Unread: 0}, 120, 1, false)

	if !strings.Contains(out, "\033[7m") {
		t.Errorf("expected inverse video escape sequence for selected row, got:\n%s", out)
	}
}

func TestRenderKeybindingHint(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "main",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "1m",
			WtDirName: "main",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false)

	if !strings.Contains(out, "j/k: navigate") {
		t.Errorf("expected keybinding hint 'j/k: navigate' in output, got:\n%s", out)
	}
}

func TestRenderTimelineColumn(t *testing.T) {
	sparkline := strings.Repeat("\033[32m\u2588\033[0m", 30)
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--thing",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "2m",
			WtDirName: "feat--thing",
			Timeline:  sparkline,
		},
	}

	// With timeline enabled
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 200, 0, true)
	if !strings.Contains(out, "TIMELINE") {
		t.Errorf("expected 'TIMELINE' header when showTimeline=true, got:\n%s", out)
	}
	if !strings.Contains(out, "\u2588") {
		t.Errorf("expected sparkline blocks in output when showTimeline=true")
	}

	// With timeline disabled
	out = render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 200, 0, false)
	if strings.Contains(out, "TIMELINE") {
		t.Errorf("expected no 'TIMELINE' header when showTimeline=false, got:\n%s", out)
	}
}

func TestRenderTimelineKeybindingHint(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "main",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "1m",
			WtDirName: "main",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, true)
	if !strings.Contains(out, "t: timeline") {
		t.Errorf("expected 't: timeline' in keybinding hint, got:\n%s", out)
	}
}
