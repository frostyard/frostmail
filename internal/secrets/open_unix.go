//go:build !windows

package secrets

import (
	"context"
	"log/slog"
	"os"

	"github.com/godbus/dbus/v5"
)

// Open picks maild's secret store by FROSTMAIL_SECRETS:
//   - "file": the plain file at file, with a warning (CI, development, a
//     server with no systemd credentials);
//   - "sealed": the file beside it sealed with the key of the systemd
//     credential frostmail-secrets-key, never a fallback (ADR-0044); a
//     plain file moves in once and is removed;
//   - empty: the Secret Service on the session bus, or, when no service
//     answers, the plain file with a warning. With the Secret Service,
//     secrets still in the file are moved over and the file is removed;
//   - anything else is an error.
func Open(ctx context.Context, file string, log *slog.Logger) (Store, error) {
	fs := NewFile(file)
	switch v := os.Getenv("FROSTMAIL_SECRETS"); v {
	case "file":
		log.Warn("keeping secrets in a plain file; a server seals them with FROSTMAIL_SECRETS=sealed (ADR-0044)", "file", file)
		return fs, nil
	case "sealed":
		return openSealed(ctx, file, log)
	case "":
	default:
		return nil, errUnknownMode(v)
	}
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err == nil {
		var ss *SecretService
		if ss, err = OpenSecretService(ctx, conn); err == nil {
			if err := migrate(ctx, fs, ss); err != nil {
				return nil, err
			}
			return ss, nil
		}
		_ = conn.Close()
	}
	log.Warn("no Secret Service; keeping secrets in a file", "file", file, "err", err)
	return fs, nil
}

// OpenReader opens the store Open would pick, to read it and nothing else
// (mailctl export, with maild stopped): it never moves a secret out of the
// file and never writes. With the Secret Service or the sealed file, a key
// it does not hold is also looked up in the plain file, a leftover Open has
// not migrated yet. With "sealed" it needs the credential's key.
func OpenReader(ctx context.Context, file string) (Store, error) {
	fs := NewFile(file)
	switch v := os.Getenv("FROSTMAIL_SECRETS"); v {
	case "file":
		return readOnly{first: fs}, nil
	case "sealed":
		return openSealedReader(file)
	case "":
	default:
		return nil, errUnknownMode(v)
	}
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err == nil {
		var ss *SecretService
		if ss, err = OpenSecretService(ctx, conn); err == nil {
			return readOnly{first: ss, second: fs}, nil
		}
		_ = conn.Close()
	}
	return readOnly{first: fs}, nil
}
