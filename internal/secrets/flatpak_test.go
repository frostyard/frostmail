//go:build !windows

package secrets

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/config"
)

const flatpakOverride = "flatpak override --user --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail"

// openNamespace opens a store on the fake at addr in the Flatpak namespace
// of storeID, on a connection of its own.
func openNamespace(t *testing.T, addr string, ns Namespace, hint string) *SecretService {
	t.Helper()
	ss, err := OpenSecretService(t.Context(), connect(t, addr), ns, hint)
	if err != nil {
		t.Fatal(err)
	}
	return ss
}

func flatpakNS(storeID string) Namespace {
	return Namespace{Application: config.AppID, Store: storeID}
}

// itemAttrs returns the attributes of every item the fake holds.
func (f *fakeSecrets) itemAttrs() []map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]string
	for _, it := range f.items {
		out = append(out, maps.Clone(it.attrs))
	}
	return out
}

// Two stores, two ids, one service: each reads its own account 1, and a
// delete in one leaves the other's item.
func TestFlatpakStoresKeepTheirOwnSecrets(t *testing.T) {
	addr := privateBus(t)
	f := startFake(t, addr)
	ctx := t.Context()
	hint := FlatpakOptions("x").Hint
	a := openNamespace(t, addr, flatpakNS("aaaa"), hint)
	b := openNamespace(t, addr, flatpakNS("bbbb"), hint)
	key := AccountPassword(1)
	if err := a.Set(ctx, key, "password-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("store B reads store A's item: %v", err)
	}
	if err := b.Set(ctx, key, "password-b"); err != nil {
		t.Fatal(err)
	}
	if got := len(f.itemAttrs()); got != 2 {
		t.Fatalf("the service holds %d items, want 2: B's Set replaced A's item", got)
	}
	for store, want := range map[*SecretService]string{a: "password-a", b: "password-b"} {
		if got, err := store.Get(ctx, key); err != nil || got != want {
			t.Errorf("Get = %q, %v; want %q", got, err, want)
		}
	}
	if err := a.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("A reads its deleted item: %v", err)
	}
	if got, err := b.Get(ctx, key); err != nil || got != "password-b" {
		t.Errorf("B's item after A's delete = %q, %v", got, err)
	}
}

// The host's namespace and the Flatpak's do not see each other's items, in
// both directions, and a Set of one never replaces the other's item.
func TestHostAndFlatpakNamespacesDoNotSeeEachOther(t *testing.T) {
	addr := privateBus(t)
	f := startFake(t, addr)
	ctx := t.Context()
	host := openNamespace(t, addr, Namespace{}, HostOptions().Hint)
	flat := openNamespace(t, addr, flatpakNS("aaaa"), FlatpakOptions("aaaa").Hint)
	key := AccountPassword(1)

	if err := host.Set(ctx, key, "host-pw"); err != nil {
		t.Fatal(err)
	}
	if _, err := flat.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the Flatpak reads the host's item: %v", err)
	}
	if err := flat.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if got, err := host.Get(ctx, key); err != nil || got != "host-pw" {
		t.Fatalf("the Flatpak's delete changed the host's item: %q, %v", got, err)
	}
	if err := flat.Set(ctx, key, "flatpak-pw"); err != nil {
		t.Fatal(err)
	}
	if got := len(f.itemAttrs()); got != 2 {
		t.Fatalf("the service holds %d items, want 2", got)
	}
	if got, err := host.Get(ctx, key); err != nil || got != "host-pw" {
		t.Errorf("host Get = %q, %v", got, err)
	}
	if got, err := flat.Get(ctx, key); err != nil || got != "flatpak-pw" {
		t.Errorf("Flatpak Get = %q, %v", got, err)
	}
	if err := host.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if got, err := flat.Get(ctx, key); err != nil || got != "flatpak-pw" {
		t.Errorf("the host's delete changed the Flatpak's item: %q, %v", got, err)
	}
}

