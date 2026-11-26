package opener

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenerAllowsTheDefaultURLs(t *testing.T) {
	for _, raw := range []string{"http://example.org/a", "https://example.org/a?b=c#d", "HTTPS://example.org", "mailto:a@example.org", "mailto:?subject=hi"} {
		if got, err := (Opener{}).CheckURL(raw); err != nil || got == "" {
			t.Errorf("CheckURL(%q) = %q, %v", raw, got, err)
		}
	}
}

func TestOpenerRefusesOtherURLs(t *testing.T) {
	o := Opener{}
	for _, raw := range []string{
		"", "example.org", "/etc/passwd", "file:///etc/passwd", "FILE:///etc/passwd", "javascript:alert(1)",
		"ftp://example.org", "smb://host/share", "data:text/html,x", "https:", "https://", "https:example.org",
		"https:\\\\example.org", "mailto:", "http://exa mple.org", "http://example.org/\nmore", "http://example.org/\x7f",
	} {
		if got, err := o.CheckURL(raw); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("CheckURL(%q) = %q, %v, want a refusal", raw, got, err)
		}
	}
}

func TestOpenerSchemesReplaceTheDefaultsButNeverAddFiles(t *testing.T) {
	o := Opener{Schemes: []string{"obsidian:", "file"}}
	if _, err := o.CheckURL("obsidian://open?vault=a"); err != nil {
		t.Errorf("a listed scheme: %v", err)
	}
	if _, err := o.CheckURL("https://example.org"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a default scheme not listed: %v", err)
	}
	if _, err := o.CheckURL("file:///etc/passwd"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("file listed as a scheme: %v", err)
	}
}

// mailtoCases are mailto links and what SanitizeMailto makes of them
// (ADR-0010). An opener opens the same, except a link that names nothing,
// which it refuses.
var mailtoCases = []struct {
	name, in, want string
	namesNothing   bool
}{
	// The cases of an app that filtered mailto links itself before the
	// kit had the rule: the rule gives the same, so the app can drop its
	// copy.
	{"attach between kept fields", "mailto:a@example.com?subject=hi&attach=/etc/passwd&body=x", "mailto:a@example.com?subject=hi&body=x", false},
	{"attach, attachment and case", "mailto:a@e.test?attach=/etc/passwd&subject=Hi%20there&Attachment=x&cc=b@e.test&ATTACH=y&body=a+b", "mailto:a@e.test?subject=Hi%20there&cc=b@e.test&body=a+b", false},
	{"attach alone", "mailto:a@e.test?attach=/home/u/.ssh/id_ed25519", "mailto:a@e.test", false},
	{"encoded attach, kept To", "mailto:a@e.test?%61ttach=x&To=c@e.test", "mailto:a@e.test?To=c@e.test", false},
	{"no fields", "mailto:a@e.test", "mailto:a@e.test", false},

	{"each kept field", "mailto:a@e.test?to=b@e.test&cc=c@e.test&bcc=d@e.test&subject=s&body=b", "mailto:a@e.test?to=b@e.test&cc=c@e.test&bcc=d@e.test&subject=s&body=b", false},
	{"kept fields in any case", "mailto:a@e.test?TO=b@e.test&Cc=c@e.test&bCC=d@e.test&SuBjEcT=s&BODY=b", "mailto:a@e.test?TO=b@e.test&Cc=c@e.test&bCC=d@e.test&SuBjEcT=s&BODY=b", false},
	{"file and other headers", "mailto:a@e.test?file=/etc/passwd&in-reply-to=%3Cx@e.test%3E&references=r&from=f@e.test&reply-to=r@e.test&X-Mailer=m&sender=s@e.test", "mailto:a@e.test", false},
	{"the scheme in capitals", "MAILTO:a@e.test?Subject=x&FILE=y", "mailto:a@e.test?Subject=x", false},
	{"repeated fields, in order", "mailto:a@e.test?to=b@e.test&attach=x&to=c@e.test&subject=1&subject=2&body=1&body=2", "mailto:a@e.test?to=b@e.test&to=c@e.test&subject=1&subject=2&body=1&body=2", false},
	{"repeated attach", "mailto:a@e.test?attach=x&attach=y&ATTACH=z", "mailto:a@e.test", false},
	{"plus and %20 as written", "mailto:a@e.test?subject=a+b&body=c%20d+e", "mailto:a@e.test?subject=a+b&body=c%20d+e", false},
	{"encoded kept names", "mailto:a@e.test?%74o=b@e.test&%53ubject=x&body=y", "mailto:a@e.test?body=y", false},
	{"double encoded attach", "mailto:a@e.test?%2561ttach=x&subject=y", "mailto:a@e.test?subject=y", false},
	{"Unicode folding", "mailto:a@e.test?ſubject=x&body=y", "mailto:a@e.test?body=y", false},
	{"fields without =", "mailto:a@e.test?subject&body=x&attach&to", "mailto:a@e.test?body=x", false},
	{"empty fields and values", "mailto:a@e.test?&&subject=&&body=x&", "mailto:a@e.test?subject=&body=x", false},
	{"an address and to=", "mailto:a@e.test?to=b@e.test&attach=x", "mailto:a@e.test?to=b@e.test", false},
	{"to= without an address", "mailto:?to=b@e.test&attach=x", "mailto:?to=b@e.test", false},
	{"a fragment carrying a field", "mailto:a@e.test?subject=x#&attach=/etc/passwd", "mailto:a@e.test?subject=x", false},
	{"a fragment without fields", "mailto:a@e.test#attach=x", "mailto:a@e.test", false},
	{"malformed escapes", "mailto:a@e.test?subject=100%&body=%zz&%zzattach=x&%=y", "mailto:a@e.test?subject=100%&body=%zz", false},
	{"encoded separators in a value", "mailto:a@e.test?body=x%26attach%3D/etc/passwd", "mailto:a@e.test?body=x%26attach%3D/etc/passwd", false},
	{"a ? and nothing after it", "mailto:a@e.test?", "mailto:a@e.test", false},
	{"no address, no field", "mailto:", "mailto:", true},
	{"no address, only attach", "mailto:?attach=/etc/passwd", "mailto:", true},
	{"no address, only a fragment", "mailto:?#&attach=x", "mailto:", true},
}

