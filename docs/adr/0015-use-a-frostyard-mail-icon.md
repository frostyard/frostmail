# 0015 — Use a Frostyard mail icon

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

The original application icon is a white envelope on a rounded blue tile.
It identifies mail but has no distinct connection to Frostyard. The application
needs an identity that remains recognizable in launchers and notifications.

## Decision

Use a faceted envelope carrying a snowflake-marked letter, on a chamfered dark
tile. Take the cold palette from core's `frostyard-design/tokens/colors.css`:
ink, deep, panel, ice, sky, blue, text-body and line-strong. Use sharp folds and
restrained gradients, without shadows or typography.

Keep an editable SVG master in `app/src-tauri/icons/icon.svg` and render the
existing 32, 128 and 512 pixel RGBA PNGs from it. Both Tauri and Flatpak keep
using those PNG paths. The application icon does not replace Frostyard's
organization wordmark or change the reader UI's tokens.

## Consequences

The icon has a recognizable Frostyard palette and a reproducible vector source.
Regeneration requires Python with PyGObject, GdkPixbuf and librsvg; regular app
builds use the checked-in PNGs and need no additional tools. Small-size previews
must be checked after changing the master.

## Alternatives considered

- **Recolor the existing envelope:** retains the generic silhouette.
- **Painted raster artwork:** harder to maintain and less predictable at small
  notification sizes.

## References

- Shapes: [Running on the desktop](../design/desktop.md#application-icon)
- Tokens: [Frostyard design system](https://github.com/frostyard/core/blob/main/.agents/skills/frostyard-design/tokens/colors.css)
