// Package opener checks links and files from outside the app (a link in a
// message, an attachment) against allow-lists before the client hands them
// to the system. It was frostui's Opener (v0.10.1, MIT, the same owner),
// without its calls into MyGo: the client opens what passes with its own
// platform's calls (NSGo's OpenURL and OpenFile).
package opener

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// ErrNotAllowed is what an Opener's refusals wrap: the target is not on
// its allow-lists.
var ErrNotAllowed = errors.New("not allowed")

// DefaultOpenSchemes are the URL schemes an Opener with no Schemes allows.
func DefaultOpenSchemes() []string { return []string{"http", "https", "mailto"} }

// runnableExtensions is a deny list of file types that run, install or
// redirect when the system opens them, or mount and hide what they hold:
// an Opener never allows them. It is a backstop, and cannot be complete:
// Opener.Extensions is the allow-list for files from outside the app.
var runnableExtensions = []string{
	".app", ".appimage", ".appinstaller", ".application", ".appref-ms", ".appx", ".appxbundle",
	".bash", ".bat", ".chm", ".cmd", ".com", ".command", ".cpl", ".deb", ".desktop", ".dmg",
	".exe", ".fileloc", ".flatpak", ".flatpakref", ".gadget", ".hta", ".img", ".inetloc", ".inf",
	".iso", ".jar", ".js", ".jse", ".library-ms", ".lnk", ".mpkg", ".msc", ".msi", ".msix",
	".msixbundle", ".msp", ".mst", ".pif", ".pkg", ".pl", ".ps1", ".ps1xml", ".psc1", ".psd1",
	".psm1", ".py", ".pyw", ".pyz", ".rb", ".reg", ".rpm", ".run", ".scf", ".scr", ".sct",
	".search-ms", ".settingcontent-ms", ".sh", ".terminal", ".url", ".vb", ".vbe", ".vbs",
	".vhd", ".vhdx", ".webloc", ".workflow", ".ws", ".wsc", ".wsf", ".wsh", ".xbap", ".xll",
}

// Opener checks links and files behind allow-lists, for what comes from
// outside the app: a link in a message, an attachment. Its zero value
// allows http, https and mailto links, and no file. Anything else is
// refused with an error wrapping ErrNotAllowed. A mailto link is allowed as
// SanitizeMailto returns it. It holds no state, and is safe from any
// goroutine.
type Opener struct {
	// Schemes are the URL schemes CheckURL allows, in any case; nil is
	// DefaultOpenSchemes. A "file" entry allows nothing: files go through
	// CheckFile, under Roots. A mailto link always goes through
	// SanitizeMailto.
	Schemes []string
	// Roots are the directories CheckFile allows files under; none allows
	// no file. Symbolic links are followed before the check, so one
	// under a root that leads out of it is refused.
	Roots []string
	// Extensions, when set, are the only file extensions CheckFile allows,
	// as ".pdf", in any case. Set it for files from outside the app, as
	// attachments: without it CheckFile relies on a deny list of types that
	// run, install or redirect, a backstop that cannot be complete. Types
	// on the deny list are refused whether listed here or not.
	Extensions []string
	// Folders lets CheckFile allow directories under Roots.
	Folders bool
}

// refused is an error saying why target is not opened.
func refused(target, why string) error {
	return fmt.Errorf("refused to open %q: %s: %w", target, why, ErrNotAllowed)
}

// parseURL parses raw as a URL, refusing spaces and control characters.
func parseURL(raw string) (*url.URL, error) {
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return nil, refused(raw, "it holds spaces or control characters")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, refused(raw, "it is not a URL")
	}
	return u, nil
}

// mailtoFields are the header fields a mailto link keeps (ADR-0010).
var mailtoFields = []string{"to", "cc", "bcc", "subject", "body"}

// isMailtoField reports whether name is one of mailtoFields, in any ASCII
// case and not encoded. ASCII alone: Unicode folding would read "ſubject"
// as "subject".
func isMailtoField(name string) bool {
	return slices.ContainsFunc(mailtoFields, func(field string) bool {
		if len(name) != len(field) {
			return false
		}
		for i := range len(field) {
			// field[i] is a lowercase letter: only it and its capital
			// give it with the 0x20 bit set.
			if name[i]|0x20 != field[i] {
				return false
			}
		}
		return true
	})
}

