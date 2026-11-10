package mailsync

import "context"

// SetEraseHook makes m call hook before each mailbox transaction of EraseOld,
// with the account's ID; nil removes it. A test uses it to change an account's
// settings between the pass's list read and the transaction that erases
// (erase_test.go, TestEraseOldReadsSettingsInTheTransaction).
func SetEraseHook(m *Manager, hook func(ctx context.Context, accountID int64)) {
	m.eraseHook = hook
}
