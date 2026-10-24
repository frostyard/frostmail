package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// CredentialName is the systemd credential that holds the key of the
	// sealed store (ADR-0044): the unit's
	// LoadCredentialEncrypted=frostmail-secrets-key puts it in
	// $CREDENTIALS_DIRECTORY.
	CredentialName = "frostmail-secrets-key"
	// SealedFileName is the sealed store's file in maild's data directory.
	SealedFileName = "secrets.sealed"

	sealedExt   = ".sealed"
	sealedMagic = "FMSEAL01"
	keySize     = 32 // AES-256
)

// NewSealedFile returns a File store at path whose bytes are sealed with
// AES-256-GCM under key, which must be 32 bytes (ADR-0044). A file is the
// magic "FMSEAL01", a fresh random 12-byte nonce, then the ciphertext with
// its tag; the magic is also the additional data. The rest is File's: the
// atomic write, mode 0600, a missing file is an empty store.
func NewSealedFile(path string, key []byte) (*File, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secrets: the sealing key is %d bytes, want %d", len(key), keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: the sealing key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: the sealing key: %w", err)
	}
	return &File{path: path, seal: sealer(aead), unseal: unsealer(aead)}, nil
}

func sealer(aead cipher.AEAD) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		nonce := make([]byte, aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("make a nonce: %w", err)
		}
		out := make([]byte, 0, len(sealedMagic)+len(nonce)+len(data)+aead.Overhead())
		out = append(out, sealedMagic...)
		out = append(out, nonce...)
		return aead.Seal(out, nonce, data, []byte(sealedMagic)), nil
	}
}

func unsealer(aead cipher.AEAD) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		n := len(sealedMagic) + aead.NonceSize()
		if len(data) < n+aead.Overhead() {
			return nil, fmt.Errorf("the file is %d bytes, too short to be a sealed store", len(data))
		}
		if !bytes.HasPrefix(data, []byte(sealedMagic)) {
			return nil, errors.New("the file is not a sealed store (wrong magic)")
		}
		plain, err := aead.Open(nil, data[len(sealedMagic):n], data[n:], []byte(sealedMagic))
		if err != nil {
			return nil, errors.New("the key does not open the file, or the file was changed")
		}
		return plain, nil
	}
}

// LoadKey reads the sealing key from the credential CredentialName in
// credDir ($CREDENTIALS_DIRECTORY). It must be exactly 32 raw bytes. An
// error names the credential and never holds the key.
func LoadKey(credDir string) ([]byte, error) {
	if credDir == "" {
		return nil, fmt.Errorf("$CREDENTIALS_DIRECTORY is not set: the unit needs LoadCredentialEncrypted=%s", CredentialName)
	}
	path := filepath.Join(credDir, CredentialName)
	key, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the credential %s is missing (%s): the unit needs LoadCredentialEncrypted=%s", CredentialName, path, CredentialName)
	}
	if err != nil {
		return nil, fmt.Errorf("read the credential %s (%s): %w", CredentialName, path, err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("the credential %s (%s) is %d bytes, want %d raw bytes", CredentialName, path, len(key), keySize)
	}
	return key, nil
}

// errUnknownMode refuses a FROSTMAIL_SECRETS value Open does not know.
func errUnknownMode(v string) error {
	return fmt.Errorf("secrets: FROSTMAIL_SECRETS=%q is not known: use file, sealed, or leave it unset", v)
}
