// Package focus foregrounds an agent's session on notification click.
package focus

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	bundleITerm2   = "com.googlecode.iterm2"
	bundleTerminal = "com.apple.Terminal"
	bundleGhostty  = "com.mitchellh.ghostty"
	selectedMarker = "selected"
	osascriptPath  = "/usr/bin/osascript"
)

var (
	errTmuxFocusSuperseded       = errors.New("focus: tmux attachment superseded")
	errTmuxAttachmentLockTimeout = errors.New("focus: timed out waiting for another attachment")
)

const (
	tmuxAttachmentLockTimeout   = 3 * time.Second
	tmuxAttachmentClientTimeout = 2 * time.Second
	tmuxAttachmentLeaseTimeout  = 10 * time.Second
	tmuxAttachmentPollInterval  = 50 * time.Millisecond
	tmuxAttachmentLeaseOption   = "@willow-focus-attachment"
	tmuxAttachmentTargetOption  = "@willow-focus-target"
)

// Indirected for tests.
var (
	runCmd = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
	writeTerminalSequence = func(tty string, sequence []byte) error {
		file, err := os.OpenFile(tty, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(sequence)
		if closeErr := file.Close(); writeErr == nil {
			writeErr = closeErr
		}
		return writeErr
	}
	withTmuxAttachmentLock    = lockTmuxAttachment
	waitForAttachedTmuxClient = pollForAttachedTmuxClient
)

// Target is a click destination, captured at fire time (the detached handler has no env).
type Target struct {
	Session    string `json:"session"`               // "repo/wtDir": both the tmux session name and the --tab-title tab
	TmuxPath   string `json:"tmux_path,omitempty"`   // absolute tmux path; click handlers do not inherit the shell PATH
	TmuxSocket string `json:"tmux_socket,omitempty"` // tmux socket path; empty when the agent wasn't in tmux
	TmuxPane   string `json:"tmux_pane,omitempty"`   // agent pane id; empty when the agent wasn't in tmux
	TmuxClient string `json:"tmux_client,omitempty"` // originating client; keeps multi-client focus in the captured terminal
	TermBundle string `json:"term_bundle,omitempty"` // host terminal bundle id, from __CFBundleIdentifier
}

// CanFocus reports whether a captured target has enough context for a click
// action. Registering another terminal driver automatically makes its bundle
// eligible here.
func CanFocus(t Target) bool {
	if t.TmuxPath != "" && t.TmuxSocket != "" {
		return true
	}
	return terminalDriverForBundle(t.TermBundle) != nil
}

// Focus foregrounds the agent's session; steps are best-effort.
func Focus(t Target) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("focus: unsupported platform %q", runtime.GOOS)
	}
	return focusTarget(t)
}

func focusTarget(t Target) error {
	var focusErr error
	terminalHandled := false
	if t.TmuxPath != "" && t.TmuxSocket != "" {
		var client tmuxClient
		client, focusErr = focusTmuxSession(t)
		if errors.Is(focusErr, errTmuxFocusSuperseded) {
			return nil
		}
		if focusErr != nil {
			if err := selectTab(t); err == nil {
				focusErr = nil
			}
		} else if client.name != "" {
			focusErr = foregroundTmuxClient(t, client)
			if errors.Is(focusErr, errTmuxFocusSuperseded) {
				return nil
			}
			terminalHandled = true
		}
	} else {
		focusErr = selectTab(t)
	}

	if !terminalHandled {
		if err := activate(t.TermBundle); err != nil && focusErr == nil {
			focusErr = err
		}
	}
	return focusErr
}

