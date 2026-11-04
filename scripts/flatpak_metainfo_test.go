//go:build linux

package scripts_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Software centres show the default screenshot first. appstreamcli validate
// fails on a missing default but only gives an info for an http URL, so this
// test checks both, and that each URL names a PNG in this repository on main,
// so a renamed or deleted image fails here before a software centre shows a
// broken link.

// pngSignature is the 8 bytes every PNG file starts with.
const pngSignature = "\x89PNG\r\n\x1a\n"

type metainfo struct {
	Screenshots struct {
		Screenshots []screenshot `xml:"screenshot"`
	} `xml:"screenshots"`
}

type screenshot struct {
	Type    string   `xml:"type,attr"`
	Caption string   `xml:"caption"`
	Images  []string `xml:"image"`
}

func checkScreenshots(meta []byte, root string) []error {
	var parsed metainfo
	if err := xml.Unmarshal(meta, &parsed); err != nil {
		return []error{fmt.Errorf("parse metainfo XML: %w", err)}
	}

	shots := parsed.Screenshots.Screenshots
	var errs []error
	if len(shots) == 0 {
		return []error{errors.New("metainfo has no screenshots")}
	}

	if shots[0].Type != "default" {
		errs = append(errs, errors.New("first screenshot is not type=\"default\""))
	}
	for i := 1; i < len(shots); i++ {
		if shots[i].Type == "default" {
			errs = append(errs, fmt.Errorf("screenshot %d also has type=\"default\"", i+1))
		}
	}

	const prefix = "/frostyard/frostmail/main/"
	for i, shot := range shots {
		if strings.TrimSpace(shot.Caption) == "" {
			errs = append(errs, fmt.Errorf("screenshot %d has an empty caption", i+1))
		}
		if len(shot.Images) == 0 {
			errs = append(errs, fmt.Errorf("screenshot %d has no image", i+1))
			continue
		}
		for j, rawURL := range shot.Images {
			parsedURL, err := url.Parse(rawURL)
			if err != nil {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL is invalid: %w", i+1, j+1, err))
				continue
			}
			if parsedURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL does not use https", i+1, j+1))
			}
			if parsedURL.Host != "raw.githubusercontent.com" {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL has the wrong host", i+1, j+1))
			}
			if !strings.HasPrefix(parsedURL.Path, prefix) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d URL path is outside /frostyard/frostmail/main/", i+1, j+1))
				continue
			}

			rest := strings.TrimPrefix(parsedURL.Path, prefix)
			if rest == "" {
				errs = append(errs, fmt.Errorf("screenshot %d image %d names no file under root", i+1, j+1))
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(rest))
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d names a file outside root", i+1, j+1))
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				errs = append(errs, fmt.Errorf("screenshot %d image %d file does not exist: %s", i+1, j+1, rest))
				continue
			}
			if !bytes.HasPrefix(data, []byte(pngSignature)) {
				errs = append(errs, fmt.Errorf("screenshot %d image %d file is not a PNG: %s", i+1, j+1, rest))
			}
		}
	}
	return errs
}

func TestFlatpakMetainfoScreenshots(t *testing.T) {
	meta := readFile(t, "..", "packaging/flatpak/org.frostyard.Frostmail.metainfo.xml")
	for _, err := range checkScreenshots([]byte(meta), "..") {
		t.Error(err)
	}
}

func TestFlatpakMetainfoScreenshotsRejects(t *testing.T) {
	validImage := "https://raw.githubusercontent.com/frostyard/frostmail/main/docs/images/main.png"
	for _, tc := range []struct {
		name    string
		meta    func(*testing.T, string) string
		problem string
	}{
		{name: "no screenshots", meta: func(*testing.T, string) string { return `<component><screenshots/></component>` }, problem: "no screenshots"},
		{name: "first is not default", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot><caption>mail</caption><image>` + validImage + `</image></screenshot></screenshots></component>`
		}, problem: "first screenshot"},
		{name: "2 defaults", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>one</caption><image>` + validImage + `</image></screenshot><screenshot type="default"><caption>two</caption><image>` + validImage + `</image></screenshot></screenshots></component>`
		}, problem: "also has type=\"default\""},
		{name: "empty caption", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption> </caption><image>` + validImage + `</image></screenshot></screenshots></component>`
		}, problem: "empty caption"},
		{name: "no image", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>mail</caption></screenshot></screenshots></component>`
		}, problem: "no image"},
		{name: "http URL", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>mail</caption><image>http://raw.githubusercontent.com/frostyard/frostmail/main/docs/images/main.png</image></screenshot></screenshots></component>`
		}, problem: "https"},
		{name: "another host", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>mail</caption><image>https://example.com/frostyard/frostmail/main/docs/images/main.png</image></screenshot></screenshots></component>`
		}, problem: "wrong host"},
		{name: "path outside main", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>mail</caption><image>https://raw.githubusercontent.com/frostyard/frostmail/dev/docs/images/main.png</image></screenshot></screenshots></component>`
		}, problem: "outside /frostyard/frostmail/main/"},
		{name: "file does not exist", meta: func(*testing.T, string) string {
			return `<component><screenshots><screenshot type="default"><caption>mail</caption><image>https://raw.githubusercontent.com/frostyard/frostmail/main/docs/images/missing.png</image></screenshot></screenshots></component>`
		}, problem: "does not exist"},
		{name: "file is not PNG", meta: func(t *testing.T, root string) string {
			writeFile(t, root, "docs/images/not-png.png", "not a png")
			return `<component><screenshots><screenshot type="default"><caption>mail</caption><image>https://raw.githubusercontent.com/frostyard/frostmail/main/docs/images/not-png.png</image></screenshot></screenshots></component>`
		}, problem: "not a PNG"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			meta := tc.meta(t, root)
			errs := checkScreenshots([]byte(meta), root)
			if len(errs) == 0 {
				t.Fatal("checkScreenshots returned no errors")
			}
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), tc.problem) {
					found = true
				}
			}
			if !found {
				t.Errorf("errors do not name %q: %v", tc.problem, errs)
			}
		})
	}
}
