# 0058 — Pin MyGo's quote-depth fix from the owner's fork

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Frostmail #241 loses quotation levels when a list occurs inside nested
quotations. MyGo v0.9.0 represents quotation membership with a boolean and
uses paragraph indentation for list levels. The compose converter cannot
preserve both dimensions through editor operations with that representation.

The editor change is proposed upstream in
[MyGo #1731](https://github.com/egoist/mygo/pull/1731), from the owner's fork.
The owner chose to use that fork while the upstream change is reviewed.

## Decision

Replace `github.com/egoist/mygo` with an immutable pseudo-version of
`github.com/bketelsen/mygo` containing the quote-depth change, based on
upstream commit `0d644f4a`. Keep imports on the upstream module path.
Record the exact version and checksums in `go.mod` and `go.sum`, and
regenerate the Flatpak's offline sources for that version.

The compose converter stores quotation depth in `ParagraphStyle.QuoteDepth`
and list depth in `Indent`. It preserves them through save, split, join,
undo and list formatting.

After upstream includes the change, switch to an upstream version containing
it, remove the replacement, regenerate offline sources and run the Go,
desktop and compose checks again. A later fork branch update is not consumed
automatically.

## Consequences

The fix can ship before the upstream review finishes. Frostmail temporarily
owns a dependency replacement and must track the upstream PR. The pinned
fork also includes upstream changes between v0.9.0 and `0d644f4a`; the normal
application checks cover the combined dependency update.

## Alternatives considered

- **Wait for upstream:** leaves #241 unresolved until the change is merged
  and consumed; the owner chose the temporary fork instead.
- **Overload list indentation:** cannot represent quotation and list depth
  independently and loses information during editor operations.
- **Use a local directory replacement or moving branch:** would not provide
  reproducible CI and release builds.

## References

- Shapes: [plan 0015, Compose](../plans/0015-mygo-desktop.md#phase-5--compose-l).
- Builds on: [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md).
- Fixes: [Frostmail #241](https://github.com/frostyard/frostmail/issues/241).
