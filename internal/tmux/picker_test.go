package tmux

import (
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
)

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"single ANSI code stripped", "\033[0;32mgreen\033[0m", "green"},
		{"multiple escapes", "\033[0;31mred\033[0m and \033[0;34mblue\033[0m", "red and blue"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde prefix expands", "~/foo", "/fakehome/foo"},
		{"absolute path unchanged", "/usr/local/bin", "/usr/local/bin"},
		{"relative path unchanged", "relative/path", "relative/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/fakehome")
			got := expandHome(tt.path)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"home prefix becomes tilde", "/fakehome/code/project", "~/code/project"},
		{"non-home path unchanged", "/other/path", "/other/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/fakehome")
			got := shortenPath(tt.path)
			if got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"shorter than limit", "hi", 10, "hi"},
		{"longer than limit", "abcdefghij", 5, "abcde"},
		{"exact length", "abc", 3, "abc"},
		{"empty string", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		name   string
		status claude.Status
		want   string
	}{
		{"BUSY is green", claude.StatusBusy, colorGreen},
		{"WAIT is red", claude.StatusWait, colorRed},
		{"DONE is blue", claude.StatusDone, colorBlue},
		{"IDLE is yellow", claude.StatusIdle, colorYellow},
		{"OFFLINE is dim", claude.StatusOffline, colorDim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusColor(tt.status)
			if got != tt.want {
				t.Errorf("statusColor(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}


func TestDisplayName(t *testing.T) {
	item := PickerItem{RepoName: "myrepo", Branch: "feat-auth"}

	tests := []struct {
		name      string
		multiRepo bool
		want      string
	}{
		{"single repo shows branch only", false, "feat-auth"},
		{"multi repo shows repo/branch", true, "myrepo/feat-auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayName(item, tt.multiRepo)
			if got != tt.want {
				t.Errorf("displayName(item, %v) = %q, want %q", tt.multiRepo, got, tt.want)
			}
		})
	}
}

func TestHasMultipleRepos(t *testing.T) {
	tests := []struct {
		name  string
		items []PickerItem
		want  bool
	}{
		{"empty slice", nil, false},
		{"single item", []PickerItem{{RepoName: "a"}}, false},
		{"same repo items", []PickerItem{{RepoName: "a"}, {RepoName: "a"}}, false},
		{"mixed repos", []PickerItem{{RepoName: "a"}, {RepoName: "b"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasMultipleRepos(tt.items)
			if got != tt.want {
				t.Errorf("hasMultipleRepos() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterActiveSessions(t *testing.T) {
	now := time.Now()
	stale := time.Now().Add(-5 * time.Minute)

	tests := []struct {
		name     string
		sessions []*claude.SessionStatus
		wantLen  int
	}{
		{
			"mix of active and idle",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: now},
				{Status: claude.StatusIdle, Timestamp: now},
				{Status: claude.StatusWait, Timestamp: now},
			},
			2,
		},
		{
			"all idle returns empty",
			[]*claude.SessionStatus{
				{Status: claude.StatusIdle, Timestamp: now},
				{Status: claude.StatusIdle, Timestamp: now},
			},
			0,
		},
		{
			"all active",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: now},
				{Status: claude.StatusDone, Timestamp: now},
				{Status: claude.StatusWait, Timestamp: now},
			},
			3,
		},
		{
			"stale BUSY/WAIT become idle, stale DONE stays",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: stale},
				{Status: claude.StatusDone, Timestamp: stale},
				{Status: claude.StatusWait, Timestamp: stale},
			},
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterActiveSessions(tt.sessions)
			if len(got) != tt.wantLen {
				t.Errorf("filterActiveSessions() returned %d sessions, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestExtractPathFromLine(t *testing.T) {
	t.Setenv("HOME", "/fakehome")

	tests := []struct {
		name string
		line string
		want string
	}{
		{
			"formatted picker line",
			"\033[0;32m\U0001F916 BUSY   \033[0m | feat-auth | \033[2m~/code/project\033[0m",
			"/fakehome/code/project",
		},
		{
			"sub-row line",
			"  \033[0;32m\U0001F916 BUSY \033[0m | \033[2m\u2514 abcd1234 5m ago\033[2m\033[0m | \033[2m~/code/project\033[0m",
			"/fakehome/code/project",
		},
		{
			"no pipes returns original expanded",
			"just-a-string",
			"just-a-string",
		},
		{
			"empty string",
			"",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPathFromLine(tt.line)
			if got != tt.want {
				t.Errorf("ExtractPathFromLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
