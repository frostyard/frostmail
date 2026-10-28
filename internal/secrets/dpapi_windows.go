package secrets

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewDPAPIFile returns a File store whose file is sealed with DPAPI for the
// user (CryptProtectData): only the user's Windows sign-in can unseal it,
// and it has no size cap, unlike Credential Manager (ADR-0038).
func NewDPAPIFile(path string) *File {
	return &File{path: path, seal: protect, unseal: unprotect}
}

// Open picks maild's secret store on Windows: the DPAPI-sealed file beside
// file (secrets.dpapi), unless FROSTMAIL_SECRETS=file keeps a development
// or test maild's secrets in the plain file. Secrets still in the plain
// file are moved over and the file is removed. FROSTMAIL_SECRETS=sealed
// (a systemd credential, ADR-0044) is an error here, and so is any other
// value but file. It has no Secret Service, so opts changes nothing but the
// refusal of the Flatpak's namespace with no store id.
func Open(ctx context.Context, file string, log *slog.Logger, opts Options) (Store, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	plain := NewFile(file)
	switch v := os.Getenv("FROSTMAIL_SECRETS"); v {
	case "file":
		return plain, nil
	case "":
	default:
		return nil, errNotOnWindows(v)
	}
	sealed := NewDPAPIFile(strings.TrimSuffix(file, filepath.Ext(file)) + ".dpapi")
	if _, err := migrate(ctx, plain, sealed); err != nil {
		return nil, err
	}
	log.Debug("secrets sealed with DPAPI", "file", sealed.path)
	return sealed, nil
}

// OpenReader opens the store Open would pick, to read it and nothing else
// (mailctl export, with maild stopped): the sealed file, or the plain file
// when FROSTMAIL_SECRETS=file. A key the sealed file does not hold is also
// looked up in the plain file, a leftover Open has not migrated yet. It
// never moves a secret and never writes. Like Open, it refuses "sealed" and
// any value it does not know.
func OpenReader(_ context.Context, file string, _ Options) (Store, error) {
	plain := NewFile(file)
	switch v := os.Getenv("FROSTMAIL_SECRETS"); v {
	case "file":
		return readOnly{first: plain}, nil
	case "":
	default:
		return nil, errNotOnWindows(v)
	}
	return readOnly{first: NewDPAPIFile(strings.TrimSuffix(file, filepath.Ext(file)) + ".dpapi"), second: plain}, nil
}

// errNotOnWindows refuses a FROSTMAIL_SECRETS value Windows has no store
// for: "sealed" needs systemd credentials, which Windows does not have.
func errNotOnWindows(v string) error {
	if v == "sealed" {
		return errors.New(`secrets: FROSTMAIL_SECRETS=sealed needs a systemd credential, which Windows does not have: use file or leave it unset`)
	}
	return errUnknownMode(v)
}

func protect(data []byte) ([]byte, error) {
	return crypt(data, windows.CryptProtectData)
}

func unprotect(data []byte) ([]byte, error) {
	return crypt(data, func(in *windows.DataBlob, _ *uint16, entropy *windows.DataBlob, reserved uintptr,
		prompt *windows.CryptProtectPromptStruct, flags uint32, out *windows.DataBlob) error {
		return windows.CryptUnprotectData(in, nil, entropy, reserved, prompt, flags, out)
	})
}

type cryptFunc func(in *windows.DataBlob, name *uint16, entropy *windows.DataBlob, reserved uintptr,
	prompt *windows.CryptProtectPromptStruct, flags uint32, out *windows.DataBlob) error

// crypt runs CryptProtectData or CryptUnprotectData over data, never
// prompting, and frees the output DPAPI allocated.
func crypt(data []byte, f cryptFunc) ([]byte, error) {
	if len(data) == 0 {
		data = []byte{}
	}
	in := windows.DataBlob{Size: uint32(len(data))}
	if len(data) > 0 {
		in.Data = &data[0]
	}
	var out windows.DataBlob
	if err := f(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
