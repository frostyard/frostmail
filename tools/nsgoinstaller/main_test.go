package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testdata/FrostMail.nsi is the script that nsgo v0.3.0's build wrote for
// the client (go tool nsgo build -target windows_x64 in cmd/frostmail-nsgo),
// before any addition. Take it again after a change of NSGo's script.

const (
	includeFile = "installer.nsh"
	checkFile   = "installer-check.nsh"
)

func read(t *testing.T, path ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func packaging(t *testing.T, name string) string {
	t.Helper()
	return read(t, "..", "..", "packaging", "windows", name)
}

// index is the line of lines that is text, which must be there once.
func index(t *testing.T, lines []string, text string) int {
	t.Helper()
	i, err := only(lines, 0, len(lines), text)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// The additions go where they run, and the rest of NSGo's script stays as
// it was.
func TestAdditionsGoWhereTheyRun(t *testing.T) {
	script := read(t, "testdata", "FrostMail.nsi")
	include := packaging(t, includeFile)
	defines := []define{{"FM_NAME", "FrostMail"}, {"FM_EXE", "FrostMail.exe"}}
	out, err := patch(script, defines, []string{include})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "\xef\xbb\xbf") {
		t.Error("the script lost its byte order mark, without which makensis reads it in the system's code page")
	}
	lines := strings.Split(out, "\n")

	init := index(t, lines, lineInit)
	if got := lines[init+1 : init+3]; !slices.Equal(got, []string{"  !insertmacro FM_INIT", "FunctionEnd"}) {
		t.Errorf(".onInit holds %q, want FM_INIT alone", got)
	}
	head := index(t, lines, "; Frostmail's additions (tools/nsgoinstaller).")
	first := index(t, lines, `!define FM_NAME "FrostMail"`)
	second := index(t, lines, `!define FM_EXE "FrostMail.exe"`)
	macro := index(t, lines, "!macro FM_INIT")
	// MUI2.nsh and the languages come first: the include's message boxes
	// and LogicLib need them.
	language := index(t, lines, `!insertmacro MUI_LANGUAGE "English"`)
	if !slices.IsSorted([]int{language, head, first, second, macro, init}) {
		t.Errorf("the languages, the defines, the include and .onInit are at lines %d, %d, %d, %d, %d, %d, not in that order",
			language, head, first, second, macro, init)
	}

	install := index(t, lines, "  !insertmacro FM_INSTALL")
	if lines[install+1] != lineSectionEnd {
		t.Errorf("FM_INSTALL is followed by %q, not the end of its section", lines[install+1])
	}
	if section := index(t, lines, lineInstall); section > install || slices.Contains(lines[section:install], lineSectionEnd) {
		t.Error("FM_INSTALL is not in the install section")
	}

	uninstall := index(t, lines, "  !insertmacro FM_UNINSTALL")
	if lines[uninstall-1] != lineRegistered {
		t.Errorf("FM_UNINSTALL follows %q, want %q", lines[uninstall-1], lineRegistered)
	}
	// An uninstaller left in a folder that an upgrade moved from jumps to
	// files: it must not undo the registrations of the install that is.
	skip := index(t, lines, "  StrCmp $0 $INSTDIR 0 files")
	files := index(t, lines, "  files:")
	if !slices.IsSorted([]int{index(t, lines, lineUninstall), skip, uninstall, files}) {
		t.Errorf("FM_UNINSTALL at line %d is not between the jump at %d and files: at %d", uninstall, skip, files)
	}

	// With the additions taken out, the script is NSGo's.
	added := map[int]bool{init + 1: true, install: true, uninstall: true}
	var rest []string
	for i, line := range lines {
		if (i < head || i >= init) && !added[i] {
			rest = append(rest, line)
		}
	}
	if got := strings.Join(rest, "\n"); got != script {
		t.Error("the script differs from NSGo's in more than the additions")
	}
}

// A check's names come ahead of the include, which defines only the names
// that are not defined yet.
func TestCheckNamesComeFirst(t *testing.T) {
	check, include := packaging(t, checkFile), packaging(t, includeFile)
	out, err := patch(read(t, "testdata", "FrostMail.nsi"), nil, []string{check, include})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if c, i := index(t, lines, "!define FM_CHECK"), index(t, lines, "!macro FM_INIT"); c > i {
		t.Errorf("FM_CHECK is defined at line %d, after the include at %d", c, i)
	}
	names := 0
	for line := range strings.Lines(check) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "!define" || fields[1] == "FM_CHECK" {
			continue
		}
		names++
		if !strings.Contains(include, "\n!ifndef "+fields[1]+"\n") {
			t.Errorf("%s defines %s, which %s defines whatever it finds: makensis refuses a second !define", checkFile, fields[1], includeFile)
		}
	}
	if names == 0 {
		t.Errorf("%s defines no name", checkFile)
	}
	// The 2 steps with no name to change are the ones FM_CHECK leaves out.
	for _, step := range []string{`-autostart on`, `CurrentVersion\Run`} {
		i := strings.Index(include, step)
		if i < 0 {
			t.Fatalf("%s has no %q", includeFile, step)
		}
		before := include[:i]
		if strings.LastIndex(before, "!ifndef FM_CHECK") < strings.LastIndex(before, "!endif") {
			t.Errorf("%s: %q is not inside !ifndef FM_CHECK", includeFile, step)
		}
	}
}

