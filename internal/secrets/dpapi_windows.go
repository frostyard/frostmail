package secrets

import (
	"context"
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
// file are moved over and the file is removed.
func Open(ctx context.Context, file string, log *slog.Logger) (Store, error) {
	plain := NewFile(file)
	if os.Getenv("FROSTMAIL_SECRETS") == "file" {
		return plain, nil
	}
	sealed := NewDPAPIFile(strings.TrimSuffix(file, filepath.Ext(file)) + ".dpapi")
	if err := migrate(ctx, plain, sealed); err != nil {
		return nil, err
	}
	log.Debug("secrets sealed with DPAPI", "file", sealed.path)
	return sealed, nil
}

// OpenReader opens the store Open would pick, to read it and nothing else
// (mailctl export, with maild stopped): the sealed file, or the plain file
// when FROSTMAIL_SECRETS=file. A key the sealed file does not hold is also
// looked up in the plain file, a leftover Open has not migrated yet. It
// never moves a secret and never writes.
func OpenReader(_ context.Context, file string) (Store, error) {
	plain := NewFile(file)
	if os.Getenv("FROSTMAIL_SECRETS") == "file" {
		return readOnly{first: plain}, nil
	}
	return readOnly{first: NewDPAPIFile(strings.TrimSuffix(file, filepath.Ext(file)) + ".dpapi"), second: plain}, nil
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
