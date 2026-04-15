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
