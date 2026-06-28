package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// setupWorktreeHome creates a fake willow base dir with a worktree at
// <home>/.willow/worktrees/<repo>/<wt> and chdirs into it. Returns the
// (repo, wt) pair for assertions.
func setupWorktreeHome(t *testing.T) (repo, wt string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo, wt = "myrepo", "feat-x"
	wtPath := filepath.Join(home, ".willow", "worktrees", repo, wt)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	// Resolve symlinks (macOS /var → /private/var) so the handler's
	// EvalSymlinks on cwd matches.
	if resolved, err := filepath.EvalSymlinks(wtPath); err == nil {
		wtPath = resolved
	}

	prevCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(prevCwd) })
	if err := os.Chdir(wtPath); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return repo, wt
}

func TestHandleHook_UserPromptSubmitWritesBusy(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	in := HookInput{SessionID: "s1", HookEventName: "UserPromptSubmit"}
	raw, _ := json.Marshal(in)
	if err := HandleHook(bytes.NewReader(raw)); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	got := readSession(SessionPath(repo, wt, "claude", "s1"))
	if got.Status != StatusBusy {
		t.Errorf("status = %q, want %q", got.Status, StatusBusy)
	}
	if got.SessionID != "s1" {
		t.Errorf("session_id = %q, want s1", got.SessionID)
	}
	if got.StartTime.IsZero() {
		t.Error("start_time should be set")
	}
}

func TestHandleHook_PreToolUseIncrementsCount(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	send := func(ev HookInput) {
		raw, _ := json.Marshal(ev)
		if err := HandleHook(bytes.NewReader(raw)); err != nil {
			t.Fatalf("HandleHook: %v", err)
		}
	}

	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Read"})
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Read"})
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Read"})

	got := readSession(SessionPath(repo, wt, "claude", "s1"))
	if got.ToolCount != 3 {
		t.Errorf("tool_count = %d, want 3", got.ToolCount)
	}
	if got.Tool != "Read" {
		t.Errorf("tool = %q, want Read", got.Tool)
	}
}

