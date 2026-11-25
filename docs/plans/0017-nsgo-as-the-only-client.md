# Plan 0017: NSGo as the only client

- **Status:** Proposed (2026-11-24). Nothing of it is built. The owner set
  its end on 2026-11-24: the NSGo clients are the default, MyGo is removed,
  and the new versions are tagged and published. Its open questions are the
  owner's to answer before the phases they gate.

This plan takes FrostMail, Frostnotes and PocketGo from two desktop clients
each to one, the NSGo client on the shared kit
(`github.com/frostyard/frostui/workspaceui`), and to a release of each. It
follows [plan 0016](0016-a-shared-look-for-the-nsgo-apps.md), which gave the
three NSGo clients one look and says that changing FrostMail's default is
not its part. It is written here because plan 0016 is; each repository
records its own decision as an ADR before its switch, and keeps its own
status of its phases.

## What is true today

Read from each repository's `main` on 2026-11-24 by read-only agents. A
phase checks its items again when it starts: this list orders the work, and
is no record of a check.

| | FrostMail | Frostnotes | PocketGo |
| --- | --- | --- | --- |
| Default on Linux | MyGo (`FROSTMAIL_UI=nsgo` chooses NSGo in the Flatpak's launcher) | NSGo (`--mygo` chooses MyGo) | NSGo (`make build-mygo` builds MyGo) |
| In Lodge | the React app | NSGo's web build | NSGo's web build, outside Lodge |
| Windows | MyGo's installer, which also holds `frostmail-nsgo.exe` and does not start it | no build | no build shipped |
| macOS, iPad, iPhone | none | none | MyGo only, built by hand on a Mac |
| How far NSGo is from MyGo | near: the list below | far: the list below | near on Linux; MyGo is the only client of 3 platforms |
| Last release | `v0.4.0` | `v0.8.0` | candidates `v0.1.0-rc.1` to `rc.4`; no release |

**What MyGo does that NSGo does not, FrostMail:** the first-run welcome
and the Flatpak's move note; the `bridge.socket_lost` problem record and
the crash output; the kept window and view state (`window.json`); moving,
resizing and drawing an event by a drag in the calendar; a window of its
own for compose, an event, Settings and the reminders; the Keyboard
Shortcuts dialog and 2 keys (the sidebar's, Activity's); Windows'
notifier. Different, not missing: print (the text rendering, not the page),
reminders (1 snooze). Its NSGo package is still `org.frostyard.Frostmail.Preview`
at a fixed version that the release's stamp does not reach. Open:
#392, #401, #402, #403, and #253, plan 0015's parity gate of the MyGo
client.

**Frostnotes:** version history with compare and revert, and the merge
view; the list of sync conflicts and their resolving; a hub's devices,
approvals and leaving; attachments, images and files that are not
Markdown; the reading view; the side panes (backlinks, outgoing links,
tags, properties, outline, tasks) and the properties' editor; tabs, splits
and back and forward; folder operations; daily notes and templates; a
vault's settings and the editor's preferences; some 50 editor commands,
find in a note among them; most of live preview's marks (rules, tags,
footnotes, math, tables, callouts, code, embeds, Markdown links); the
problem record; in Lodge, routes to a note and Settings. Its NSGo package
is `org.frostyard.Frostnotes.Preview` too.

**PocketGo:** the iPad's and iPhone's rules (suspend and resume, no
terminal, the compact tab bar); the list of lessons in force; the
worktree's folder and branch shown before a new session is sent; the
status bar (counts, pocketd's version, usage by provider, the unsaved
GitHub sign-in); some board and sidebar notes; 4 commands. On the web: no
owner sign-in and no merge. Its preferences are kept in 2 places with no
move between them. Its NSGo terminal leaves a program's clipboard reads
and writes off, as NSGo does by default (checked in the code,
2026-11-24: the view sets neither `AllowClipboardRead` nor
`AllowClipboardWrite`), and has no test that holds it, as MyGo's has.

**What a removal deletes or must first split.** In FrostMail the NSGo
client imports packages that import MyGo (`internal/desktop/compose`,
`organize`, `settings`, `uikit`, `mail`, `icons`) for their plain helpers,
and calls `frostui.Opener` and `frostui.TrayHosted`: those are split
before `cmd/frostmail`, `internal/desktop/{mainwin,pim,commands,e2e}` and
`mygo.json` go, and the Windows installer, which MyGo's tool builds, is
made another way. In Frostnotes `internal/desktop/system/open.go` imports
frostui, 1 test imports MyGo, and the React app's Vite build still makes
the NSGo web build's loader. In PocketGo 3 tests of `internal/state` use
frostui's test loop, and the Makefile, CI, lint rules and 2 script tests
name MyGo.

## Phase 0 — Decisions (S)

