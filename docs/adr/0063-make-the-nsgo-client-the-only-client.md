# 0063 — Make the NSGo client the only client

- **Status:** Accepted (Brian, 2026-11-24)
- **Date:** 2026-11-24

## Context

FrostMail has 3 user interfaces over maild's one API:

- **The MyGo client** (`cmd/frostmail`, `internal/desktop/`), which
  [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
  chose for the desktop. It is the default: the Flatpak's launcher starts
  it, and the Windows installer's program is it.
- **The NSGo client** (`cmd/frostmail-nsgo`), which
  [ADR-0061](0061-share-the-mail-client-with-nsgo.md) proposed as one view
  tree for Linux, Windows and Lodge. The Flatpak and the Windows installer
  hold it, and nothing starts it unless `FROSTMAIL_UI=nsgo` is set.
  [ADR-0062](0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
  and [plan 0016](../plans/0016-a-shared-look-for-the-nsgo-apps.md) gave
  it Frostyard's look from the kit that Frostnotes and PocketGo share.
- **The React app** (`app/`), which Lodge serves.

Each interaction is written 3 times, and a change of the product lands in
1 of them first. The NSGo client lacks some of what the MyGo client does
([plan 0017](../plans/0017-nsgo-as-the-only-client.md#what-is-true-today)).
It has run under test in a Linux window and in Lodge in a browser, and in
no Windows window.

On 2026-11-24 the owner set the end: the NSGo clients are the default,
MyGo is removed, and the new versions are released. He answered plan
0017's questions the same day.

## Decision

The NSGo client becomes FrostMail's only client, in 3 steps, each a
release or a part of one.

1. **It becomes the default on the desktop.** Before that it gets the
   first-run welcome with the Flatpak's move note, the kept window and
   view state, the calendar's drags, and the `bridge.socket_lost` problem
   record. It stays 1 window: compose, an event, Settings and the
   reminders are dialogs in it, and get no window of their own. Its
   package takes the released app's id and version. `FROSTMAIL_UI=mygo`
   starts the MyGo client for 1 release after the switch.
2. **MyGo is removed,** on Linux and on Windows. The Windows installer
   holds the NSGo client and starts it, and that is checked in a window
   on the Windows VM before the MyGo client goes. The packages the NSGo
   client shares with the MyGo client are split first, so that its module
   requires neither MyGo nor frostui's root module.
3. **The NSGo web build takes the React app's place in Lodge,** after
   step 2.

This accepts ADR-0061's one view tree for the 3 hosts, and supersedes
ADR-0054's choice of MyGo and frostui for the desktop.

## Consequences

- One client to change, test and draw. The kit's standard for an app
  (frostui's `docs/specs/workspaceui.md`) and its live suite are the
  product's, not a second client's.
- What the MyGo client has and the decision leaves out is gone at step
  2: a window of its own for compose, an event, Settings and the
  reminders. Print stays a text rendering and not the page, and a
  reminder keeps 1 snooze, unless a later change adds them.
- [Plan 0015](../plans/0015-mygo-desktop.md)'s parity gate (#253), of the
  MyGo client on real desktops, is not passed and will not be: it ends
  with the client it is of.
- Windows gains a risk: the NSGo client has run in no Windows window
  under test. Step 2 does not happen until it has.
- MyGo's own test tool goes with it (`internal/desktop/e2e`,
  `make desktop-e2e`). The NSGo suite (`make nsgo-test`, `make nsgo-live`)
  is the client's test, and has no Windows run.
- The repository keeps a second module (`cmd/frostmail-nsgo/go.mod`) and
  requires NSGo, which frostyard does not own.
- Step 3 ends the React app, and with it `app/`'s build, its tests and
  its part of CI. Until then Lodge serves React.
- The release steps do not change: Brian tags a candidate, runs the
  clean-install gate, `make bump` and Promote
  ([plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md)).

## Alternatives considered

- **Keep MyGo as the default and the NSGo client as an option:** every
  interaction stays written 3 times, which is what the owner ended.
- **Keep MyGo on Windows only:** it leaves the second client and its
  build tool for 1 platform. The owner chose the NSGo client there,
  checked first.
- **No Windows build in the release that removes MyGo:** FrostMail ships
  on Windows today ([ADR-0038](0038-windows-as-a-second-platform.md)).
- **Replace the React app in Lodge in the same release as the switch:**
  the owner put it after MyGo's removal, so that 1 host changes at a
  time.

## References

- Shapes: [plan 0017](../plans/0017-nsgo-as-the-only-client.md),
  [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)
- Builds on: [ADR-0061](0061-share-the-mail-client-with-nsgo.md),
  [ADR-0062](0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
- Supersedes: [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
