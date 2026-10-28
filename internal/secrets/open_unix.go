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
//     server with no systemd credentials, a Flatpak whose user asked for it);
//   - "sealed": the file beside it sealed with the key of the systemd
//     credential frostmail-secrets-key, never a fallback (ADR-0044); a
//     plain file moves in once and is removed;
//   - empty: the Secret Service on the session bus, in opts.Namespace. With
//     it, secrets still in the file are moved over (all or nothing), with
//     one Info line, and the file is removed. With no Secret Service, the
//     plain file with a warning, unless opts.NoFallback: then a store that
//     fails every call with ErrNoKeyring, an Error line at start, and the
//     file, if there is one, left as it is (ADR-0046);
//   - anything else is an error.
//
// Options with the Flatpak's namespace and no store id are refused.
func Open(ctx context.Context, file string, log *slog.Logger, opts Options) (Store, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
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
		if ss, err = OpenSecretService(ctx, conn, opts.Namespace, opts.Hint); err == nil {
			moved, err := migrate(ctx, fs, ss)
			if err != nil {
				return nil, err
			}
			if moved > 0 {
				log.Info("moved the secrets from the file into the Secret Service",
					"file", file, "keys", moved, "application", opts.Namespace.application(), "store", opts.Namespace.Store)
			}
			return ss, nil
		}
		_ = conn.Close()
	}
	if opts.NoFallback {
		return openNoKeyring(file, err, opts, log), nil
	}
	log.Warn("no Secret Service; keeping secrets in a file", "file", file, "err", err)
	return fs, nil
}

// OpenReader opens the store Open would pick, to read it and nothing else
// (mailctl export, with maild stopped): it never moves a secret out of the
// file and never writes. With the Secret Service or the sealed file, a key
// it does not hold is also looked up in the plain file, a leftover Open has
// not migrated yet. With no Secret Service it reads the plain file alone,
// whether opts.NoFallback is set or not: the file is what an export needs
// of a store that Open has not moved yet. The Flatpak's namespace with no
// store id asks the Secret Service nothing, for no item can have that
// namespace yet. With "sealed" it needs the credential's key.
func OpenReader(ctx context.Context, file string, opts Options) (Store, error) {
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
	if opts.Namespace.flatpakWithoutStore() {
		return readOnly{first: fs}, nil
	}
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err == nil {
		var ss *SecretService
		if ss, err = OpenSecretService(ctx, conn, opts.Namespace, opts.Hint); err == nil {
			return readOnly{first: ss, second: fs}, nil
		}
		_ = conn.Close()
	}
	return readOnly{first: fs}, nil
}