- The owner answers the open questions below. Each answer that changes an
  accepted decision is an ADR in its repository: FrostMail supersedes
  [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
  and accepts or rewrites [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
  which is still Proposed; Frostnotes supersedes its ADR-0047 and
  ADR-0038; PocketGo supersedes its ADR-0070 and, by the answer, ADR-0040,
  ADR-0041 and ADR-0044.
- **Done when:** each question has an answer in an accepted ADR, or is
  marked as not gating any phase.

## Phase 1 — The kit's next version (S, alongside)

- What the three clients asked the kit for (plan 0016, Phase 5): an
  `EmptyState` with buttons, a color for a `NavLabel`'s icon
  (FrostMail's flag rows, #401), a scale for a document's headings, a
  `FileExplorer` that reports its tree's width, and the kept window as one
  part, which Frostnotes has in its own code (frostyard/frostnotes#285)
  and the other 2 need.
- **Done when:** the kit is tagged with them, the 3 clients take it, and
  no client's style test lists an exception.

## Phase 2 — FrostMail: NSGo is the default (M)

- The missing things above that the owner keeps (question 4), each with
  its test and pictures. The package's id and version are the released
  app's, stamped as MyGo's are.
- The Flatpak's launcher starts NSGo, and `FROSTMAIL_UI=mygo` starts MyGo
  for 1 release. The README's pictures are the NSGo suite's (plan 0016,
  Phase 4, closes here).
- A candidate, and plan 0013's clean-install gate run on it.
- **Done when:** a fresh VM installs the candidate and its first run, an
  account's sign-in, a message read, written and sent, and a restart all
  happen in the NSGo client with no variable set.

## Phase 3 — FrostMail: MyGo is removed (M)

- The shared packages are split so that the NSGo client's module no
  longer requires MyGo or frostui's root module. MyGo's client, its
  tests, `mygo.json` and its CI go. `internal/workflowcheck`, the risk
  paths, Dependabot, `AGENTS.md` and the design docs follow.
- Windows by question 3: an installer of the NSGo client, checked on the
  Windows VM, or no Windows build in this release.
- In Lodge by question 5.
- **Done when:** `go.mod` names neither `github.com/egoist/mygo` nor
  frostui's root module, `make ci` passes, and the owner releases the
  version (`make bump`, then Promote).

## Phase 4 — PocketGo (M, or L by question 2)

- The Linux and web gaps above, a test of the terminal's clipboard rule,
  and 1 place for the preferences with a move from the other.
- The 3 platforms by question 2.
- MyGo is removed: `desktop/internal/app`, `desktop/cmd/pocket`,
  `mygo.json`, and what names them. Its first release is cut: there is
  none today.
- **Done when:** `desktop/go.mod` names neither MyGo nor frostui's root
  module, and the owner tags and promotes `v0.1.0`.

## Phase 5 — Frostnotes (L or XL by question 1)

- The features the owner keeps, in an order he sets. Each is a change
  with its tests and pictures, as plan 0016's were.
- MyGo is removed, and the React app is reduced to the NSGo build's
  loader or the loader is made without it.
- **Done when:** the Flatpak holds 1 client, the root `go.mod` names
  neither MyGo nor its fork's `replace`, and the owner releases the
  version.

## Later / ideas

- macOS builds of FrostMail and Frostnotes: neither has one today.
- A Windows build of Frostnotes: both of its clients fail to build there
  (`internal/desktop/system/lifetime.go`).

## Open questions

Each is the owner's. The agent's lean is given as a lean.

1. **Frostnotes: which of MyGo's features must the NSGo client have
   before MyGo goes, and which are dropped?** It decides whether Phase 5
   is weeks or months. Lean: keep version history and revert, sync
   conflicts, attachments and images, and live preview's missing marks;
   decide the side panes, tabs, daily notes and templates one by one. By
   Phase 5.
2. **PocketGo: the iPad, iPhone and macOS clients are MyGo's.** Port
   them to NSGo's targets for those platforms, which this repository has
   never built; keep MyGo for them alone, so that "removed" means Linux;
   or stop shipping them. Lean: none; it turns on whether the owner uses
   the iPad client. By Phase 4.
3. **Windows.** FrostMail's installer starts MyGo, and its NSGo client
   has run in no Windows window under test. Is Windows in the release
   that removes MyGo? Lean: yes for FrostMail, checked on the Windows VM,
   since it ships there today. By Phase 3.
4. **FrostMail: which of the missing things are kept?** Lean: the first
   run, the problem record, the kept window and the calendar's drags are
   kept; the separate windows are not (ADR-0061 chose 1 window); the
   Windows notifier by question 3. By Phase 2.
5. **FrostMail in Lodge.** Lodge serves the React app, and the NSGo web
   build is built and not shipped. Does this plan replace the React app
   there, as Frostnotes did? Lean: yes, after Phase 3, as its own phase.
   By Phase 3.
6. **The order.** FrostMail, then PocketGo, then Frostnotes, by how near
   each is. Lean: that order, with Phase 1 alongside. By Phase 2.

## References

- Follows: [plan 0016](0016-a-shared-look-for-the-nsgo-apps.md),
  [plan 0015](0015-mygo-desktop.md), [plan 0013](0013-m6-the-standalone-desktop-flatpak.md)
- Decisions it changes: [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
  [ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)
