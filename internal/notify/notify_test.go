package notify

import "testing"

func TestSend_NoPanic(t *testing.T) {
	// Send may fail in CI (no display), just verify it doesn't panic.
	_ = Send("test title", "test body")
}

func TestSendCustom_NoPanic(t *testing.T) {
	err := SendCustom("true", "test title", "test body")
	if err != nil {
		t.Errorf("SendCustom() with 'true' command should not error: %v", err)
	}
}

func TestSendWithClick_NilClickFallsBack(t *testing.T) {
	// A nil click must behave like a plain Send (no panic, no crash in CI).
	_ = SendWithClick("test title", "test body", nil)
}

func TestSendWithClick_NoPanic(t *testing.T) {
	_ = SendWithClick("test title", "test body", &Click{
		Execute: "true",
		Sender:  "com.apple.Terminal",
		Group:   "willow-repo/wt",
	})
}