func TestSanitizeMailtoKeepsOnlyAddresseesSubjectAndBody(t *testing.T) {
	for _, tc := range mailtoCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SanitizeMailto(tc.in)
			if err != nil || got != tc.want {
				t.Fatalf("SanitizeMailto(%q) = %q, %v, want %q", tc.in, got, err, tc.want)
			}
			if again, err := SanitizeMailto(got); err != nil || again != got {
				t.Errorf("SanitizeMailto(%q) = %q, %v: not idempotent", got, again, err)
			}
		})
	}
}

func TestOpenerAllowsMailtoLinksSanitized(t *testing.T) {
	for _, tc := range mailtoCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Opener{}).CheckURL(tc.in)
			if tc.namesNothing {
				if !errors.Is(err, ErrNotAllowed) {
					t.Errorf("CheckURL(%q) = %q, %v, want a refusal", tc.in, got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("CheckURL(%q) = %q, %v, want %q", tc.in, got, err, tc.want)
			}
		})
	}
}

func TestOpenerSanitizesMailtoWhateverTheSchemes(t *testing.T) {
	o := Opener{Schemes: []string{"MAILTO:"}}
	if got, err := o.CheckURL("mailto:a@e.test?attach=/etc/passwd&subject=x"); err != nil || got != "mailto:a@e.test?subject=x" {
		t.Errorf("mailto listed by the app: %q, %v", got, err)
	}
}

func TestSanitizeMailtoRefusesOtherLinks(t *testing.T) {
	for _, raw := range []string{
		"", "a@e.test", "https://example.org/?attach=x", "file:///etc/passwd", "javascript:alert(1)",
		"mailto:a@e.test?subject=a b", "mailto:a@e.test?subject=x\nattach=y", "mailto:a@e.test\x7f",
		"mailto:a@e.test#%zz", "mailto:a@e.test?\tattach=x",
	} {
		if got, err := SanitizeMailto(raw); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("SanitizeMailto(%q) = %q, %v, want a refusal", raw, got, err)
		}
	}
}

// vault makes a directory holding a note, and returns its path.
func vault(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "note.md"), 0o644)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func write(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestOpenerAllowsFilesUnderARoot(t *testing.T) {
	dir := vault(t)
	write(t, filepath.Join(dir, "sub", "photo.PNG"), 0o644)
	var allowed []string
	o := Opener{Roots: []string{dir}}
	for _, p := range []string{filepath.Join(dir, "note.md"), filepath.Join(dir, "sub", "photo.PNG"), filepath.Join(dir, "sub", "..", "note.md")} {
		got, err := o.CheckFile(p)
		if err != nil {
			t.Errorf("CheckFile(%q) = %v", p, err)
		}
		allowed = append(allowed, got)
	}
	if len(allowed) != 3 || allowed[2] != filepath.Join(dir, "note.md") {
		t.Errorf("allowed %q", allowed)
	}
}

func TestOpenerRefusesOtherFiles(t *testing.T) {
	dir, outside := vault(t), vault(t)
	write(t, filepath.Join(dir, "run.bin"), 0o755)
	write(t, filepath.Join(dir, "launcher.desktop"), 0o644)
	write(t, filepath.Join(dir, "setup.EXE"), 0o644)
	if err := os.Symlink(filepath.Join(outside, "note.md"), filepath.Join(dir, "away.md")); err != nil {
		t.Fatal(err)
	}
	o := Opener{Roots: []string{dir}}
	for _, p := range []string{
		filepath.Join(outside, "note.md"),
		filepath.Join(dir, "..", filepath.Base(outside), "note.md"),
		filepath.Join(dir, "away.md"),
		filepath.Join(dir, "missing.md"),
		filepath.Join(dir, "run.bin"),
		filepath.Join(dir, "launcher.desktop"),
		filepath.Join(dir, "setup.EXE"),
		filepath.Join(dir, "sub"),
		"note.md",
		"",
	} {
		if got, err := o.CheckFile(p); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("CheckFile(%q) = %q, %v, want a refusal", p, got, err)
		}
	}
	if _, err := (Opener{}).CheckFile(filepath.Join(dir, "note.md")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("no roots allowed a file: %v", err)
	}
}

