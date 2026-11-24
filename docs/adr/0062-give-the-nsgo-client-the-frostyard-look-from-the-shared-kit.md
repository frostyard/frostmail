# 0062 — Give the NSGo client the Frostyard look from the shared kit

- **Status:** Accepted (Brian, 2026-11-24)
- **Date:** 2026-11-24

## Context

The NSGo client ([ADR-0061](0061-share-the-mail-client-with-nsgo.md)) has
the MyGo client's behaviour and not its look. It sets no theme, so it draws
NSGo's stock grey and blue; its toolbar and reader actions are text
buttons; its rows carry their marks as characters in the sender's name
([plan 0016](../plans/0016-a-shared-look-for-the-nsgo-apps.md#what-is-wrong-today)).
The product's bar is Mail.app's polish, and the MyGo client drew
Frostyard's brand ([plan 0015](../plans/0015-mygo-desktop.md#the-frostyard-look)).

Frostnotes and PocketGo have NSGo clients with the same gaps. The three
share `workspaceui`, 6 files in `frostyard/pocketgo` at
`desktop/workspaceui/v0.1.0-rc.1`, which holds a shell and a few surfaces
and no theme.

NSGo v0.3.0 constrains the choice:

- A theme sets every color role and the fonts. The `ui` package fixes its
  corners (8 for controls, 12 for cards), its heights and its type scale.
- Under a hidden title bar, the GTK backend draws no window controls and
  has no call to resize the window from its edges. `workspaceui.Shell`
  restores the system's title bar on Linux for that reason.

## Decision

1. **The look (D1).** The NSGo client draws Frostyard's colors, in a dark
   and a light theme that follow the desktop, with Inter. The colors are
   those of frostui's theme spec. The layout and its measures follow
   GoVault's three-pane window (a toolbar of icon buttons with one filled
   button, a sidebar with counts at the rows' ends, summary rows with an
   avatar, grouped cards). Corners are NSGo's own: 8 for controls and rows,
   12 for cards. Frostyard's nearly square corners are not carried over.
2. **The kit's home (D2).** The shared kit moves from `frostyard/pocketgo`
   to `frostyard/frostui` as a module of its own,
   `github.com/frostyard/frostui/workspaceui`, beside the MyGo kit and with
   no MyGo in its `go.mod`. The package keeps its name. FrostMail, Frostnotes
   and PocketGo pin its tags.
3. **The title bar on Linux (D3).** Linux keeps the system's title bar, and
   the toolbar sits under it, until NSGo can resize an undecorated GTK
   window and show its controls. Windows hides the title bar and uses
   NSGo's controls. The shell stops drawing the app's name where the
   system's bar shows it.
4. **The selected row (D4).** A chosen row takes the accent at a low alpha,
   with rounded corners, and keeps its own text colors. The kit's gallery
   pictures of plan 0016's Phase 2 confirm this, or a new ADR changes it.

App code holds no color, text size, corner or padding number of its own: it
takes a part or a token of the kit, and a test holds that
([plan 0016](../plans/0016-a-shared-look-for-the-nsgo-apps.md#rules-for-every-app),
rule 2).

## Consequences

- The three apps look alike by construction: a change of the look is a
  change of the kit and a version bump in each app.
- A fourth repository takes part in a UI change. A kit change is merged and
  tagged in frostui before an app can use it, and the maintainer tags.
- frostui holds two kits with two tag series (`vX.Y.Z` for the MyGo kit,
  `workspaceui/vX.Y.Z` for this one) and two sets of import rules.
- FrostMail's NSGo client and its MyGo client no longer match corner for
  corner. The README's pictures change when plan 0016's Phase 4 closes.
- On Linux the window has two rows at its top (the system's title bar and
  the toolbar) until NSGo closes its gap; the kit's spec names the
  workaround to remove then.
- The import path changes once in each of the three apps, and PocketGo
  removes its copy of the module.

## Alternatives considered

- **A neutral, macOS-like palette, as GoVault's:** it gives up the brand
  the other Frostyard apps and Lodge draw.
- **Frostyard's square corners:** `ui` fixes its radii, so every control
  would be drawn again from `base`; the gain is small beside the cost of
  owning every control.
- **Leave the kit in `frostyard/pocketgo`:** kit changes would go through
  PocketGo's gates and releases, and the look's spec, its ADRs, Inter and
  the contrast table are already in frostui.
- **A new repository for the kit:** a whole baseline to build and keep for
  6 files, when frostui has it.
- **Draw window controls and resize edges in the kit now:** it copies what
  the framework should do, on the one system where a wrong resize edge
  makes the window unusable.
- **A full accent fill for the chosen row, as Mail.app's and GoVault's:**
  every text and mark in the row needs a second color, and the flag's and
  the unread dot's colors are lost on the fill.

## References

- Shapes: [plan 0016](../plans/0016-a-shared-look-for-the-nsgo-apps.md),
  [design/app.md](../design/app.md)
- Builds on: [ADR-0061](0061-share-the-mail-client-with-nsgo.md),
  [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- frostui: `docs/specs/frostyard-theme.md`, and the kit's own ADR and spec
  there (plan 0016, Phase 0)
- pocketgo: ADR-0068