// A Flatpak item has exactly three attributes and the usual label; a host
// item has no store attribute.
func TestFlatpakItemAttributes(t *testing.T) {
	addr := privateBus(t)
	f := startFake(t, addr)
	ctx := t.Context()
	flat := openNamespace(t, addr, flatpakNS("0123abcd"), FlatpakOptions("0123abcd").Hint)
	if err := flat.Set(ctx, "account/1/password", "pw"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	for _, it := range f.items {
		want := map[string]string{"application": "org.frostyard.Frostmail", "store": "0123abcd", "key": "account/1/password"}
		if !maps.Equal(it.attrs, want) || it.label != "Frostmail: account/1/password" {
			t.Errorf("item attributes = %v, label %q; want %v", it.attrs, it.label, want)
		}
	}
	f.mu.Unlock()

	host := openNamespace(t, addr, Namespace{}, HostOptions().Hint)
	if err := host.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.items {
		if it.attrs["key"] == "k" && !maps.Equal(it.attrs, map[string]string{"application": "frostmail", "key": "k"}) {
			t.Errorf("host item attributes = %v", it.attrs)
		}
	}
}

func TestOptionsFor(t *testing.T) {
	host := OptionsFor(false, "ignored")
	if host.Namespace != (Namespace{}) || host.NoFallback || !strings.Contains(host.Hint, "FROSTMAIL_SECRETS=file") {
		t.Errorf("host options = %+v", host)
	}
	flat := OptionsFor(true, "0123")
	if flat.Namespace != flatpakNS("0123") || !flat.NoFallback || !strings.Contains(flat.Hint, flatpakOverride) {
		t.Errorf("Flatpak options = %+v", flat)
	}
}

func TestProbe(t *testing.T) {
	ss, f := openFake(t)
	ctx := t.Context()
	if err := Probe(ctx, ss); err != nil {
		t.Fatalf("Probe = %v", err)
	}
	if got := f.snapshot(); len(got) != 0 {
		t.Errorf("Probe left %v", got)
	}

	// A dismissed prompt is ErrDismissed.
	f.lockAll()
	f.mu.Lock()
	f.dismiss = true
	f.mu.Unlock()
	if err := Probe(ctx, ss); !errors.Is(err, ErrDismissed) {
		t.Errorf("Probe with a dismissed prompt = %v, want ErrDismissed", err)
	}

	// A store that keeps nothing is ErrNoKeyring.
	err := Probe(ctx, noKeyring{cause: ErrUnavailable, hint: "the hint"})
	if !errors.Is(err, ErrNoKeyring) || !strings.Contains(err.Error(), "the hint") {
		t.Errorf("Probe on the no-keyring store = %v", err)
	}
	// The Secret Service that left is ErrUnavailable.
	if _, err := f.conn.ReleaseName(ssName); err != nil {
		t.Fatal(err)
	}
	if err := Probe(ctx, ss); err == nil {
		t.Error("Probe with no service succeeded")
	}
}

// A store that fails to read back or to delete is a failed probe, and a
// probe item that was written is deleted again.
func TestProbeFailures(t *testing.T) {
	ctx := t.Context()
	m := &mapStore{m: map[string]string{}, corrupt: true}
	if err := Probe(ctx, m); err == nil || !strings.Contains(err.Error(), "another value") {
		t.Errorf("Probe on a store that reads back another value = %v", err)
	}
	if len(m.m) != 0 {
		t.Errorf("Probe left %v", m.m)
	}
	m = &mapStore{m: map[string]string{}, noDelete: true}
	if err := Probe(ctx, m); err == nil || !strings.Contains(err.Error(), "delete") {
		t.Errorf("Probe on a store that cannot delete = %v", err)
	}
}

type mapStore struct {
	m                 map[string]string
	corrupt, noDelete bool
}

func (s *mapStore) Get(_ context.Context, key string) (string, error) {
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	if s.corrupt {
		return v + "x", nil
	}
	return v, nil
}

func (s *mapStore) Set(_ context.Context, key, value string) error { s.m[key] = value; return nil }

func (s *mapStore) Delete(_ context.Context, key string) error {
	if s.noDelete {
		return errors.New("no delete")
	}
	delete(s.m, key)
	return nil
}

// The no-keyring store fails every call with ErrNoKeyring, never with
// ErrNotFound, and says what to do.
func TestNoKeyringStore(t *testing.T) {
	st := noKeyring{cause: errors.New("no bus"), hint: FlatpakOptions("x").Hint}
	ctx := t.Context()
	_, getErr := st.Get(ctx, "account/1/password")
	for name, err := range map[string]error{"get": getErr, "set": st.Set(ctx, "k", "v"), "delete": st.Delete(ctx, "k")} {
		if !errors.Is(err, ErrNoKeyring) || errors.Is(err, ErrNotFound) {
			t.Errorf("%s = %v; want ErrNoKeyring and not ErrNotFound", name, err)
		}
		if !strings.Contains(err.Error(), "no bus") || !strings.HasSuffix(err.Error(), "to keep secrets in a plain file instead") ||
			!strings.Contains(err.Error(), flatpakOverride) {
			t.Errorf("%s error = %v", name, err)
		}
	}
}

func TestOpenRefusesTheFlatpakNamespaceWithNoStoreID(t *testing.T) {
	t.Setenv("FROSTMAIL_SECRETS", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", privateBus(t))
	for _, mode := range []string{"", "file"} {
		t.Setenv("FROSTMAIL_SECRETS", mode)
		file := filepath.Join(t.TempDir(), "secrets.json")
		if st, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), FlatpakOptions("")); err == nil {
			t.Errorf("FROSTMAIL_SECRETS=%q: Open with no store id = %T, nil", mode, st)
		}
	}
}