func focusTmuxSession(t Target) (tmuxClient, error) {
	exactSession := "=" + t.Session
	request := newTmuxFocusRequest(t.Session)
	for {
		preparation, err := prepareTmuxFocus(t, exactSession, request)
		if errors.Is(err, errTmuxAttachmentLockTimeout) {
			continue
		}
		if err != nil {
			return tmuxClient{}, err
		}
		if preparation.client.name != "" {
			preparation.client.focusRequest = request
			return preparation.client, nil
		}

		if preparation.claimed {
			if err := openLeasedTmuxSession(t, preparation.lease); err != nil {
				for {
					clearErr := clearOwnedTmuxAttachmentLease(t, preparation.lease)
					if !errors.Is(clearErr, errTmuxAttachmentLockTimeout) {
						break
					}
				}
				for {
					current, currentErr := tmuxFocusRequestIsCurrent(t, request)
					if errors.Is(currentErr, errTmuxAttachmentLockTimeout) {
						continue
					}
					if currentErr != nil {
						return tmuxClient{}, currentErr
					}
					if !current {
						return tmuxClient{}, errTmuxFocusSuperseded
					}
					break
				}
				return tmuxClient{}, err
			}
		}

		if client := waitForAttachedTmuxClient(t); client.name != "" {
			focused, err := finishTmuxAttachment(t, exactSession, preparation.lease, request)
			if errors.Is(err, errTmuxAttachmentLockTimeout) {
				continue
			}
			if err != nil {
				return tmuxClient{}, err
			}
			if focused.name != "" {
				focused.focusRequest = request
				return focused, nil
			}
		}
		active, err := tmuxAttachmentRequestIsActive(t, preparation.lease, request)
		if errors.Is(err, errTmuxAttachmentLockTimeout) {
			continue
		}
		if err != nil {
			return tmuxClient{}, err
		}
		if active {
			continue
		}
		// The opener never registered a client. Re-enter the serialized
		// preparation so only the latest request can claim a replacement.
	}
}

func foregroundTmuxClient(t Target, client tmuxClient) error {
	for {
		err := withTmuxAttachmentLock(t.TmuxSocket, func() error {
			currentRequest, err := readTmuxFocusRequest(t)
			if err != nil {
				return err
			}
			if currentRequest.value != client.focusRequest.value {
				return errTmuxFocusSuperseded
			}
			if client.tty != "" && selectTerminal(client.tty, t.Session, t.TermBundle, client.termName, client.currentPath) == nil {
				return nil
			}
			return activate(t.TermBundle)
		})
		if !errors.Is(err, errTmuxAttachmentLockTimeout) {
			return err
		}
	}
}

func requireTmuxSession(t Target, exactSession string) error {
	if err := runCmd(t.TmuxPath, "-S", t.TmuxSocket, "has-session", "-t", exactSession); err != nil {
		return fmt.Errorf("focus: tmux session %q not found: %w", t.Session, err)
	}
	return nil
}

func listTmuxClient(t Target) (tmuxClient, error) {
	out, err := runCmdOutput(t.TmuxPath, "-S", t.TmuxSocket, "list-clients", "-F", "#{client_activity}\t#{client_name}\t#{client_tty}\t#{client_termname}")
	if err != nil {
		return tmuxClient{}, fmt.Errorf("focus: list tmux clients: %w", err)
	}
	return preferredClient(out, t.TmuxClient), nil
}

func switchTmuxClient(t Target, exactSession string, client tmuxClient) (tmuxClient, error) {
	if err := runCmd(t.TmuxPath, "-S", t.TmuxSocket, "switch-client", "-c", client.name, "-t", exactSession); err != nil {
		return tmuxClient{}, fmt.Errorf("focus: switch tmux client %q: %w", client.name, err)
	}
	return tmuxClientWithCurrentPath(t, exactSession, client), nil
}

func tmuxClientWithCurrentPath(t Target, exactSession string, client tmuxClient) tmuxClient {
	if out, err := runCmdOutput(t.TmuxPath, "-S", t.TmuxSocket, "display-message", "-p", "-t", exactSession, "#{pane_current_path}"); err == nil {
		client.currentPath = strings.TrimSuffix(string(out), "\n")
	}
	return client
}

type tmuxAttachmentLease struct {
	value     string
	expiresAt time.Time
}

func (l tmuxAttachmentLease) active() bool {
	return l.value != "" && time.Now().Before(l.expiresAt)
}

type tmuxFocusRequest struct {
	value         string
	startedAt     int64
	pid           int64
	nonce         int64
	targetSession string
}

func (r tmuxFocusRequest) newerThan(other tmuxFocusRequest) bool {
	if r.startedAt != other.startedAt {
		return r.startedAt > other.startedAt
	}
	if r.pid != other.pid {
		return r.pid > other.pid
	}
	return r.nonce > other.nonce
}

type tmuxFocusPreparation struct {
	client  tmuxClient
	lease   tmuxAttachmentLease
	claimed bool
}

