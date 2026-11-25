// Command nsgoinstaller writes Frostmail's additions into the NSIS script
// that nsgo build writes for the NSGo client's Windows package
// (docs/design/desktop.md, Windows):
//
//	nsgoinstaller -script FrostMail.nsi [-define NAME=VALUE]... INCLUDE...
//
// NSGo's script has no place for an app's own steps. This command puts the
// INCLUDE files into it, in their order, ahead of .onInit, with a !define
// for each -define ahead of them, and their 3 macros where they run:
// FM_INIT in .onInit, FM_INSTALL at the end of the install section, and
// FM_UNINSTALL in the uninstall section, among the steps that undo the
// registered install. The script is edited in place: the installer takes
// every other file of its folder, so nothing is written beside it.
//
// Each place is found by a line of NSGo's script, which must be there
// exactly once. A script that another version of NSGo writes otherwise is
// refused, so no installer is compiled without the additions.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The macros of packaging/windows/nsgo-installer.nsh.
const (
	macroInit      = "FM_INIT"
	macroInstall   = "FM_INSTALL"
	macroUninstall = "FM_UNINSTALL"
)

// The lines of NSGo's script (its cmd/nsgo/nsis.go) that the places are
// found by.
const (
	lineInit        = "Function .onInit"
	lineFunctionEnd = "FunctionEnd"
	lineInstall     = "Section"
	lineUninstall   = `Section "Uninstall"`
	lineSectionEnd  = "SectionEnd"
	// The last of the uninstaller's steps that run only for the registered
	// install: one left in a folder that an upgrade moved from skips them.
	lineRegistered = "  Call un.RemoveURLSchemes"
)

// define is one -define: an NSIS !define written ahead of the includes.
type define struct{ name, value string }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nsgoinstaller:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("nsgoinstaller", flag.ContinueOnError)
	script := fs.String("script", "", "the NSIS script nsgo build wrote, edited in place")
	var defines []define
	fs.Func("define", "an NSIS `NAME=VALUE` to define ahead of the includes; repeat for more", func(v string) error {
		d, err := parseDefine(v)
		if err != nil {
			return err
		}
		defines = append(defines, d)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *script == "" || fs.NArg() == 0 {
		return errors.New("nsgoinstaller takes -script FILE and 1 include file or more")
	}
	var includes []string
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		includes = append(includes, string(data))
	}
	data, err := os.ReadFile(*script)
	if err != nil {
		return err
	}
	out, err := patch(string(data), defines, includes)
	if err != nil {
		return fmt.Errorf("%s: %w", *script, err)
	}
	return os.WriteFile(*script, []byte(out), 0o644)
}

var defineName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// parseDefine reads NAME=VALUE. The value is written between double quotes
// as it is, so it holds none, nor NSIS's other quote, nor a "$", which
// starts a variable there.
func parseDefine(s string) (define, error) {
	name, value, ok := strings.Cut(s, "=")
	if !ok || !defineName.MatchString(name) {
		return define{}, fmt.Errorf("-define %q is not NAME=VALUE", s)
	}
	if strings.ContainsAny(value, "\"`$\r\n") {
		return define{}, fmt.Errorf("-define %s: the value %q holds a quote, a $ or a line break", name, value)
	}
	return define{name, value}, nil
}

// patch returns script, which nsgo build wrote, with the defines and the
// includes ahead of .onInit and the 3 macros at their places.
func patch(script string, defines []define, includes []string) (string, error) {
	lines := strings.Split(script, "\n")
	for _, m := range []string{macroInit, macroInstall, macroUninstall} {
		if !defined(includes, m) {
			return "", fmt.Errorf("no include defines the macro %s", m)
		}
		if at(lines, 0, len(lines), "  !insertmacro "+m) != nil {
			return "", errors.New("the script has Frostmail's additions already")
		}
	}
	init, err := only(lines, 0, len(lines), lineInit)
	if err != nil {
		return "", err
	}
	initEnd, err := first(lines, init, lineFunctionEnd)
	if err != nil {
		return "", err
	}
	install, err := only(lines, 0, len(lines), lineInstall)
	if err != nil {
		return "", err
	}
	installEnd, err := first(lines, install, lineSectionEnd)
	if err != nil {
		return "", err
	}
	uninstall, err := only(lines, 0, len(lines), lineUninstall)
	if err != nil {
		return "", err
	}
	uninstallEnd, err := first(lines, uninstall, lineSectionEnd)
	if err != nil {
		return "", err
	}
	registered, err := only(lines, uninstall, uninstallEnd, lineRegistered)
	if err != nil {
		return "", fmt.Errorf("in the uninstall section: %w", err)
	}
	if initEnd >= install || installEnd >= uninstall {
		return "", errors.New("NSGo's script has .onInit, the install section and the uninstall section in another order")
	}

	head := []string{"; Frostmail's additions (tools/nsgoinstaller)."}
	for _, d := range defines {
		head = append(head, fmt.Sprintf(`!define %s "%s"`, d.name, d.value))
	}
	for _, inc := range includes {
		inc = strings.TrimRight(strings.ReplaceAll(inc, "\r\n", "\n"), "\n")
		head = append(head, strings.Split(inc, "\n")...)
	}
	head = append(head, "")

	var out []string
	for i, line := range lines {
		switch i {
		case init:
			out = append(out, head...)
		case initEnd:
			out = append(out, "  !insertmacro "+macroInit)
		case installEnd:
			out = append(out, "  !insertmacro "+macroInstall)
		}
		out = append(out, line)
		if i == registered {
			out = append(out, "  !insertmacro "+macroUninstall)
		}
	}
	return strings.Join(out, "\n"), nil
}

// defined reports whether one of the includes defines the macro name.
func defined(includes []string, name string) bool {
	for _, inc := range includes {
		for line := range strings.Lines(inc) {
			if strings.TrimSpace(line) == "!macro "+name {
				return true
			}
		}
	}
	return false
}

// at returns the indexes in [from, to) of the lines that are text. A line
// may end in a carriage return.
func at(lines []string, from, to int, text string) []int {
	var found []int
	for i := from; i < to; i++ {
		if strings.TrimSuffix(lines[i], "\r") == text {
			found = append(found, i)
		}
	}
	return found
}

// only returns the index of the one line in [from, to) that is text.
func only(lines []string, from, to int, text string) (int, error) {
	found := at(lines, from, to, text)
	if len(found) != 1 {
		return 0, fmt.Errorf("NSGo's script has the line %q %d times, not once", text, len(found))
	}
	return found[0], nil
}

// first returns the index of the first line after from that is text.
func first(lines []string, from int, text string) (int, error) {
	found := at(lines, from+1, len(lines), text)
	if len(found) == 0 {
		return 0, fmt.Errorf("NSGo's script has no line %q after line %d", text, from+1)
	}
	return found[0], nil
}