// In the Flatpak with no Secret Service, Open keeps no plain file: every
// call fails with ErrNoKeyring, the leftover file keeps its bytes, and no
// file appears.
func TestOpenInTheFlatpakWithNoSecretServiceKeepsNoFile(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", privateBus(t)) // a bus with no service
	t.Setenv("FROSTMAIL_SECRETS", "")
	ctx := t.Context()
	opts := FlatpakOptions("0123abcd")

	// No file: Set makes none.
	dir := t.TempDir()
	file := filepath.Join(dir, "secrets.json")
	log, logged := logBuf()
	st, err := Open(ctx, file, log, opts)
	if err != nil {
		t.Fatal(err)
	}
	checkNoKeyring(t, st)
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a secrets file appeared: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the data directory holds %d entries", len(entries))
	}
	out := logged.String()
	if strings.Count(out, "level=ERROR") != 1 || !strings.Contains(out, "flatpak override") || strings.Contains(out, "level=WARN") {
		t.Errorf("the log = %s", out)
	}

	// A leftover file: untouched, with a warning.
	if err := NewFile(file).Set(ctx, AccountPassword(1), "pw-from-file"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	log, logged = logBuf()
	st, err = Open(ctx, file, log, opts)
	if err != nil {
		t.Fatal(err)
	}
	checkNoKeyring(t, st)
	if after, err := os.ReadFile(file); err != nil || !bytes.Equal(before, after) {
		t.Errorf("the leftover file changed: %v", err)
	}
	if !strings.Contains(logged.String(), "level=WARN") || !strings.Contains(logged.String(), file) || strings.Contains(logged.String(), "pw-from-file") {
		t.Errorf("the log = %s", logged)
	}
}

func checkNoKeyring(t *testing.T, st Store) {
	t.Helper()
	ctx := t.Context()
	_, getErr := st.Get(ctx, AccountPassword(1))
	for name, err := range map[string]error{"get": getErr, "set": st.Set(ctx, AccountPassword(1), "v"), "delete": st.Delete(ctx, AccountPassword(1))} {
		if !errors.Is(err, ErrNoKeyring) || errors.Is(err, ErrNotFound) {
			t.Errorf("%s = %v; want ErrNoKeyring", name, err)
			continue
		}
		if !strings.Contains(err.Error(), flatpakOverride) {
			t.Errorf("%s error does not name the override: %v", name, err)
		}
	}
}

// FROSTMAIL_SECRETS=file in the Flatpak is the plain file store.
func TestOpenInTheFlatpakWithFileMode(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", privateBus(t))
	t.Setenv("FROSTMAIL_SECRETS", "file")
	file := filepath.Join(t.TempDir(), "secrets.json")
	st, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), FlatpakOptions("0123abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*File); !ok {
		t.Fatalf("store = %T, want *File", st)
	}
	if err := st.Set(t.Context(), "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := NewFile(file).Get(t.Context(), "k"); err != nil || got != "v" {
		t.Errorf("the file holds %q, %v", got, err)
	}
}

