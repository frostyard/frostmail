package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/frostyard/frostmail/internal/config"
)

// hostApplication is the application attribute of the host's items
// (ADR-0011).
const hostApplication = "frostmail"

// Namespace names the Secret Service items a store owns: its items have the
// attributes application=Application and, when Store is not empty,
// store=Store, beside key=<key>. Every search, write and delete of the store
// uses all of them, so that two namespaces never see each other's items.
type Namespace struct {
	Application string
	Store       string
}

// application is the namespace's application attribute: the zero value is
// the host's.
func (n Namespace) application() string {
	if n.Application == "" {
		return hostApplication
	}
	return n.Application
}

// flatpakWithoutStore reports a Flatpak namespace that has no store id: its
// items would have no store attribute, and so be shared.
func (n Namespace) flatpakWithoutStore() bool {
	return n.Application == config.AppID && n.Store == ""
}

// Options say where Open and OpenReader find the Secret Service's items and
// what they do without one.
type Options struct {
	// Namespace names the items; the zero value is the host's
	// (application=frostmail, no store attribute, ADR-0011).
	Namespace Namespace
	// NoFallback makes Open keep no plain file when FROSTMAIL_SECRETS is
	// unset and no Secret Service answers: its store fails every Get, Set
	// and Delete with an error that wraps ErrNoKeyring.
	NoFallback bool
	// Hint ends the message of ErrNoKeyring errors: how the user keeps
	// secrets in a file instead.
	Hint string
}

// HostOptions are the options of a maild outside a Flatpak.
func HostOptions() Options {
	return Options{Hint: "set FROSTMAIL_SECRETS=file to keep secrets in a file"}
}

// FlatpakOptions are the options of the Flatpak's maild (ADR-0046): its own
// namespace, with the id of its database as the store, and no plain file
// unless the user asks for one. An empty storeID is refused by Open.
func FlatpakOptions(storeID string) Options {
	return Options{
		Namespace:  Namespace{Application: config.AppID, Store: storeID},
		NoFallback: true,
		Hint:       "run `flatpak override --user --env=FROSTMAIL_SECRETS=file " + config.AppID + "` to keep secrets in a plain file instead",
	}
}

// OptionsFor picks the options of a maild: the Flatpak's, with storeID, when
// it runs in the Flatpak (config.InFlatpak), else the host's. maild and
// mailctl export use it, so that they look in the same namespace.
func OptionsFor(inFlatpak bool, storeID string) Options {
	if inFlatpak {
		return FlatpakOptions(storeID)
	}
	return HostOptions()
}

// check refuses options that would write items with no store attribute in
// the Flatpak's namespace.
func (o Options) check() error {
	if o.Namespace.flatpakWithoutStore() {
		return errors.New("secrets: the Flatpak's namespace needs a store id")
	}
	return nil
}

// ProbeKey is the key Probe writes, reads back and deletes.
const ProbeKey = "probe"

// Probe checks that s can keep a secret: it writes a random value that is
// not a secret under ProbeKey, reads it back, compares it and deletes it. A
// failure wraps its cause, so that ErrNoKeyring, ErrDismissed and
// ErrUnavailable stay testable. A probe item that was written is deleted
// again, whatever the failure.
func Probe(ctx context.Context, s Store) error {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("secrets: probe: draw a value: %w", err)
	}
	want := hex.EncodeToString(raw)
	if err := s.Set(ctx, ProbeKey, want); err != nil {
		return fmt.Errorf("secrets: probe: write: %w", err)
	}
	got, err := s.Get(ctx, ProbeKey)
	switch {
	case err != nil:
		err = fmt.Errorf("secrets: probe: read back: %w", err)
	case got != want:
		err = errors.New("secrets: probe: the store read back another value")
	}
	if derr := s.Delete(context.WithoutCancel(ctx), ProbeKey); derr != nil && err == nil {
		err = fmt.Errorf("secrets: probe: delete: %w", derr)
	}
	return err
}
