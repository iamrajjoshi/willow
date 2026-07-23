package notify

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type invocation struct {
	path string
	args []string
}

func recordingNotifier(t *testing.T, goos string, paths map[string]string) (notifier, *[]invocation) {
	t.Helper()
	var calls []invocation
	return notifier{
		goos: goos,
		lookPath: func(name string) (string, error) {
			if path := paths[name]; path != "" {
				return path, nil
			}
			return "", fmt.Errorf("%s not found", name)
		},
		run: func(_ context.Context, path string, args ...string) error {
			calls = append(calls, invocation{path: path, args: append([]string(nil), args...)})
			return nil
		},
	}, &calls
}

func TestSendWithClick_UsesTerminalNotifierWithoutFakeSender(t *testing.T) {
	n, calls := recordingNotifier(t, "darwin", map[string]string{
		"terminal-notifier": "/opt/homebrew/bin/terminal-notifier",
	})
	click := &Click{Execute: "'/opt/homebrew/bin/willow' focus --session 'repo/wt'", Group: "willow-repo/wt"}

	if err := n.sendWithClick("willow", "done", click); err != nil {
		t.Fatalf("sendWithClick: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %#v, want one terminal-notifier call", *calls)
	}
	wantArgs := []string{
		"-title", "willow",
		"-message", "done",
		"-execute", click.Execute,
		"-group", click.Group,
	}
	if got := (*calls)[0]; got.path != "/opt/homebrew/bin/terminal-notifier" || !reflect.DeepEqual(got.args, wantArgs) {
		t.Fatalf("terminal-notifier call = %#v, want path %q args %#v", got, "/opt/homebrew/bin/terminal-notifier", wantArgs)
	}
	if strings.Contains(strings.Join((*calls)[0].args, " "), "-sender") {
		t.Fatal("clickable notifications must not combine -sender with -execute")
	}
}

func TestSendWithClick_GroupOnlyOmitsExecute(t *testing.T) {
	n, calls := recordingNotifier(t, "darwin", map[string]string{
		"terminal-notifier": "/opt/homebrew/bin/terminal-notifier",
	})
	click := &Click{Group: "willow-repo/wt"}

	if err := n.sendWithClick("willow", "done", click); err != nil {
		t.Fatalf("sendWithClick: %v", err)
	}
	wantArgs := []string{"-title", "willow", "-message", "done", "-group", click.Group}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0].args, wantArgs) {
		t.Fatalf("terminal-notifier calls = %#v, want args %#v", *calls, wantArgs)
	}
}

func TestNotifierSupportsClick(t *testing.T) {
	darwin, _ := recordingNotifier(t, "darwin", map[string]string{
		"terminal-notifier": "/opt/homebrew/bin/terminal-notifier",
	})
	if !darwin.supportsClick() {
		t.Fatal("darwin notifier with terminal-notifier should support clicks")
	}

	missing, _ := recordingNotifier(t, "darwin", nil)
	if missing.supportsClick() {
		t.Fatal("darwin notifier without terminal-notifier should not support clicks")
	}

	linux, _ := recordingNotifier(t, "linux", nil)
	linux.lookPath = func(name string) (string, error) {
		t.Fatalf("linux click support should not look up %s", name)
		return "", nil
	}
	if linux.supportsClick() {
		t.Fatal("linux notifier should not report click support")
	}
}

func TestSendWithClick_BoundsTerminalNotifierRuntime(t *testing.T) {
	const cursorHookTimeout = 5 * time.Second
	if ClickDispatchTimeout >= cursorHookTimeout {
		t.Fatalf("click dispatch timeout %s must be below Cursor hook timeout %s", ClickDispatchTimeout, cursorHookTimeout)
	}

	n, _ := recordingNotifier(t, "darwin", map[string]string{"terminal-notifier": "/bin/terminal-notifier"})
	n.run = func(ctx context.Context, _ string, _ ...string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("terminal-notifier context should have a deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > ClickDispatchTimeout {
			t.Fatalf("terminal-notifier deadline remaining = %s, want (0, %s]", remaining, ClickDispatchTimeout)
		}
		return nil
	}

	if err := n.sendWithClick("willow", "done", &Click{Execute: "true"}); err != nil {
		t.Fatalf("sendWithClick: %v", err)
	}
}