// With a Secret Service, a leftover secrets.json (what the Flatpak's maild
// kept in Phases 2 and 3) moves into the Flatpak's namespace, the file is
// removed, one Info line says so, and the host's namespace finds nothing.
func TestOpenInTheFlatpakMovesTheLeftoverFileIn(t *testing.T) {
	addr := privateBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	t.Setenv("FROSTMAIL_SECRETS", "")
	f := startFake(t, addr)
	ctx := t.Context()
	file := filepath.Join(t.TempDir(), "secrets.json")
	old := NewFile(file)
	want := map[string]string{AccountPassword(1): "pw1", RefreshToken(2): "rt2"}
	for k, v := range want {
		if err := old.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	log, logged := logBuf()
	st, err := Open(ctx, file, log, FlatpakOptions("0123abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*SecretService); !ok {
		t.Fatalf("store = %T, want *SecretService", st)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the file is still there: %v", err)
	}
	for k, v := range want {
		if got, err := st.Get(ctx, k); err != nil || got != v {
			t.Errorf("Get(%s) = %q, %v; want %q", k, got, err, v)
		}
	}
	for _, attrs := range f.itemAttrs() {
		if attrs["application"] != config.AppID || attrs["store"] != "0123abcd" {
			t.Errorf("a moved item has attributes %v", attrs)
		}
	}
	host := openNamespace(t, addr, Namespace{}, HostOptions().Hint)
	for k := range want {
		if _, err := host.Get(ctx, k); !errors.Is(err, ErrNotFound) {
			t.Errorf("the host namespace finds %s: %v", k, err)
		}
	}
	out := logged.String()
	if strings.Count(out, "level=INFO") != 1 || !strings.Contains(out, "keys=2") || !strings.Contains(out, file) ||
		!strings.Contains(out, "store=0123abcd") || strings.Contains(out, "pw1") || strings.Contains(out, "rt2") {
		t.Errorf("the log = %s", out)
	}
}

// A service that cannot make the default collection, with the Flatpak's
// options: the error wraps ErrNoKeyring and names the override command.
func TestFlatpakCreateDefaultRefused(t *testing.T) {
	addr := privateBus(t)
	f := startFake(t, addr)
	f.mu.Lock()
	f.alias = noPrompt
	f.refuseCreate = true
	f.mu.Unlock()
	opts := FlatpakOptions("0123abcd")
	ss := openNamespace(t, addr, opts.Namespace, opts.Hint)
	err := ss.Set(t.Context(), "account/1/password", "v")
	if !errors.Is(err, ErrNoKeyring) || !strings.Contains(err.Error(), flatpakOverride) {
		t.Fatalf("Set = %v", err)
	}
	if !strings.HasSuffix(err.Error(), opts.Hint) {
		t.Errorf("the message does not end with the hint: %v", err)
	}
}

// OpenReader with the Flatpak's options reads the namespaced items, then
// the leftover file; with no Secret Service, the file alone.
func TestOpenReaderInTheFlatpak(t *testing.T) {
	addr := privateBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	t.Setenv("FROSTMAIL_SECRETS", "")
	ctx := t.Context()
	file := filepath.Join(t.TempDir(), "secrets.json")
	if err := NewFile(file).Set(ctx, AccountPassword(2), "from-file"); err != nil {
		t.Fatal(err)
	}
	opts := FlatpakOptions("0123abcd")
	st, err := OpenReader(ctx, file, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get(ctx, AccountPassword(2)); err != nil || got != "from-file" {
		t.Fatalf("no service: Get = %q, %v", got, err)
	}

	startFake(t, addr)
	flat := openNamespace(t, addr, opts.Namespace, opts.Hint)
	host := openNamespace(t, addr, Namespace{}, HostOptions().Hint)
	if err := flat.Set(ctx, AccountPassword(1), "flatpak-pw"); err != nil {
		t.Fatal(err)
	}
	if err := host.Set(ctx, AccountPassword(3), "host-pw"); err != nil {
		t.Fatal(err)
	}
	st, err = OpenReader(ctx, file, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get(ctx, AccountPassword(1)); err != nil || got != "flatpak-pw" {
		t.Errorf("a namespaced item = %q, %v", got, err)
	}
	if got, err := st.Get(ctx, AccountPassword(2)); err != nil || got != "from-file" {
		t.Errorf("the leftover file = %q, %v", got, err)
	}
	if _, err := st.Get(ctx, AccountPassword(3)); !errors.Is(err, ErrNotFound) {
		t.Errorf("the reader sees the host's item: %v", err)
	}
	// No store id: the file alone.
	st, err = OpenReader(ctx, file, FlatpakOptions(""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, AccountPassword(1)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reader with no store id reads the service: %v", err)
	}
	if got, err := st.Get(ctx, AccountPassword(2)); err != nil || got != "from-file" {
		t.Errorf("a reader with no store id: Get = %q, %v", got, err)
	}
}
