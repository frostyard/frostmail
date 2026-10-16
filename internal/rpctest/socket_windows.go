package rpctest

import (
	"crypto/rand"
	"encoding/hex"
)

// SocketPath is a new named pipe for a test maild; a pipe goes with its
// listener, so cleanup has nothing to remove.
func SocketPath() (path string, cleanup func(), err error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	return `\\.\pipe\frostmail-test-` + hex.EncodeToString(b), func() {}, nil
}