func TestHandleHook_WaitTools(t *testing.T) {
	cases := []struct {
		name  string
		event string
		tool  string
		want  Status
	}{
		{"PostToolUse AskUserQuestion", "PostToolUse", "AskUserQuestion", StatusWait},
		{"PostToolUse ExitPlanMode", "PostToolUse", "ExitPlanMode", StatusWait},
		{"PreToolUse AskUserQuestion", "PreToolUse", "AskUserQuestion", StatusWait},
		{"PreToolUse ExitPlanMode", "PreToolUse", "ExitPlanMode", StatusWait},
		{"PreToolUse Read", "PreToolUse", "Read", StatusBusy},
		{"PostToolUse Read", "PostToolUse", "Read", StatusBusy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, wt := setupWorktreeHome(t)
			in := HookInput{SessionID: "s1", HookEventName: tc.event, ToolName: tc.tool}
			raw, _ := json.Marshal(in)
			if err := HandleHook(bytes.NewReader(raw)); err != nil {
				t.Fatalf("HandleHook: %v", err)
			}
			got := readSession(SessionPath(repo, wt, "claude", "s1"))
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

func TestHandleHook_StopSetsDone(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	in := HookInput{SessionID: "s1", HookEventName: "Stop"}
	raw, _ := json.Marshal(in)
	if err := HandleHook(bytes.NewReader(raw)); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	got := readSession(SessionPath(repo, wt, "claude", "s1"))
	if got.Status != StatusDone {
		t.Errorf("status = %q, want %q", got.Status, StatusDone)
	}
}

func TestHandleHook_CursorWorkEventSetsBusy(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	raw := []byte(`{
		"session_id": "cursor-s1",
		"hook_event_name": "preToolUse",
		"tool_name": "Shell"
	}`)
	if err := HandleHook(bytes.NewReader(raw), "cursor"); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	got := readSession(SessionPath(repo, wt, "cursor", "cursor-s1"))
	if got.Status != StatusBusy {
		t.Errorf("status = %q, want %q", got.Status, StatusBusy)
	}
	if got.ToolCount != 1 {
		t.Errorf("tool_count = %d, want 1", got.ToolCount)
	}
	if got.Tool != "Shell" {
		t.Errorf("tool = %q, want Shell", got.Tool)
	}
}

func TestHandleHook_CursorStopSetsDone(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	raw := []byte(`{
		"conversation_id": "cursor-conv",
		"generation_id": "gen-1",
		"hook_event_name": "stop",
		"model": "gpt-5"
	}`)
	if err := HandleHook(bytes.NewReader(raw), "cursor"); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	got := readSession(SessionPath(repo, wt, "cursor", "cursor-conv"))
	if got.Status != StatusDone {
		t.Errorf("status = %q, want %q", got.Status, StatusDone)
	}
	if got.TurnID != "gen-1" || got.Model != "gpt-5" {
		t.Errorf("metadata = %#v, want turn/model preserved", got)
	}
}

func TestHandleHook_CursorSessionEndRemovesFiles(t *testing.T) {
	repo, wt := setupWorktreeHome(t)
	dir := SessionDir(repo, wt, "cursor")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "s1.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "s1.files"), []byte("/x"), 0o644)
	os.WriteFile(filepath.Join(dir, "s1.timeline"), []byte("{}"), 0o644)

	raw := []byte(`{
		"session_id": "s1",
		"hook_event_name": "sessionEnd"
	}`)
	if err := HandleHook(bytes.NewReader(raw), "cursor"); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	for _, ext := range []string{".json", ".files", ".timeline"} {
		if _, err := os.Stat(filepath.Join(dir, "s1"+ext)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", ext)
		}
	}
}

func TestHandleHook_CursorTracksEditedFiles(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	raw := []byte(`{
		"session_id": "s1",
		"hook_event_name": "afterFileEdit",
		"file_path": "/tmp/a.go"
	}`)
	if err := HandleHook(bytes.NewReader(raw), "cursor"); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	data, err := os.ReadFile(FilesPathForHarness(repo, wt, "cursor", "s1"))
	if err != nil {
		t.Fatalf("read files list: %v", err)
	}
	if strings.TrimSpace(string(data)) != "/tmp/a.go" {
		t.Fatalf("files list = %q, want edited path", data)
	}
}

func TestHandleHook_SessionEndRemovesFiles(t *testing.T) {
	repo, wt := setupWorktreeHome(t)
	dir := SessionDir(repo, wt, "claude")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "s1.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "s1.files"), []byte("/x"), 0o644)
	os.WriteFile(filepath.Join(dir, "s1.timeline"), []byte("{}"), 0o644)

	in := HookInput{SessionID: "s1", HookEventName: "SessionEnd"}
	raw, _ := json.Marshal(in)
	if err := HandleHook(bytes.NewReader(raw)); err != nil {
		t.Fatalf("HandleHook: %v", err)
	}

	for _, ext := range []string{".json", ".files", ".timeline"} {
		if _, err := os.Stat(filepath.Join(dir, "s1"+ext)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", ext)
		}
	}
}

func TestHandleHook_NotificationRespectsBusy(t *testing.T) {
	repo, wt := setupWorktreeHome(t)
	// Prime session as BUSY
	raw, _ := json.Marshal(HookInput{SessionID: "s1", HookEventName: "UserPromptSubmit"})
	HandleHook(bytes.NewReader(raw))

	// Now Notification should NOT overwrite BUSY with WAIT
	raw, _ = json.Marshal(HookInput{SessionID: "s1", HookEventName: "Notification"})
	HandleHook(bytes.NewReader(raw))

	got := readSession(SessionPath(repo, wt, "claude", "s1"))
	if got.Status != StatusBusy {
		t.Errorf("status = %q, want BUSY (Notification should not override)", got.Status)
	}
}

func TestHandleHook_NotificationSetsWaitWhenIdle(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	raw, _ := json.Marshal(HookInput{SessionID: "s1", HookEventName: "Notification"})
	HandleHook(bytes.NewReader(raw))

	got := readSession(SessionPath(repo, wt, "claude", "s1"))
	if got.Status != StatusWait {
		t.Errorf("status = %q, want WAIT", got.Status)
	}
}

func TestHandleHook_TracksWriteFiles(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	send := func(ev HookInput) {
		raw, _ := json.Marshal(ev)
		HandleHook(bytes.NewReader(raw))
	}
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Write", FilePath: "/a"})
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Edit", FilePath: "/b"})
	// Duplicate should be skipped
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Write", FilePath: "/a"})

	data, err := os.ReadFile(FilesPathForHarness(repo, wt, "claude", "s1"))
	if err != nil {
		t.Fatalf("read files list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("files list has %d entries, want 2: %q", len(lines), lines)
	}
}

func TestHandleHook_TimelineDedupes(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	send := func(ev HookInput) {
		raw, _ := json.Marshal(ev)
		HandleHook(bytes.NewReader(raw))
	}
	// BUSY, BUSY, BUSY -> one timeline entry
	send(HookInput{SessionID: "s1", HookEventName: "UserPromptSubmit"})
	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Read"})
	send(HookInput{SessionID: "s1", HookEventName: "PostToolUse", ToolName: "Read"})
	// DONE -> second entry
	send(HookInput{SessionID: "s1", HookEventName: "Stop"})

	data, err := os.ReadFile(TimelinePathForHarness(repo, wt, "claude", "s1"))
	if err != nil {
		t.Fatalf("read timeline: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("timeline has %d entries, want 2", len(lines))
	}
}

func TestHandleHook_OutsideWorktreeIsNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	prevCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(prevCwd) })
	os.Chdir(home) // not under willow's managed worktrees

	raw, _ := json.Marshal(HookInput{SessionID: "s1", HookEventName: "Stop"})
	if err := HandleHook(bytes.NewReader(raw)); err != nil {
		t.Errorf("HandleHook should swallow out-of-worktree: %v", err)
	}
	// No status dir should have been created
	if entries, err := os.ReadDir(StatusDir()); err == nil && len(entries) > 0 {
		t.Errorf("status dir should be empty: %v", entries)
	}
}

func TestHandleHook_MissingSessionIDIsNoop(t *testing.T) {
	setupWorktreeHome(t)

	raw, _ := json.Marshal(HookInput{HookEventName: "Stop"})
	if err := HandleHook(bytes.NewReader(raw)); err != nil {
		t.Errorf("HandleHook should swallow missing session_id: %v", err)
	}
}

func TestHandleHook_InvalidJSONIsNoop(t *testing.T) {
	setupWorktreeHome(t)

	if err := HandleHook(bytes.NewReader([]byte("not json"))); err != nil {
		t.Errorf("HandleHook should swallow bad JSON: %v", err)
	}
}

func TestHandleHook_PreservesStartTime(t *testing.T) {
	repo, wt := setupWorktreeHome(t)

	send := func(ev HookInput) {
		raw, _ := json.Marshal(ev)
		HandleHook(bytes.NewReader(raw))
	}
	send(HookInput{SessionID: "s1", HookEventName: "UserPromptSubmit"})
	first := readSession(SessionPath(repo, wt, "claude", "s1")).StartTime

	send(HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Read"})
	second := readSession(SessionPath(repo, wt, "claude", "s1")).StartTime

	if !first.Equal(second) {
		t.Errorf("start_time changed: %v → %v", first, second)
	}
}

func TestWriteSessionConcurrentWriters(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "session.json")

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- writeSession(path, SessionStatus{
				SessionID: "s1",
				Status:    StatusBusy,
				ToolCount: i,
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("writeSession returned error under concurrent writes: %v", err)
		}
	}

	if got := readSession(path); got.SessionID != "s1" {
		t.Fatalf("session_id = %q, want s1", got.SessionID)
	}
	matches, err := filepath.Glob(filepath.Join(home, "session.json.*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("leftover temp files: %v", matches)
	}
}

func TestClickForKey_BuildsTmuxTarget(t *testing.T) {
	t.Setenv("TMUX", "/tmp/sock,123,0")
	t.Setenv("__CFBundleIdentifier", "com.googlecode.iterm2")

	click := clickForKey("myrepo/feat-x")
	if click == nil {
		t.Fatal("clickForKey returned nil")
	}
	if click.Sender != "com.googlecode.iterm2" {
		t.Errorf("Sender = %q, want iterm2 bundle", click.Sender)
	}
	if click.Group != "willow-myrepo/feat-x" {
		t.Errorf("Group = %q, want willow-myrepo/feat-x", click.Group)
	}
	for _, want := range []string{"focus", "--session 'myrepo/feat-x'", "--tmux-socket '/tmp/sock'"} {
		if !strings.Contains(click.Execute, want) {
			t.Errorf("Execute = %q, missing %q", click.Execute, want)
		}
	}
}

func TestClickForKey_NoTmuxOmitsSocket(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("__CFBundleIdentifier", "com.apple.Terminal")

	click := clickForKey("myrepo/feat-x")
	if click == nil {
		t.Fatal("clickForKey returned nil")
	}
	if strings.Contains(click.Execute, "--tmux-socket") {
		t.Errorf("Execute should omit --tmux-socket outside tmux: %q", click.Execute)
	}
}

func TestResolveWorktree_UsesConfiguredBaseDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WILLOW_BASE_DIR", filepath.Join(home, "custom willow"))

	wtPath := filepath.Join(home, "custom willow", "worktrees", "configrepo", "branch-x")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	prevCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(prevCwd) })
	if err := os.Chdir(wtPath); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	repo, wt, ok := resolveWorktree()
	if !ok {
		t.Fatal("resolveWorktree() = not ok, want ok")
	}
	if repo != "configrepo" || wt != "branch-x" {
		t.Fatalf("resolveWorktree() = (%q, %q), want (%q, %q)", repo, wt, "configrepo", "branch-x")
	}
}