func prepareTmuxFocus(t Target, exactSession string, request tmuxFocusRequest) (tmuxFocusPreparation, error) {
	var preparation tmuxFocusPreparation
	err := withTmuxAttachmentLock(t.TmuxSocket, func() error {
		if err := requireTmuxSession(t, exactSession); err != nil {
			return err
		}

		client, listErr := listTmuxClient(t)
		if listErr != nil {
			return listErr
		}

		currentRequest, err := readTmuxFocusRequest(t)
		if err != nil {
			return err
		}
		if currentRequest.value != request.value && currentRequest.newerThan(request) {
			return errTmuxFocusSuperseded
		}
		if currentRequest.value != request.value {
			if err := markTmuxFocusRequest(t, request); err != nil {
				return err
			}
		}
		selectTmuxPane(t)

		if client.name != "" {
			if err := clearTmuxAttachmentLease(t); err != nil {
				return err
			}
			preparation.client, err = switchTmuxClient(t, exactSession, client)
			return err
		}

		lease, err := readTmuxAttachmentLease(t)
		if err != nil {
			return err
		}
		if lease.active() {
			preparation.lease = lease
			return nil
		}

		lease = newTmuxAttachmentLease()
		if err := markTmuxAttachmentLease(t, lease); err != nil {
			return err
		}

		// A delayed prior attachment can appear while its lease is expiring. Check
		// once more after claiming the replacement lease and before opening a window.
		client, err = listTmuxClient(t)
		if err != nil {
			_ = clearTmuxAttachmentLease(t)
			return err
		}
		if client.name != "" {
			if err := clearTmuxAttachmentLease(t); err != nil {
				return err
			}
			preparation.client, err = switchTmuxClient(t, exactSession, client)
			return err
		}

		preparation.lease = lease
		preparation.claimed = true
		return nil
	})
	return preparation, err
}

func finishTmuxAttachment(t Target, exactSession string, owned tmuxAttachmentLease, request tmuxFocusRequest) (tmuxClient, error) {
	var finished tmuxClient
	err := withTmuxAttachmentLock(t.TmuxSocket, func() error {
		currentRequest, err := readTmuxFocusRequest(t)
		if err != nil {
			return err
		}
		if currentRequest.value != request.value {
			return errTmuxFocusSuperseded
		}
		currentLease, err := readTmuxAttachmentLease(t)
		if err != nil {
			return err
		}
		if currentLease.value != owned.value {
			return errTmuxFocusSuperseded
		}
		if err := requireTmuxSession(t, exactSession); err != nil {
			return err
		}
		client, err := listTmuxClient(t)
		if err != nil {
			return err
		}
		if client.name == "" {
			return nil
		}
		if err := clearTmuxAttachmentLease(t); err != nil {
			return err
		}
		finished, err = switchTmuxClient(t, exactSession, client)
		return err
	})
	return finished, err
}

type tmuxAttachmentAction func() error

func lockTmuxAttachment(socket string, action tmuxAttachmentAction) error {
	return lockTmuxAttachmentAt(tmuxAttachmentLockPath(socket), tmuxAttachmentLockTimeout, tmuxAttachmentPollInterval, action)
}

func tmuxAttachmentLockPath(socket string) string {
	socketHash := sha256.Sum256([]byte(socket))
	return fmt.Sprintf("/tmp/willow-focus-%d-%x.lock", os.Getuid(), socketHash[:8])
}

func lockTmuxAttachmentAt(lockPath string, timeout, interval time.Duration, action tmuxAttachmentAction) error {
	lock, err := acquireTmuxAttachmentLock(lockPath, timeout, interval)
	if err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}()
	return action()
}

func acquireTmuxAttachmentLock(lockPath string, timeout, interval time.Duration) (*os.File, error) {
	// Keep the file after unlocking so waiters always contend on the same inode.
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("focus: open tmux attachment lock: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = lock.Close()
			return nil, fmt.Errorf("focus: lock tmux attachment: %w", err)
		}
		if !time.Now().Before(deadline) {
			_ = lock.Close()
			return nil, errTmuxAttachmentLockTimeout
		}
		time.Sleep(interval)
	}
}

