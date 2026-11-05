package mailsync

import (
	"context"
	"errors"
)

// CodeApplyFailed is the problem code of a store write that fails while it
// applies the server's state: the mailbox list, new headers, flag and label
// changes, removals and the sync state of a pass (ADR-0049). The record's
// MESSAGE is the error's text; the code names no mail.
const CodeApplyFailed = "sync.apply_failed"

// applyError marks a store write that applies the server's state, and
// keeps the error's text. Network, sign-in and stop errors are never
// marked.
type applyError struct{ err error }

func (e *applyError) Error() string { return e.err.Error() }
func (e *applyError) Unwrap() error { return e.err }

// applied marks err, the result of a store write that applies the server's
// state, with applyError. nil stays nil, and so does an error of a pass
// that stops (ctx ended): that is no failure of the store.
func applied(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &applyError{err}
}

// recordApply records CodeApplyFailed for err, a session's end, when a
// store write marked it, at most once in each failure streak: reported
// stays true until a pass completes again (healthy). It returns the new
// value of reported.
func (a *actor) recordApply(err error, healthy, reported bool) bool {
	if healthy {
		reported = false
	}
	var ae *applyError
	if !errors.As(err, &ae) {
		return reported
	}
	if !reported {
		a.m.cfg.Problems.Error(CodeApplyFailed, err)
	}
	return true
}
