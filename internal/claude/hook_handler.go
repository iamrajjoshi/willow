package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/notify"
)

// HookInput is the JSON payload Claude Code pipes to stdin for every hook event.
type HookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	FilePath      string `json:"file_path"`
}

// HandleHook reads a JSON event from r, updates status files for the current
// worktree, and fires a desktop notification on BUSY → DONE/WAIT transitions.
// Errors are written to errOut; the function returns nil when there's nothing
// to do (not a willow worktree, missing session_id, etc.) so the hook never
// blocks Claude Code.
func HandleHook(r io.Reader, errOut io.Writer) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var in HookInput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &in); err != nil {
		return nil
	}
	if in.SessionID == "" {
		return nil
	}

	repo, wt, ok := resolveWorktree()
	if !ok {
		return nil
	}

	destDir := filepath.Join(StatusDir(), repo, wt)
	destFile := filepath.Join(destDir, in.SessionID+".json")

	if in.HookEventName == "SessionEnd" {
		os.Remove(destFile)
		os.Remove(filepath.Join(destDir, in.SessionID+".files"))
		os.Remove(filepath.Join(destDir, in.SessionID+".timeline"))
		return nil
	}

	status, skip := computeStatus(in, destFile)
	if skip {
		return nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir status dir: %w", err)
	}

	now := time.Now().UTC()
	prev := readSession(destFile)

	toolCount := prev.ToolCount
	if in.HookEventName == "PreToolUse" {
		toolCount++
	}

	startTime := prev.StartTime
	if startTime.IsZero() {
		startTime = now
	}

	toolField := ""
	if in.HookEventName == "PreToolUse" {
		toolField = in.ToolName
	}

	if in.HookEventName == "PreToolUse" && isWriteTool(in.ToolName) && in.FilePath != "" {
		appendFileList(filepath.Join(destDir, in.SessionID+".files"), in.FilePath)
	}

	session := SessionStatus{
		Status:    status,
		SessionID: in.SessionID,
		Tool:      toolField,
		ToolCount: toolCount,
		Timestamp: now,
		StartTime: startTime,
		Worktree:  wt,
	}
	if err := writeSession(destFile, session); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	appendTimeline(filepath.Join(destDir, in.SessionID+".timeline"), status, now)

	fireNotifications(repo, wt)
	return nil
}

// computeStatus returns the new status for this event plus a skip flag.
// Skip=true means the event should not touch the status file
// (e.g. Notification arriving while session is already DONE or BUSY).
func computeStatus(in HookInput, destFile string) (Status, bool) {
	switch in.HookEventName {
	case "UserPromptSubmit":
		return StatusBusy, false
	case "PreToolUse":
		if isWaitTool(in.ToolName) {
			return StatusWait, false
		}
		return StatusBusy, false
	case "PostToolUse":
		if isWaitTool(in.ToolName) {
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

// isWaitTool reports whether a tool invocation should transition the session
// to WAIT instead of BUSY. These tools block on the user for input and must
// surface as "needs input" in the picker and notifications.
func isWaitTool(name string) bool {
	return name == "AskUserQuestion" || name == "ExitPlanMode"
}

func isWriteTool(name string) bool {
	return name == "Write" || name == "Edit" || name == "NotebookEdit"
}

// resolveWorktree returns (repo, worktreeDir) if cwd is under ~/.willow/worktrees.
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

// fireNotifications aggregates sessions for this worktree, detects transitions
// against the saved state, and dispatches notifications. Aggregation and
// transition detection happen inside the flock so concurrent hooks across
// sibling sessions observe a consistent view of the state file — otherwise
// two hooks could each read the prior state, each compute "BUSY→DONE", and
// emit duplicate notifications.
func fireNotifications(repo, wt string) {
	key := repo + "/" + wt

	var transitions []Transition
	_ = withNotifyLock(func() error {
		sessions := ReadAllSessions(repo, wt)
		agg := AggregateStatus(sessions)
		transitions = DetectTransitions(
			map[string]Status{key: agg.Status},
			NotifyStateFile(),
		)
		return nil
	})
	if len(transitions) == 0 {
		return
	}

	cfg := config.Load("")
	if cfg.Notify.Desktop != nil && !*cfg.Notify.Desktop && cfg.Notify.Command == "" {
		return
	}

	for _, tr := range transitions {
		if tr.Key != key {
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

		var err error
		switch {
		case cfg.Notify.Command != "":
			err = notify.SendCustom(cfg.Notify.Command, "willow", body)
		case cfg.Notify.Desktop == nil || *cfg.Notify.Desktop:
			err = notify.Send("willow", body)
		}
		if err != nil {
			sentry.CaptureException(err)
		}
	}
}