func readTmuxAttachmentLease(t Target) (tmuxAttachmentLease, error) {
	out, err := runCmdOutput(t.TmuxPath, "-S", t.TmuxSocket, "show-options", "-s", "-qv", tmuxAttachmentLeaseOption)
	if err != nil {
		return tmuxAttachmentLease{}, fmt.Errorf("focus: read tmux attachment lease: %w", err)
	}
	value := strings.TrimSpace(string(out))
	expiresValue, _, _ := strings.Cut(value, ":")
	expires, err := strconv.ParseInt(expiresValue, 10, 64)
	if err != nil {
		return tmuxAttachmentLease{value: value}, nil
	}
	return tmuxAttachmentLease{value: value, expiresAt: time.UnixMilli(expires)}, nil
}

func newTmuxAttachmentLease() tmuxAttachmentLease {
	expiresAt := time.Now().Add(tmuxAttachmentLeaseTimeout)
	value := fmt.Sprintf("%d:%d:%d", expiresAt.UnixMilli(), os.Getpid(), time.Now().UnixNano())
	return tmuxAttachmentLease{value: value, expiresAt: expiresAt}
}

func readTmuxFocusRequest(t Target) (tmuxFocusRequest, error) {
	out, err := runCmdOutput(t.TmuxPath, "-S", t.TmuxSocket, "show-options", "-s", "-qv", tmuxAttachmentTargetOption)
	if err != nil {
		return tmuxFocusRequest{}, fmt.Errorf("focus: read tmux attachment target: %w", err)
	}
	value := strings.TrimSpace(string(out))
	fields := strings.SplitN(value, ":", 4)
	if len(fields) != 4 {
		return tmuxFocusRequest{value: value}, nil
	}
	startedAt, startedErr := strconv.ParseInt(fields[0], 10, 64)
	pid, pidErr := strconv.ParseInt(fields[1], 10, 64)
	nonce, nonceErr := strconv.ParseInt(fields[2], 10, 64)
	target, targetErr := base64.RawURLEncoding.DecodeString(fields[3])
	if startedErr != nil || pidErr != nil || nonceErr != nil || targetErr != nil {
		return tmuxFocusRequest{value: value}, nil
	}
	return tmuxFocusRequest{
		value:         value,
		startedAt:     startedAt,
		pid:           pid,
		nonce:         nonce,
		targetSession: string(target),
	}, nil
}

func newTmuxFocusRequest(targetSession string) tmuxFocusRequest {
	startedAt := time.Now().UnixNano()
	pid := int64(os.Getpid())
	nonce := time.Now().UnixNano()
	encodedTarget := base64.RawURLEncoding.EncodeToString([]byte(targetSession))
	value := fmt.Sprintf("%d:%d:%d:%s", startedAt, pid, nonce, encodedTarget)
	return tmuxFocusRequest{
		value:         value,
		startedAt:     startedAt,
		pid:           pid,
		nonce:         nonce,
		targetSession: targetSession,
	}
}

func markTmuxAttachmentLease(t Target, lease tmuxAttachmentLease) error {
	if err := runCmd(t.TmuxPath, "-S", t.TmuxSocket, "set-option", "-s", tmuxAttachmentLeaseOption, lease.value); err != nil {
		return fmt.Errorf("focus: mark tmux attachment in progress: %w", err)
	}
	return nil
}

func markTmuxFocusRequest(t Target, request tmuxFocusRequest) error {
	if err := runCmd(t.TmuxPath, "-S", t.TmuxSocket, "set-option", "-s", tmuxAttachmentTargetOption, request.value); err != nil {
		return fmt.Errorf("focus: mark tmux attachment target: %w", err)
	}
	return nil
}

func clearTmuxAttachmentLease(t Target) error {
	if err := runCmd(t.TmuxPath, "-S", t.TmuxSocket, "set-option", "-s", "-q", "-u", tmuxAttachmentLeaseOption); err != nil {
		return fmt.Errorf("focus: clear tmux attachment lease: %w", err)
	}
	return nil
}

func clearOwnedTmuxAttachmentLease(t Target, owned tmuxAttachmentLease) error {
	return withTmuxAttachmentLock(t.TmuxSocket, func() error {
		current, err := readTmuxAttachmentLease(t)
		if err != nil {
			return err
		}
		if current.value != owned.value {
			return nil
		}
		return clearTmuxAttachmentLease(t)
	})
}

func tmuxFocusRequestIsCurrent(t Target, request tmuxFocusRequest) (bool, error) {
	current := false
	err := withTmuxAttachmentLock(t.TmuxSocket, func() error {
		stored, err := readTmuxFocusRequest(t)
		if err != nil {
			return err
		}
		current = stored.value == request.value
		return nil
	})
	return current, err
}

