// Package problemrec makes the problem recorders of maild and mailctl
// (ADR-0049): github.com/frostyard/std/problems with Frostmail's app ID and
// the release version of the build, or an inert recorder for a build that
// is no release.
package problemrec

import (
	"strings"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/internal/config"
)

// Version maps a build's version (the ldflags main.version) to the version
// of its problem records: it strips 1 leading "v", so that `make build` on
// the tag v0.4.1 gives 0.4.1. It never invents a version: a placeholder
// (dev, flatpak, 0.0.0-<commit>-snapshot, a git describe past a tag such as
// v0.4.1-3-gabc1234 or one that ends in -dirty) stays as it is, and the
// recorder that New makes of it is inert.
func Version(build string) string {
	return strings.TrimPrefix(build, "v")
}

// New returns the recorder of component for the build version build. It is
// inert unless Version(build) is a release semver (1.2.3 or 1.2.3-rc.1);
// its Err says why.
func New(build string, component problems.Component, opts ...problems.Option) *problems.Recorder {
	return problems.New(config.AppID, Version(build), component, opts...)
}