// SanitizeMailto returns the mailto link raw with only its addressees,
// subject and body (ADR-0010). It keeps the address and every field named
// to, cc, bcc, subject or body, in any ASCII case, as written and in
// order. It removes, rather than refuses, every other field (attach=), a
// field whose name is encoded (%61ttach) or has no "=", the fragment, and
// a "?" with nothing after it. It decodes nothing: a kept value passes
// byte for byte, "+" and malformed escapes included. It refuses, with an
// error wrapping ErrNotAllowed, what is not a mailto link or holds spaces
// or control characters. Opener.CheckURL applies it to every mailto link,
// then refuses one that names nothing.
func SanitizeMailto(raw string) (string, error) {
	u, err := parseURL(raw)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(u.Scheme, "mailto") {
		return "", refused(raw, "it is not a mailto link")
	}
	sanitizeMailto(u)
	return u.String(), nil
}

// sanitizeMailto drops from the mailto link u the fields and the fragment
// that SanitizeMailto removes.
func sanitizeMailto(u *url.URL) {
	var kept []string
	for field := range strings.SplitSeq(u.RawQuery, "&") {
		if name, _, ok := strings.Cut(field, "="); ok && isMailtoField(name) {
			kept = append(kept, field)
		}
	}
	u.RawQuery = strings.Join(kept, "&")
	u.ForceQuery = false
	u.Fragment, u.RawFragment = "", ""
}

// CheckURL reports whether raw may be opened: the URL to open, or why not.
// A mailto link is returned as SanitizeMailto returns it.
func (o Opener) CheckURL(raw string) (string, error) {
	u, err := parseURL(raw)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return "", refused(raw, "it has no scheme")
	}
	if scheme == "file" {
		return "", refused(raw, "a file goes through CheckFile")
	}
	allowed := o.Schemes
	if allowed == nil {
		allowed = DefaultOpenSchemes()
	}
	if !slices.ContainsFunc(allowed, func(s string) bool { return strings.EqualFold(strings.TrimSuffix(s, ":"), scheme) }) {
		return "", refused(raw, "the scheme "+scheme+" is not allowed")
	}
	if scheme == "mailto" {
		sanitizeMailto(u)
	}
	switch {
	case (scheme == "http" || scheme == "https") && u.Hostname() == "":
		return "", refused(raw, "it has no host")
	case u.Opaque == "" && u.Host == "" && u.Path == "" && u.RawQuery == "":
		return "", refused(raw, "it names nothing")
	}
	return u.String(), nil
}

// CheckFile reports whether path, which must be absolute, may be opened:
// its real path, symbolic links followed, or why not.
func (o Opener) CheckFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", refused(path, "the path is not absolute")
	}
	if len(o.Roots) == 0 {
		return "", refused(path, "no folder is allowed")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", refused(path, "it cannot be read: "+err.Error())
	}
	inside := false
	for _, root := range o.Roots {
		r, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(r, resolved); err == nil && filepath.IsLocal(rel) {
			inside = true
			break
		}
	}
	if !inside {
		return "", refused(path, "it is outside the allowed folders")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", refused(path, "it cannot be read: "+err.Error())
	}
	name := filepath.Base(resolved)
	if runtime.GOOS == "windows" {
		// Windows drops trailing dots and spaces, and reads what follows
		// a colon as a stream of the file.
		if strings.Contains(name, ":") {
			return "", refused(path, "the name holds a colon")
		}
		name = strings.TrimRight(name, ". ")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if slices.Contains(runnableExtensions, ext) {
		return "", refused(path, "files of the type "+ext+" can run, install or redirect")
	}
	switch {
	case info.IsDir():
		if !o.Folders {
			return "", refused(path, "it is a folder")
		}
		return resolved, nil
	case !info.Mode().IsRegular():
		return "", refused(path, "it is not a regular file")
	case info.Mode().Perm()&0o111 != 0:
		return "", refused(path, "it is executable")
	}
	if o.Extensions != nil && !slices.ContainsFunc(o.Extensions, func(e string) bool { return strings.EqualFold(e, ext) }) {
		return "", refused(path, "the type "+ext+" is not allowed")
	}
	return resolved, nil
}