func tmuxAttachmentRequestIsActive(t Target, expectedLease tmuxAttachmentLease, request tmuxFocusRequest) (bool, error) {
	active := false
	err := withTmuxAttachmentLock(t.TmuxSocket, func() error {
		storedRequest, err := readTmuxFocusRequest(t)
		if err != nil {
			return err
		}
		if storedRequest.value != request.value {
			return errTmuxFocusSuperseded
		}
		lease, err := readTmuxAttachmentLease(t)
		if err != nil {
			return err
		}
		if lease.value != "" && lease.value != expectedLease.value {
			return errTmuxFocusSuperseded
		}
		active = lease.active()
		return nil
	})
	return active, err
}

func pollForAttachedTmuxClient(t Target) tmuxClient {
	deadline := time.Now().Add(tmuxAttachmentClientTimeout)
	for {
		client, err := listTmuxClient(t)
		if err == nil && client.name != "" {
			return client
		}
		if !time.Now().Before(deadline) {
			return tmuxClient{}
		}
		time.Sleep(tmuxAttachmentPollInterval)
	}
}

func selectTmuxPane(t Target) {
	if t.TmuxPane == "" {
		return
	}
	out, err := runCmdOutput(t.TmuxPath, "-S", t.TmuxSocket, "display-message", "-p", "-t", t.TmuxPane, "#{session_name}")
	if err != nil || strings.TrimSpace(string(out)) != t.Session {
		return
	}
	_ = runCmd(t.TmuxPath, "-S", t.TmuxSocket, "select-window", "-t", t.TmuxPane)
	_ = runCmd(t.TmuxPath, "-S", t.TmuxSocket, "select-pane", "-t", t.TmuxPane)
}

type tmuxClient struct {
	name         string
	tty          string
	termName     string
	currentPath  string
	activity     int64
	focusRequest tmuxFocusRequest
}

func mostRecentClient(out []byte) string {
	return preferredClient(out, "").name
}

func preferredClient(out []byte, preferred string) tmuxClient {
	var best tmuxClient
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		client := tmuxClient{}
		switch len(fields) {
		case 1:
			client.name = fields[0]
		case 2:
			client.name = fields[1]
			client.activity, _ = strconv.ParseInt(strings.TrimSpace(fields[0]), 10, 64)
		case 3:
			client.name = fields[1]
			client.tty = fields[2]
			client.activity, _ = strconv.ParseInt(strings.TrimSpace(fields[0]), 10, 64)
		case 4:
			client.name = fields[1]
			client.tty = fields[2]
			client.termName = fields[3]
			client.activity, _ = strconv.ParseInt(strings.TrimSpace(fields[0]), 10, 64)
		}
		client.name = strings.TrimSpace(client.name)
		client.tty = strings.TrimSpace(client.tty)
		client.termName = strings.TrimSpace(client.termName)
		if client.name != "" && client.name == preferred {
			return client
		}
		if client.name != "" && (best.name == "" || client.activity > best.activity) {
			best = client
		}
	}
	return best
}

func openTmuxSession(t Target) error {
	return openTmuxSessionCommand(t, tmuxAttachCommand(t))
}

func openLeasedTmuxSession(t Target, lease tmuxAttachmentLease) error {
	return openTmuxSessionCommand(t, tmuxLeasedAttachCommand(t, lease))
}

func openTmuxSessionCommand(t Target, command string) error {
	driver := terminalDriverForBundle(t.TermBundle)
	if driver == nil {
		return fmt.Errorf("focus: no attached tmux client and terminal %q cannot open one", t.TermBundle)
	}
	if err := driver.openAttachment(command); err != nil {
		return fmt.Errorf("focus: open tmux session: %w", err)
	}
	return nil
}

func iterm2OpenScript(command string) string {
	return fmt.Sprintf(`tell application id %q
	create window with default profile command %q
end tell`, bundleITerm2, command)
}

func terminalOpenScript(command string) string {
	return fmt.Sprintf("tell application \"Terminal\" to do script %q", command)
}

func ghosttyOpenScript(command string) string {
	return fmt.Sprintf(`tell application id %q
	set cfg to new surface configuration
	set command of cfg to %q
	set wait after command of cfg to false
	set newWin to new window with configuration cfg
	activate window newWin
end tell`, bundleGhostty, command)
}

