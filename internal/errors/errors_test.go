package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

func TestUserErrorWrappingAndDetection(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := User(cause)

	if err.Error() != "root cause" {
		t.Fatalf("Error() = %q, want root cause", err.Error())
	}
	if !stderrors.Is(err, cause) {
		t.Fatalf("User error should unwrap to cause")
	}
	if !IsUser(err) {
		t.Fatalf("IsUser() should identify User(cause)")
	}
	ue, ok := err.(interface{ IsUserError() bool })
	if !ok || !ue.IsUserError() {
		t.Fatalf("wrapped error should expose IsUserError")
	}
}

func TestUserf(t *testing.T) {
	err := Userf("hello %s", "willow")
	if err.Error() != "hello willow" {
		t.Fatalf("Userf() = %q, want formatted message", err.Error())
	}
	if !IsUser(err) {
		t.Fatalf("IsUser() should identify Userf error")
	}
	if IsUser(fmt.Errorf("plain")) {
		t.Fatalf("IsUser() should not identify plain errors")
	}
}
