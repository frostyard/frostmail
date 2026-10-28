package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/frostyard/frostmail/internal/config"
)

// defaultFlatpakInfo is the file a Flatpak sandbox describes itself in;
// FROSTMAIL_FLATPAK_INFO replaces it, for the smoke test.
const defaultFlatpakInfo = "/.flatpak-info"

// printMode is -mode: which maild the desktop uses, from app.json, the
// same rule the mode check applies (docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md).
func printMode(w io.Writer) error {
	mode, err := config.AppMode(os.Getenv)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, mode)
	return err
}

// printBuild is -build: the Flatpak build of this instance.
func printBuild(w io.Writer) error {
	build, err := flatpakBuild(os.Getenv)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, build)
	return err
}

// flatpakBuild is the app-commit of the [Instance] group of the sandbox's
// .flatpak-info: it differs for every build of the Flatpak, even of one
// version. The launcher compares it with the latest installed build's
// (maild -build run through flatpak-spawn --latest-version) to replace an
// older maild.
func flatpakBuild(getenv func(string) string) (string, error) {
	path := getenv("FROSTMAIL_FLATPAK_INFO")
	if path == "" {
		path = defaultFlatpakInfo
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	var inInstance bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inInstance = line == "[Instance]"
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && inInstance && strings.TrimSpace(key) == "app-commit" {
			if value = strings.TrimSpace(value); value != "" {
				return value, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return "", fmt.Errorf("%s has no app-commit in [Instance]", path)
}

// serverBuild is what rpc.hello reports as build: the Flatpak build, only
// in this application's Flatpak and only when it is known.
func serverBuild(logger *slog.Logger) string {
	if !config.InFlatpak(os.Getenv) {
		return ""
	}
	build, err := flatpakBuild(os.Getenv)
	if err != nil {
		logger.Warn("the Flatpak build is unknown", "err", err)
		return ""
	}
	return build
}

// lodgeMode reports whether this maild must not run. Only in this
// application's Flatpak, and only now, under the data lock and before the
// database opens: an app.json that names lodge means lodge's maild is the
// engine, and a second one here would run every rule, send every scheduled
// message and raise every reminder twice (ADR-0046). A launcher that read
// "local" and was delayed past a change of mode lands here. maild reads
// app.json and never writes it; one that is not valid JSON is an error.
func lodgeMode(logger *slog.Logger) (bool, error) {
	if !config.InFlatpak(os.Getenv) {
		return false, nil
	}
	mode, err := config.AppMode(os.Getenv)
	if err != nil {
		return false, err
	}
	if mode != config.ModeLodge {
		return false, nil
	}
	logger.Info("app.json names lodge; this maild does not run")
	return true, nil
}
