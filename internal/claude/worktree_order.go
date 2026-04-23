package claude

// WorktreeUrgencyOrder returns the attention priority for worktree-focused
// views like pickers and tables.
// WAIT(0) < DONE unread(1) < BUSY(2) < DONE read(3) < IDLE(4) < everything else(5).
func WorktreeUrgencyOrder(s Status, unread bool) int {
	switch {
	case s == StatusWait:
		return 0
	case s == StatusDone && unread:
		return 1
	case s == StatusBusy:
		return 2
	case s == StatusDone:
		return 3
	case s == StatusIdle:
		return 4
	default:
		return 5
	}
}
