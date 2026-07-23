package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/focus"
	"github.com/iamrajjoshi/willow/internal/notify"
)

const focusCaptureTimeout = time.Second

var runTmuxCapture = func(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).Output()
}

var (
	supportsNotificationClick       = notify.SupportsClick
	dispatchNotificationTransitions = dispatchNotifications
)

// HookInput models the Claude-shaped hook payload. It stays exported for tests
// and callers that construct hook JSON directly; production handling is
// normalized per harness.
type HookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	FilePath      string `json:"file_path"`
}

// HandleHook reads a JSON event from r, updates status files for the current
// worktree, and fires a desktop notification on BUSY → DONE/WAIT transitions.
// Returns nil when there's nothing to do (not a willow worktree, missing
// session_id, etc.) so the hook never blocks Claude Code. Notification
// Notification dispatch remains best-effort so hook failures never block the
// agent harness.
func HandleHook(r io.Reader, harnessIDs ...string) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	harnessID := harness.ClaudeID
	if len(harnessIDs) > 0 && harnessIDs[0] != "" {
		harnessID = harness.NormalizeID(harnessIDs[0])
	}
	h, err := harness.MustGet(harnessID)
	if err != nil {
		return err
	}
	in, ok := h.NormalizeHook(raw)
	if !ok || in.SessionID == "" {
		return nil
	}

	repo, wt, ok := resolveWorktree()
	if !ok {
		return nil
	}

	destDir := SessionDir(repo, wt, h.ID())
	destFile := SessionPath(repo, wt, h.ID(), in.SessionID)

	if in.EventName == "SessionEnd" || in.EventName == "sessionEnd" {
		_ = removeSessionArtifacts(repo, wt, h.ID(), in.SessionID)
		fireNotifications(repo, wt)
		return nil
	}

	status, skip := computeStatus(h.ID(), in, destFile)
	if skip {
		return nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir status dir: %w", err)
	}

	now := time.Now().UTC()
	prev := readSession(destFile)

	toolCount := prev.ToolCount
	if in.EventName == "PreToolUse" || in.EventName == "preToolUse" {
		toolCount++
	}

	startTime := prev.StartTime
	if startTime.IsZero() {
		startTime = now
	}

	toolField := ""
	if in.EventName == "PreToolUse" || in.EventName == "preToolUse" || in.EventName == "PermissionRequest" {
		toolField = in.ToolName
	}

	for _, filePath := range in.FilesTouched {
		appendFileList(FilesPathForHarness(repo, wt, h.ID(), in.SessionID), filePath)
	}

	currentFocusTarget := focusTargetForStatus(repo+"/"+wt, status, prev.FocusTarget)
	session := SessionStatus{
		Harness:        h.ID(),
		Status:         status,
		SessionID:      in.SessionID,
		Tool:           toolField,
		ToolCount:      toolCount,
		Model:          nonEmpty(in.Model, prev.Model),
		TurnID:         nonEmpty(in.TurnID, prev.TurnID),
		PermissionMode: nonEmpty(in.PermissionMode, prev.PermissionMode),
		Timestamp:      now,
		StartTime:      startTime,
		Worktree:       wt,
		FocusTarget:    currentFocusTarget,
	}
	if err := writeSession(destFile, session); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	appendTimeline(TimelinePathForHarness(repo, wt, h.ID(), in.SessionID), status, now)

	fireNotifications(repo, wt)
	return nil
}

func focusTargetForStatus(key string, status Status, previous *focus.Target) *focus.Target {
	if status != StatusDone && status != StatusWait {
		return previous
	}
	return focusTargetForStatusConfig(key, status, previous, config.Load("").Notify, supportsNotificationClick())
}

func focusTargetForStatusConfig(key string, status Status, previous *focus.Target, cfg config.NotifyConfig, supportsClick bool) *focus.Target {
	if status != StatusDone && status != StatusWait {
		return previous
	}
	desktopEnabled := cfg.Desktop == nil || *cfg.Desktop
	if !desktopEnabled || cfg.Command != "" || !supportsClick {
		return previous
	}
	return captureFocusTarget(key, previous)
}