func TestSendWithClick_PrimaryAndFallbackShareDeadline(t *testing.T) {
	n, _ := recordingNotifier(t, "darwin", map[string]string{"terminal-notifier": "/bin/terminal-notifier"})
	var deadlines []time.Time
	n.run = func(ctx context.Context, _ string, _ ...string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("notification command context should have a deadline")
		}
		deadlines = append(deadlines, deadline)
		return errors.New("failed")
	}

	if err := n.sendWithClick("willow", "done", &Click{Execute: "true"}); err == nil {
		t.Fatal("sendWithClick should return the primary and fallback failures")
	}
	if len(deadlines) != 2 || !deadlines[0].Equal(deadlines[1]) {
		t.Fatalf("command deadlines = %v, want one shared dispatch deadline", deadlines)
	}
}

func TestSendDarwin_BoundsRuntime(t *testing.T) {
	n, _ := recordingNotifier(t, "darwin", nil)
	n.run = func(ctx context.Context, _ string, _ ...string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("osascript context should have a deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > darwinNotifierTimeout {
			t.Fatalf("osascript deadline remaining = %s, want (0, %s]", remaining, darwinNotifierTimeout)
		}
		return nil
	}

	if err := n.sendDarwin("willow", "done"); err != nil {
		t.Fatalf("sendDarwin: %v", err)
	}
}

func TestSendWithClick_MissingHelperFallsBackToOsascript(t *testing.T) {
	n, calls := recordingNotifier(t, "darwin", nil)

	if err := n.sendWithClick("willow", "done", &Click{Execute: "true"}); err != nil {
		t.Fatalf("sendWithClick fallback: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].path != "osascript" {
		t.Fatalf("calls = %#v, want osascript fallback", *calls)
	}
	if got := strings.Join((*calls)[0].args, " "); !strings.Contains(got, "display notification") {
		t.Fatalf("osascript args = %q, want notification script", got)
	}
}

func TestSendWithClick_NilClickFallsBackWithoutLookup(t *testing.T) {
	n, calls := recordingNotifier(t, "darwin", nil)
	n.lookPath = func(name string) (string, error) {
		t.Fatalf("nil click should not look up %s", name)
		return "", nil
	}

	if err := n.sendWithClick("willow", "done", nil); err != nil {
		t.Fatalf("sendWithClick fallback: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].path != "osascript" {
		t.Fatalf("calls = %#v, want osascript fallback", *calls)
	}
}

func TestSendWithClick_LinuxBehaviorIsUnchanged(t *testing.T) {
	n, calls := recordingNotifier(t, "linux", map[string]string{"notify-send": "/usr/bin/notify-send"})

	if err := n.sendWithClick("willow", "done", &Click{Execute: "true"}); err != nil {
		t.Fatalf("sendWithClick: %v", err)
	}
	want := invocation{path: "/usr/bin/notify-send", args: []string{"-a", "willow", "willow", "done"}}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], want) {
		t.Fatalf("calls = %#v, want %#v", *calls, want)
	}
}

func TestSendWithClick_NotifierErrorFallsBackToOsascript(t *testing.T) {
	wantErr := errors.New("notifier failed")
	n, calls := recordingNotifier(t, "darwin", map[string]string{"terminal-notifier": "/bin/terminal-notifier"})
	recordRun := n.run
	n.run = func(ctx context.Context, path string, args ...string) error {
		if err := recordRun(ctx, path, args...); err != nil {
			return err
		}
		if path == "/bin/terminal-notifier" {
			return wantErr
		}
		return nil
	}

	if err := n.sendWithClick("willow", "done", &Click{Execute: "true"}); err != nil {
		t.Fatalf("sendWithClick fallback: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0].path != "/bin/terminal-notifier" || (*calls)[1].path != "osascript" {
		t.Fatalf("calls = %#v, want terminal-notifier then osascript", *calls)
	}
}

func TestSendWithClick_ReturnsBothNotifierAndFallbackErrors(t *testing.T) {
	notifierErr := errors.New("notifier failed")
	fallbackErr := errors.New("fallback failed")
	n, _ := recordingNotifier(t, "darwin", map[string]string{"terminal-notifier": "/bin/terminal-notifier"})
	n.run = func(_ context.Context, path string, _ ...string) error {
		if path == "/bin/terminal-notifier" {
			return notifierErr
		}
		return fallbackErr
	}

	err := n.sendWithClick("willow", "done", &Click{Execute: "true"})
	if !errors.Is(err, notifierErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("sendWithClick error = %v, want both %v and %v", err, notifierErr, fallbackErr)
	}
}

func TestSend_UnsupportedPlatform(t *testing.T) {
	n, _ := recordingNotifier(t, "plan9", nil)
	if err := n.send("willow", "done"); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("send error = %v, want unsupported platform", err)
	}
}

func TestSendCustom(t *testing.T) {
	if err := SendCustom("true", "test title", "test body"); err != nil {
		t.Errorf("SendCustom() with true command: %v", err)
	}
}
