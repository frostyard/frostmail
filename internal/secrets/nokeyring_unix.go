//go:build !windows

package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// noKeyring is the store of Open with NoFallback and no Secret Service: it
// keeps nothing, and every call fails with ErrNoKeyring. It never answers
// ErrNotFound, which would make sync ask the user to sign in again, and it
// never reads or writes the plain file.
type noKeyring struct {
	cause error // why no Secret Service answered
	hint  string
}

var _ Store = noKeyring{}

func (n noKeyring) fail(op, key string) error {
	return fmt.Errorf("secrets: %s %s: %w", op, key, errNoKeyring(n.cause.Error(), n.hint))
}

func (n noKeyring) Get(_ context.Context, key string) (string, error) { return "", n.fail("get", key) }

func (n noKeyring) Set(_ context.Context, key, _ string) error { return n.fail("set", key) }

func (n noKeyring) Delete(_ context.Context, key string) error { return n.fail("delete", key) }

// openNoKeyring makes the no-keyring store for file, the plain file Open
// does not use, and says so in the log.
func openNoKeyring(file string, cause error, opts Options, log *slog.Logger) Store {
	log.Error("no Secret Service: maild cannot keep secrets, and adding or signing in to an account will fail",
		"err", cause, "hint", opts.Hint)
	if _, err := os.Stat(file); err == nil {
		log.Warn("a secrets file stays as it is until a Secret Service answers, then its secrets move in once", "file", file)
	}
	return noKeyring{cause: cause, hint: opts.Hint}
}