func captureFocusTarget(key string, previous *focus.Target) *focus.Target {
	tmuxSocket := focus.SocketFromEnv(os.Getenv("TMUX"))
	tmuxPane := os.Getenv("TMUX_PANE")
	termBundle := os.Getenv("__CFBundleIdentifier")
	if tmuxSocket == "" && tmuxPane == "" && termBundle == "" && previous != nil {
		target := *previous
		target.Session = key
		return &target
	}

	target := focus.Target{
		Session:    key,
		TmuxSocket: tmuxSocket,
		TermBundle: termBundle,
	}
	if tmuxSocket == "" {
		return &target
	}

	target.TmuxPane = tmuxPane
	if path, err := exec.LookPath("tmux"); err == nil {
		if absolutePath, err := filepath.Abs(path); err == nil {
			target.TmuxPath = absolutePath
		} else {
			target.TmuxPath = path
		}
	}
	target.TmuxClient = tmuxClientForSession(target.TmuxPath, target.TmuxSocket, target.TmuxPane, key)

	if previous != nil && previous.TmuxSocket == target.TmuxSocket {
		if target.TmuxPath == "" {
			target.TmuxPath = previous.TmuxPath
		}
		if target.TmuxPane == "" {
			target.TmuxPane = previous.TmuxPane
		}
		if target.TmuxClient == "" {
			target.TmuxClient = previous.TmuxClient
		}
		if target.TermBundle == "" {
			target.TermBundle = previous.TermBundle
		}
	}
	return &target
}

func tmuxClientForSession(tmuxPath, socket, pane, session string) string {
	if tmuxPath == "" || socket == "" || pane == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), focusCaptureTimeout)
	defer cancel()
	out, err := runTmuxCapture(
		ctx, tmuxPath, "-S", socket,
		"display-message", "-p", "-t", pane,
		"#{client_name}\t#{client_session}",
	)
	if err != nil {
		return ""
	}
	client, clientSession, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok || clientSession != session {
		return ""
	}
	return client
}

func notificationFocusTarget(key string, sessions []*SessionStatus) (focus.Target, string) {
	best, _ := highestPrioritySession(sessions)
	var target focus.Target
	if best != nil && best.FocusTarget != nil {
		target = *best.FocusTarget
	}
	target.Session = key

	identity := struct {
		Harness   string       `json:"harness,omitempty"`
		SessionID string       `json:"session_id,omitempty"`
		Target    focus.Target `json:"target"`
	}{Target: target}
	if best != nil {
		identity.Harness = best.Harness
		identity.SessionID = best.SessionID
	}
	encoded, _ := json.Marshal(identity)
	return target, string(encoded)
}

func clickForTarget(key string, target focus.Target) *notify.Click {
	click := &notify.Click{Group: "willow-" + key}
	if !focus.CanFocus(target) {
		return click
	}
	willowPath, err := os.Executable()
	if err != nil {
		return click
	}
	click.Execute = focus.ExecuteCommand(willowPath, target)
	return click
}

// computeStatus returns the new status for this event plus a skip flag.
// Skip=true means the event should not touch the status file
// (e.g. Notification arriving while session is already DONE or BUSY).
func computeStatus(harnessID string, in harness.NormalizedHook, destFile string) (Status, bool) {
	if harnessID == harness.CodexID {
		switch in.EventName {
		case "UserPromptSubmit", "PreToolUse", "PostToolUse":
			return StatusBusy, false
		case "PermissionRequest":
			return StatusWait, false
		case "Stop":
			return StatusDone, false
		}
		return StatusBusy, false
	}

	if harnessID == harness.CursorID {
		if in.EventName == "stop" {
			return StatusDone, false
		}
		return StatusBusy, false
	}

	switch in.EventName {
	case "UserPromptSubmit":
		return StatusBusy, false
	case "PreToolUse":
		if isClaudeWaitTool(in.ToolName) {
			return StatusWait, false
		}
		return StatusBusy, false
	case "PostToolUse":
		if isClaudeWaitTool(in.ToolName) {
			return StatusWait, false
		}
		return StatusBusy, false
	case "Stop":
		return StatusDone, false
	case "Notification":
		cur := readSession(destFile).Status
		if cur == StatusDone || cur == StatusBusy {
			return "", true
		}
		return StatusWait, false
	}
	return StatusBusy, false
}