// A script of another shape is refused: an installer compiled from it would
// lack an addition, or run one at the wrong place.
func TestAScriptOfAnotherShapeIsRefused(t *testing.T) {
	script := read(t, "testdata", "FrostMail.nsi")
	include := packaging(t, includeFile)
	for name, change := range map[string]func(string) string{
		"no .onInit":           func(s string) string { return strings.Replace(s, lineInit+"\n", "Function .onGUIInit\n", 1) },
		"two .onInit":          func(s string) string { return s + "\n" + lineInit + "\n" + lineFunctionEnd + "\n" },
		"a named section":      func(s string) string { return strings.Replace(s, "\n"+lineInstall+"\n", "\nSection \"Install\"\n", 1) },
		"no uninstall section": func(s string) string { return strings.Replace(s, lineUninstall, `Section "un.Install"`, 1) },
		"no registered step":   func(s string) string { return strings.Replace(s, lineRegistered+"\n", "", 1) },
		"additions already": func(s string) string {
			return strings.Replace(s, lineInit+"\n", lineInit+"\n  !insertmacro FM_INIT\n", 1)
		},
	} {
		changed := change(script)
		if changed == script {
			t.Fatalf("%s: the test changed nothing", name)
		}
		if out, err := patch(changed, nil, []string{include}); err == nil {
			t.Errorf("%s: patched, %d bytes", name, len(out))
		}
	}
	out, err := patch(script, nil, []string{include})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patch(out, nil, []string{include}); err == nil {
		t.Error("a second run patched the script again")
	}
	if _, err := patch(script, nil, []string{"!macro FM_INIT\n!macroend\n"}); err == nil {
		t.Error("patched with an include that lacks 2 of the macros")
	}
}

// The command edits the script in place, with Windows' line ends in an
// include made into the script's.
func TestRunEditsTheScriptInPlace(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "FrostMail.nsi")
	if err := os.WriteFile(script, []byte(read(t, "testdata", "FrostMail.nsi")), 0o644); err != nil {
		t.Fatal(err)
	}
	include := filepath.Join(dir, "include.nsh")
	crlf := strings.ReplaceAll(packaging(t, includeFile), "\n", "\r\n")
	if err := os.WriteFile(include, []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-script", script, "-define", "FM_NAME=FrostMail Check", include}); err != nil {
		t.Fatal(err)
	}
	out := read(t, script)
	if strings.Contains(out, "\r") {
		t.Error("the script has a carriage return")
	}
	if !strings.Contains(out, "\n!define FM_NAME \"FrostMail Check\"\n") {
		t.Error("the script has no FM_NAME")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The installer takes every file of the folder but the script.
	if len(entries) != 2 {
		t.Errorf("the folder holds %d files, want the script and the include", len(entries))
	}
	if err := run([]string{"-script", script, include}); err == nil {
		t.Error("a second run patched the script again")
	}
}

func TestDefines(t *testing.T) {
	for _, bad := range []string{"", "FM_NAME", "=x", "fm_name=x", "FM NAME=x", `FM_NAME=a"b`, "FM_NAME=a`b", "FM_NAME=$INSTDIR", "FM_NAME=a\nb"} {
		if d, err := parseDefine(bad); err == nil {
			t.Errorf("parseDefine(%q) = %v, want an error", bad, d)
		}
	}
	d, err := parseDefine("FM_APP_ID=org.frostyard.Frostmail.Check")
	if err != nil || d != (define{"FM_APP_ID", "org.frostyard.Frostmail.Check"}) {
		t.Errorf("parseDefine = %v, %v", d, err)
	}
	if d, err := parseDefine("FM_NAME="); err != nil || d != (define{"FM_NAME", ""}) {
		t.Errorf("an empty value: %v, %v", d, err)
	}
}
