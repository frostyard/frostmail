package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The Secret Service (org.freedesktop.secrets) on the session bus: GNOME
// Keyring, KWallet, KeePassXC (docs/design/accounts.md, Secrets; ADR-0011).
// Items live in the default collection, labeled "Frostmail: <key>" with the
// attributes application=frostmail and key=<key>; secrets travel in a plain
// session, since the bus is local.

const (
	ssName            = "org.freedesktop.secrets"
	ssPath            = dbus.ObjectPath("/org/freedesktop/secrets")
	ssService         = "org.freedesktop.Secret.Service"
	ssCollection      = "org.freedesktop.Secret.Collection"
	ssItem            = "org.freedesktop.Secret.Item"
	ssPrompt          = "org.freedesktop.Secret.Prompt"
	defaultCollection = dbus.ObjectPath("/org/freedesktop/secrets/aliases/default")
	noPrompt          = dbus.ObjectPath("/")
)

// ErrUnavailable means no Secret Service answered on the bus.
var ErrUnavailable = errors.New("secrets: no Secret Service")

// ErrDismissed means the user dismissed the service's unlock prompt.
var ErrDismissed = errors.New("secrets: the unlock prompt was dismissed")

// secretStruct is the Secret Service's Secret type (oayays).
type secretStruct struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// SecretService is a Store in the desktop's Secret Service.
type SecretService struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
	mu      sync.Mutex // one prompt at a time
}

var _ Store = (*SecretService)(nil)

// OpenSecretService opens a plain session with the Secret Service on conn,
// or returns an error wrapping ErrUnavailable.
func OpenSecretService(ctx context.Context, conn *dbus.Conn) (*SecretService, error) {
	var out dbus.Variant
	var session dbus.ObjectPath
	err := conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".OpenSession", 0, "plain", dbus.MakeVariant("")).
		Store(&out, &session)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &SecretService{conn: conn, session: session}, nil
}

func attributes(key string) map[string]string {
	return map[string]string{"application": "frostmail", "key": key}
}

// Get returns the secret stored under key, or ErrNotFound.
func (s *SecretService) Get(ctx context.Context, key string) (string, error) {
	item, err := s.find(ctx, key)
	if err != nil {
		return "", err
	}
	var sec secretStruct
	if err := s.conn.Object(ssName, item).CallWithContext(ctx, ssItem+".GetSecret", 0, s.session).Store(&sec); err != nil {
		return "", fmt.Errorf("secrets: get %s: %w", key, err)
	}
	return string(sec.Value), nil
}

// Set stores value under key, replacing any previous item.
func (s *SecretService) Set(ctx context.Context, key, value string) error {
	if err := s.unlock(ctx, []dbus.ObjectPath{defaultCollection}); err != nil {
		return fmt.Errorf("secrets: set %s: %w", key, err)
	}
	props := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant("Frostmail: " + key),
		ssItem + ".Attributes": dbus.MakeVariant(attributes(key)),
	}
	sec := secretStruct{Session: s.session, Value: []byte(value), ContentType: "text/plain; charset=utf8"}
	var item, prompt dbus.ObjectPath
	err := s.conn.Object(ssName, defaultCollection).CallWithContext(ctx, ssCollection+".CreateItem", 0, props, sec, true).
		Store(&item, &prompt)
	if err != nil {
		return fmt.Errorf("secrets: set %s: %w", key, err)
	}
	if prompt != noPrompt {
		if err := s.prompt(ctx, prompt); err != nil {
			return fmt.Errorf("secrets: set %s: %w", key, err)
		}
	}
	return nil
}

// Delete removes key; a missing key is not an error.
func (s *SecretService) Delete(ctx context.Context, key string) error {
	item, err := s.find(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var prompt dbus.ObjectPath
	if err := s.conn.Object(ssName, item).CallWithContext(ctx, ssItem+".Delete", 0).Store(&prompt); err != nil {
		return fmt.Errorf("secrets: delete %s: %w", key, err)
	}
	if prompt != noPrompt {
		if err := s.prompt(ctx, prompt); err != nil {
			return fmt.Errorf("secrets: delete %s: %w", key, err)
		}
	}
	return nil
}

// find returns the item holding key, unlocking it when needed.
func (s *SecretService) find(ctx context.Context, key string) (dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".SearchItems", 0, attributes(key)).
		Store(&unlocked, &locked)
	if err != nil {
		return "", fmt.Errorf("secrets: search %s: %w", key, err)
	}
	if len(unlocked) > 0 {
		return unlocked[0], nil
	}
	if len(locked) == 0 {
		return "", fmt.Errorf("secrets: get %s: %w", key, ErrNotFound)
	}
	if err := s.unlock(ctx, locked[:1]); err != nil {
		return "", fmt.Errorf("secrets: get %s: %w", key, err)
	}
	return locked[0], nil
}

// unlock unlocks objects, through the service's prompt when it asks for one.
func (s *SecretService) unlock(ctx context.Context, objects []dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	err := s.conn.Object(ssName, ssPath).CallWithContext(ctx, ssService+".Unlock", 0, objects).Store(&unlocked, &prompt)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if prompt == noPrompt {
		return nil
	}
	return s.prompt(ctx, prompt)
}

// prompt shows a service prompt and waits for its Completed signal.
func (s *SecretService) prompt(ctx context.Context, path dbus.ObjectPath) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(ssPrompt), dbus.WithMatchMember("Completed")}
	if err := s.conn.AddMatchSignalContext(ctx, match...); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	defer func() { _ = s.conn.RemoveMatchSignalContext(context.WithoutCancel(ctx), match...) }()
	signals := make(chan *dbus.Signal, 4)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)
	if err := s.conn.Object(ssName, path).CallWithContext(ctx, ssPrompt+".Prompt", 0, "").Err; err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signals:
			if sig.Path != path || sig.Name != ssPrompt+".Completed" || len(sig.Body) < 1 {
				continue
			}
			if dismissed, _ := sig.Body[0].(bool); dismissed {
				return ErrDismissed
			}
			return nil
		}
	}
}

// Open picks maild's secret store: the Secret Service on the session bus,
// unless FROSTMAIL_SECRETS=file or no service answers, in which case the
// file store at file (with a warning). With the Secret Service, secrets
// still in the file are moved over and the file is removed.
func Open(ctx context.Context, file string, log *slog.Logger) (Store, error) {
	fs := NewFile(file)
	if os.Getenv("FROSTMAIL_SECRETS") == "file" {
		return fs, nil
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

// migrate moves every secret from the file store into the Secret Service,
// reading each back before the file is removed.
func migrate(ctx context.Context, from *File, to Store) error {
	all, err := from.All()
	if err != nil || len(all) == 0 {
		return err
	}
	for key, value := range all {
		if err := to.Set(ctx, key, value); err != nil {
			return fmt.Errorf("secrets: move %s to the Secret Service: %w", key, err)
		}
		got, err := to.Get(ctx, key)
		if err != nil || got != value {
			return fmt.Errorf("secrets: %s did not read back from the Secret Service: %v", key, err)
		}
	}
	return from.Remove()
}
