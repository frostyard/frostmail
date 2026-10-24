//go:build !windows

package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// sealedPath is the sealed file beside the plain secrets file: the same name
// with the extension .sealed.
func sealedPath(file string) string {
	return strings.TrimSuffix(file, filepath.Ext(file)) + sealedExt
}

// sealedStore opens the sealed file beside file with the key from
// $CREDENTIALS_DIRECTORY, and reads it once, so that a key that does not
// open it is refused here, by name, and never later.
func sealedStore(file string) (*File, error) {
	path := sealedPath(file)
	key, err := LoadKey(os.Getenv("CREDENTIALS_DIRECTORY"))
	if err != nil {
		return nil, fmt.Errorf("secrets: FROSTMAIL_SECRETS=sealed needs the key to open %s: %w", path, err)
	}
	sealed, err := NewSealedFile(path, key)
	if err != nil {
		return nil, err
	}
	if _, err := sealed.All(); err != nil {
		return nil, fmt.Errorf("secrets: the credential %s does not open %s: %w", CredentialName, path, err)
	}
	return sealed, nil
}

// openSealed is Open with FROSTMAIL_SECRETS=sealed: the sealed file beside
// file, never a fallback. A plain file moves in once. A missing sealed file
// is an empty store, the first start.
func openSealed(ctx context.Context, file string, log *slog.Logger) (Store, error) {
	sealed, err := sealedStore(file)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, NewFile(file), sealed); err != nil {
		return nil, err
	}
	log.Info("secrets sealed with a systemd credential", "file", sealed.path)
	return sealed, nil
}

// openSealedReader is OpenReader with FROSTMAIL_SECRETS=sealed: the sealed
// file, then the plain leftover Open has not moved in yet. It never writes.
func openSealedReader(file string) (Store, error) {
	sealed, err := sealedStore(file)
	if err != nil {
		return nil, err
	}
	return readOnly{first: sealed, second: NewFile(file)}, nil
}
