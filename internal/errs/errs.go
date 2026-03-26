package errs

import (
	"errors"
	"fmt"
)

// userError wraps an error to mark it as an expected user-facing error
// (wrong directory, bad args, missing tools) that should not be
// reported to error tracking as a bug.
type userError struct {
	err error
}

func (e *userError) Error() string     { return e.err.Error() }
func (e *userError) Unwrap() error     { return e.err }
func (e *userError) IsUserError() bool { return true }

// User wraps err to mark it as a user error.
func User(err error) error {
	if err == nil {
		return nil
	}
	return &userError{err: err}
}

// Userf creates a new user error with a formatted message.
func Userf(format string, args ...any) error {
	return &userError{err: fmt.Errorf(format, args...)}
}

// IsUser reports whether err (or any error in its chain) is a user error.
func IsUser(err error) bool {
	var ue interface{ IsUserError() bool }
	return errors.As(err, &ue)
}