// isClaudeWaitTool reports whether a Claude Code tool invocation should
// transition the session to WAIT instead of BUSY.
func isClaudeWaitTool(name string) bool {
	return name == "AskUserQuestion" || name == "ExitPlanMode"
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// resolveWorktree returns (repo, worktreeDir) if cwd is under willow's
// configured worktrees directory.
func resolveWorktree() (string, string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	wtRoot := config.WorktreesDir()
	if resolved, err := filepath.EvalSymlinks(wtRoot); err == nil {
		wtRoot = resolved
	}

	rel, err := filepath.Rel(wtRoot, cwd)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", "", false
	}

	parts := strings.SplitN(rel, string(filepath.Separator), 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func readSession(path string) SessionStatus {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionStatus{}
	}
	var s SessionStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return SessionStatus{}
	}
	return s
}

func writeSession(path string, s SessionStatus) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// appendFileList appends a file path to sessionID.files only if it is not
// already present. Best-effort; errors are swallowed.
func appendFileList(path, filePath string) {
	existing, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(existing), "\n") {
		if line == filePath {
			return
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, filePath)
}

// appendTimeline appends a JSONL entry only when the status changes relative
// to the last recorded entry. Best-effort.
func appendTimeline(path string, status Status, ts time.Time) {
	last := lastTimelineStatus(path)
	if last == status {
		return
	}
	entry, err := json.Marshal(TimelineEntry{Status: status, Time: ts})
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(entry)
	f.Write([]byte("\n"))
}

func lastTimelineStatus(path string) Status {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastLine []byte
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lastLine = append(lastLine[:0], scanner.Bytes()...)
	}
	if len(lastLine) == 0 {
		return ""
	}
	var e TimelineEntry
	if err := json.Unmarshal(lastLine, &e); err != nil {
		return ""
	}
	return e.Status
}

// fireNotifications serializes grouped clickable dispatches for one worktree
// so a stale notification cannot overtake a newer target replacement. Other
// dispatchers run after the short state locks, so they cannot block sibling
// hooks while running external commands.
func fireNotifications(repo, wt string) {
	key := repo + "/" + wt
	cfg := config.Load("")
	disabled := cfg.Notify.Desktop != nil && !*cfg.Notify.Desktop && cfg.Notify.Command == ""
	clickable := !disabled && cfg.Notify.Command == "" &&
		(cfg.Notify.Desktop == nil || *cfg.Notify.Desktop) &&
		supportsNotificationClick()

	var transitions []Transition
	var clickTarget focus.Target
	collectTransitions := func() error {
		sessions := ReadAllSessions(repo, wt)
		agg := AggregateStatus(sessions)
		var targetIdentity string
		clickTarget, targetIdentity = notificationFocusTarget(key, sessions)
		_ = withNotifyLock(func() error {
			transitions = detectNotificationTransitions(key, agg.Status, targetIdentity, NotifyStateFile())
			return nil
		})
		return nil
	}

	if clickable {
		_ = withNotifyKeyLock(key, func() error {
			if err := collectTransitions(); err != nil || len(transitions) == 0 {
				return err
			}
			dispatchNotificationTransitions(cfg, key, transitions, clickTarget, true)
			return nil
		})
		return
	}

	_ = withNotifyKeyLock(key, collectTransitions)
	if len(transitions) == 0 || disabled {
		return
	}
	dispatchNotificationTransitions(cfg, key, transitions, clickTarget, false)
}

func dispatchNotifications(cfg *config.Config, key string, transitions []Transition, clickTarget focus.Target, allowRefresh bool) {
	for _, tr := range transitions {
		if tr.Key != key {
			continue
		}
		if tr.Refresh && !allowRefresh {
			continue
		}
		var body string
		switch tr.ToStatus {
		case StatusDone:
			body = fmt.Sprintf("\u2705 %s finished", tr.Key)
		case StatusWait:
			body = fmt.Sprintf("\u23F3 %s needs input", tr.Key)
		default:
			continue
		}

		switch {
		case cfg.Notify.Command != "":
			_ = notify.SendCustom(cfg.Notify.Command, "willow", body)
		case cfg.Notify.Desktop == nil || *cfg.Notify.Desktop:
			_ = notify.SendWithClick("willow", body, clickForTarget(tr.Key, clickTarget))
		}
	}
}