func tmuxAttachCommand(t Target) string {
	return strings.Join([]string{
		shellQuote(t.TmuxPath),
		"-S", shellQuote(t.TmuxSocket),
		"attach-session", "-t", shellQuote("=" + t.Session),
	}, " ")
}

func tmuxLeasedAttachCommand(t Target, lease tmuxAttachmentLease) string {
	// tmux inserts the selected if-shell command into the same client queue, so
	// replacing the lease cannot interleave between this check and the attach.
	// The opener can attach to any live session because the current request owner
	// switches the registered client to its exact target immediately afterward.
	attach := "attach-session"
	return strings.Join([]string{
		shellQuote(t.TmuxPath),
		"-S", shellQuote(t.TmuxSocket),
		"if-shell", "-F", shellQuote(tmuxAttachmentLeaseCondition(lease)), shellQuote(attach), shellQuote(""),
	}, " ")
}

func tmuxAttachmentLeaseCondition(lease tmuxAttachmentLease) string {
	return fmt.Sprintf("#{==:#{%s},%s}", tmuxAttachmentLeaseOption, lease.value)
}

// activate brings the terminal app forward without opening a new window.
func activate(bundle string) error {
	if bundle == "" {
		return nil
	}
	script := fmt.Sprintf("tell application id %q to activate", bundle)
	return runCmd(osascriptPath, "-e", script)
}

func selectTerminal(tty, title, preferredBundle, termName, currentPath string) error {
	target := terminalClientTarget{
		tty:            tty,
		title:          title,
		termName:       termName,
		currentPath:    currentPath,
		capturedBundle: preferredBundle,
	}
	for _, driver := range terminalDriversForClient(preferredBundle) {
		if err := driver.focusClient(target); err == nil {
			return nil
		}
	}
	return fmt.Errorf("focus: terminal for tty %q or title %q not found", tty, title)
}

func ghosttyWorkingDirectories() map[string]string {
	out, err := runCmdOutput(osascriptPath, "-e", ghosttyWorkingDirectoriesScript())
	if err != nil {
		return nil
	}
	var directories map[string]string
	if json.Unmarshal(out, &directories) != nil {
		return nil
	}
	return directories
}

func ghosttyTerminalIDByWorkingDirectory(path string) string {
	out, err := runCmdOutput(osascriptPath, "-e", ghosttySelectWorkingDirectoryScript(path))
	if err != nil {
		return ""
	}
	terminalID := strings.TrimSpace(string(out))
	if terminalID == "not found" {
		return ""
	}
	return terminalID
}

func iterm2SelectTTYScript(tty string) string {
	return fmt.Sprintf(`if application id %q is not running then return "not found"
tell application id %q
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if tty of s is equal to %q then
					select w
					select t
					tell s to select
					activate
					return %q
				end if
			end repeat
		end repeat
	end repeat
end tell
return "not found"`, bundleITerm2, bundleITerm2, tty, selectedMarker)
}

func terminalSelectTTYScript(tty string) string {
	return fmt.Sprintf(`if application id %q is not running then return "not found"
tell application id %q
	repeat with w in windows
		repeat with t in tabs of w
			if tty of t is equal to %q then
				set selected of t to true
				set frontmost of w to true
				activate
				return %q
			end if
		end repeat
	end repeat
end tell
return "not found"`, bundleTerminal, bundleTerminal, tty, selectedMarker)
}

// selectTab focuses the terminal titled after the session.
func selectTab(t Target) error {
	driver := terminalDriverForBundle(t.TermBundle)
	if driver == nil {
		return fmt.Errorf("focus: terminal %q does not support tab selection", t.TermBundle)
	}
	err := driver.focusTitle(t.Session)
	if errors.Is(err, errTerminalNotFound) {
		return fmt.Errorf("focus: terminal tab %q not found", t.Session)
	}
	if err != nil {
		return fmt.Errorf("focus: select terminal tab: %w", err)
	}
	return nil
}

func iterm2SelectScript(title string) string {
	return fmt.Sprintf(`tell application id %q
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
					if name of s is equal to %q then
						select w
						select t
						tell s to select
						return %q
					end if
				end repeat
			end repeat
		end repeat
		return "not found"
	end tell`, bundleITerm2, title, selectedMarker)
}