func TestOpenerRootsFollowSymlinksAndDoNotMatchPrefixes(t *testing.T) {
	base := vault(t)
	target := filepath.Join(base, "real")
	sibling := filepath.Join(base, "real-other")
	for _, d := range []string{target, sibling} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(d, "a.md"), 0o644)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	o := Opener{Roots: []string{link}}
	if _, err := o.CheckFile(filepath.Join(target, "a.md")); err != nil {
		t.Errorf("a file under a root given through a link: %v", err)
	}
	o.Roots = []string{target}
	if _, err := o.CheckFile(filepath.Join(sibling, "a.md")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a directory sharing the root's prefix: %v", err)
	}
}

func TestOpenerExtensionsAndFolders(t *testing.T) {
	dir := vault(t)
	write(t, filepath.Join(dir, "a.PDF"), 0o644)
	o := Opener{Roots: []string{dir}, Extensions: []string{".pdf"}}
	if _, err := o.CheckFile(filepath.Join(dir, "a.PDF")); err != nil {
		t.Errorf("a listed extension: %v", err)
	}
	if _, err := o.CheckFile(filepath.Join(dir, "note.md")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("an unlisted extension: %v", err)
	}
	o.Extensions = []string{".sh"}
	write(t, filepath.Join(dir, "a.sh"), 0o644)
	if _, err := o.CheckFile(filepath.Join(dir, "a.sh")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a runnable type listed: %v", err)
	}
	o = Opener{Roots: []string{dir}, Folders: true}
	if _, err := o.CheckFile(filepath.Join(dir, "sub")); err != nil {
		t.Errorf("a folder with Folders: %v", err)
	}
}

// deniedTypes is the deny list, written out so that taking a type off
// runnableExtensions fails a test.
var deniedTypes = []string{
	".app", ".appimage", ".appinstaller", ".application", ".appref-ms", ".appx", ".appxbundle",
	".bash", ".bat", ".chm", ".cmd", ".com", ".command", ".cpl", ".deb", ".desktop", ".dmg",
	".exe", ".fileloc", ".flatpak", ".flatpakref", ".gadget", ".hta", ".img", ".inetloc", ".inf",
	".iso", ".jar", ".js", ".jse", ".library-ms", ".lnk", ".mpkg", ".msc", ".msi", ".msix",
	".msixbundle", ".msp", ".mst", ".pif", ".pkg", ".pl", ".ps1", ".ps1xml", ".psc1", ".psd1",
	".psm1", ".py", ".pyw", ".pyz", ".rb", ".reg", ".rpm", ".run", ".scf", ".scr", ".sct",
	".search-ms", ".settingcontent-ms", ".sh", ".terminal", ".url", ".vb", ".vbe", ".vbs",
	".vhd", ".vhdx", ".webloc", ".workflow", ".ws", ".wsc", ".wsf", ".wsh", ".xbap", ".xll",
}

func TestOpenerRefusesEveryDeniedType(t *testing.T) {
	dir := vault(t)
	for _, ext := range deniedTypes {
		t.Run(ext, func(t *testing.T) {
			// Listing a type does not let it through, in either case.
			o := Opener{Roots: []string{dir}, Extensions: []string{ext}}
			for _, name := range []string{"a" + ext, "b" + strings.ToUpper(ext)} {
				p := filepath.Join(dir, name)
				write(t, p, 0o644)
				if got, err := o.CheckFile(p); !errors.Is(err, ErrNotAllowed) {
					t.Errorf("CheckFile(%q) = %q, %v, want a refusal", p, got, err)
				}
			}
		})
	}
	if len(runnableExtensions) != len(deniedTypes) {
		t.Errorf("the deny list has %d types, the test %d", len(runnableExtensions), len(deniedTypes))
	}
}