func terminalSelectScript(title string) string {
	return fmt.Sprintf(`tell application "Terminal"
		repeat with w in windows
			repeat with t in tabs of w
				if custom title of t is equal to %q then
					set selected of t to true
					set frontmost of w to true
					return %q
				end if
			end repeat
		end repeat
		return "not found"
	end tell`, title, selectedMarker)
}

func ghosttySelectScript(title string) string {
	return fmt.Sprintf(`if application id %q is not running then return "not found"
tell application id %q
	repeat 5 times
		repeat with term in terminals
			if name of term is equal to %q then
				focus term
				return %q
			end if
		end repeat
		delay 0.02
	end repeat
end tell
return "not found"`, bundleGhostty, bundleGhostty, title, selectedMarker)
}

func ghosttySelectTTYScript(tty string) string {
	return fmt.Sprintf(`if application id %q is not running then return "not found"
tell application id %q
	repeat with term in terminals
		try
			if tty of term is equal to %q then
				focus term
				return %q
			end if
		end try
	end repeat
end tell
return "not found"`, bundleGhostty, bundleGhostty, tty, selectedMarker)
}

func ghosttySelectWorkingDirectoryScript(path string) string {
	return fmt.Sprintf(`if application id %q is not running then return ""
tell application id %q
	repeat 5 times
		repeat with term in terminals
			if working directory of term is equal to %q then
				focus term
				return id of term
			end if
		end repeat
		delay 0.02
	end repeat
end tell
return ""`, bundleGhostty, bundleGhostty, path)
}

func ghosttyWorkingDirectoriesScript() string {
	return fmt.Sprintf(`use framework "Foundation"
set resultMap to current application's NSMutableDictionary's dictionary()
if application id %q is not running then return "{}"
tell application id %q
	repeat with term in terminals
		set terminalID to id of term
		set terminalPath to working directory of term
		if terminalPath is missing value then set terminalPath to ""
		resultMap's setObject:terminalPath forKey:terminalID
	end repeat
end tell
set jsonData to current application's NSJSONSerialization's dataWithJSONObject:resultMap options:0 |error|:(missing value)
set jsonString to current application's NSString's alloc()'s initWithData:jsonData encoding:(current application's NSUTF8StringEncoding)
return jsonString as text`, bundleGhostty, bundleGhostty)
}

func ghosttyMarkerPath(tty string) string {
	return "/tmp/willow-focus-" + strings.TrimPrefix(tty, "/dev/")
}

func isGhosttyTerm(termName string) bool {
	return strings.HasSuffix(strings.ToLower(termName), "ghostty")
}

func terminalWorkingDirectorySequence(path string) []byte {
	if path == "" {
		return []byte("\x1b]7;\x07")
	}
	uri := (&url.URL{Scheme: "file", Host: "localhost", Path: path}).String()
	return []byte("\x1b]7;" + uri + "\x07")
}

// ExecuteCommand builds the on-click command; willowPath must be absolute (no PATH when detached).
func ExecuteCommand(willowPath string, t Target) string {
	parts := []string{shellQuote(willowPath), "focus", "--session", shellQuote(t.Session)}
	if t.TmuxPath != "" {
		parts = append(parts, "--tmux-path", shellQuote(t.TmuxPath))
	}
	if t.TmuxSocket != "" {
		parts = append(parts, "--tmux-socket", shellQuote(t.TmuxSocket))
	}
	if t.TmuxPane != "" {
		parts = append(parts, "--tmux-pane", shellQuote(t.TmuxPane))
	}
	if t.TmuxClient != "" {
		parts = append(parts, "--tmux-client", shellQuote(t.TmuxClient))
	}
	if t.TermBundle != "" {
		parts = append(parts, "--term-bundle", shellQuote(t.TermBundle))
	}
	return strings.Join(parts, " ")
}

// SocketFromEnv extracts the socket path from a $TMUX value ("sock,pid,sess").
func SocketFromEnv(tmuxEnv string) string {
	if tmuxEnv == "" {
		return ""
	}
	lastComma := strings.LastIndexByte(tmuxEnv, ',')
	if lastComma < 0 {
		return tmuxEnv
	}
	secondLastComma := strings.LastIndexByte(tmuxEnv[:lastComma], ',')
	if secondLastComma < 0 {
		return tmuxEnv[:lastComma]
	}
	return tmuxEnv[:secondLastComma]
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
