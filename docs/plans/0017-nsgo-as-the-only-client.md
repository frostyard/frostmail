# Plan 0017: NSGo as the only client

- **Status:** Accepted in its order and its answers (Brian, 2026-11-24).
  Nothing of it is built. The owner set its end on 2026-11-24: the NSGo
  clients are the default, MyGo is removed, and the new versions are
  tagged and published. He answered its 6 questions the same day
  ([The owner's answers](#the-owners-answers)). FrostMail's decision is
  [ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md);
  Frostnotes' and PocketGo's ADRs are not written.

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
| How far NSGo is from MyGo | near: the list below | far: the list below | far, and MyGo is the only client of 3 platforms (corrected below) |
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

**PocketGo, corrected (2026-11-24).** The paragraph above is short of
what the 2 clients' code shows. Read side by side for PocketGo's own
plan (frostyard/pocketgo#176), the MyGo client shows 17 groups of things
the NSGo client does not: failed and loading states with a way to try
again, a run's failures, lessons in force, the status bar's counts and
usage, most of the work board's sentences, the Terminal place's states,
approve and merge's texts, and more. That plan holds the list. Nothing
was run for it.

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

- The owner answered the 6 questions (2026-11-24, below). Each answer
  that changes an accepted decision is an ADR in its repository.
  FrostMail's is [ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md),
  which supersedes [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
  and accepts [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md).
  Frostnotes' supersedes its ADR-0047 and ADR-0038, and PocketGo's its
  ADR-0070, ADR-0040, ADR-0041 and ADR-0044 in part; each is written
  when its phase starts.
- **Done when:** each repository has its accepted ADR.

## Phase 1 — The kit's next version (S, alongside)

- What the three clients asked the kit for (plan 0016, Phase 5): an
  `EmptyState` with buttons, a color for a `NavLabel`'s icon
  (FrostMail's flag rows, #401), a scale for a document's headings, a
  `FileExplorer` that reports its tree's width, the kept window as one
  part, which Frostnotes and FrostMail each have in their own code
  (frostyard/frostnotes#285; Phase 2 here), and a `Shell` that says
  whether its side panel is open and takes it at the start.
- **Built (2026-11-24).** The kit has the 6 with their tests
  (frostyard/frostui#79), and the owner tagged `workspaceui/v0.5.0`,
  then `v0.5.1` with the kept window's fix (frostyard/frostui#80; Phase
  3's second look on Windows). FrostMail takes `v0.5.1`: a flag's row of the navigation draws its icon in
  its flag's color (#401), the kit's `KeepWindow` keeps the window in
  place of the client's own code, under the same key, and the To-Do Bar
  opens as it was left (`todo-bar` in the client's storage). Left:
  Frostnotes and PocketGo take it.
- **Done when:** the kit is tagged with them, the 3 clients take it, and
  no client's style test lists an exception.

## Phase 2 — FrostMail: NSGo is the default (M)

- The missing things the owner keeps: the first-run welcome and the
  Flatpak's move note, the kept window and view state, and the
  calendar's drags, with the problem record. Each has its test and
  pictures. A window of its own for compose, an event, Settings and the
  reminders is not built: the client stays 1 window. The package's id and version are the released
  app's, stamped as MyGo's are.
- The Flatpak's launcher starts NSGo, and `FROSTMAIL_UI=mygo` starts MyGo
  for 1 release. The README's pictures are the NSGo suite's (plan 0016,
  Phase 4, closes here).
- A candidate, and plan 0013's clean-install gate run on it.
- **The kept window (2026-11-24), built.** The device keeps the
  window's size, its place where the platform reports one, the widths
  of the navigation, the To-Do Bar and the message list, and how the
  list is shown: conversations, contact photos, the sort and its
  direction (`cmd/frostmail-nsgo/window.go`). A start shows them. A
  window under 480 by 360 and a maximized one are not kept, and the
  window does not go under that size. It is in the client's storage,
  where the appearance is, not in the MyGo client's `window.json`: a
  person who moves from the MyGo client starts with the first sizes.
  - Not kept: whether the To-Do Bar is open (the kit's `Shell` holds
    that, and gives an app no way to set or follow it), and the
    console's height (the client has no console pane; Activity is a
    module).
  - `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11: 39 passed,
    2 of them new (a change is kept; a start shows what was kept, the
    real window's size with it). The test of a kept view fails with the
    keeping taken out. No picture is taken again: the window opens as
    it did where the device keeps nothing.
- **The problem record (2026-11-24), built.** When maild's local
  socket ends under the client while it is connected, the client
  records `bridge.socket_lost`, once, as the MyGo client does, and a
  release's build records its crash output
  (`cmd/frostmail-nsgo/main.go`). Lodge's connection never counts. The
  build takes the release's version from `FROSTMAIL_VERSION`
  (`scripts/build-nsgo-native.sh`), and any other build is `dev`, which
  records nothing.
  - **A release sets that version (2026-11-24), built.** The Flatpak's
    manifest passes `FROSTMAIL_VERSION=flatpak` to the client's build,
    and `stamp-version.sh` stamps it with the tag's version beside the
    ldflags, refusing a manifest that holds it other than once. A
    release's version, MAJOR.MINOR.PATCH, is the app's own version too,
    in place of `nsgo.yaml`'s fixed one. The Windows installer's build
    sets none, so the NSGo client there records nothing. The package's
    id is still the preview's.
  - `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11: 40 passed,
    1 of them new: the fixture's maild is ended under the client, 1
    record reaches a test journal, and the retries that follow add
    none. The test fails with the mark of a live connection taken out.
- **The first run (2026-11-24), built**
  ([specs/ui.md](../specs/ui.md#first-run-m6)). A store with no account
  shows a welcome in place of Mail, Calendar, People and Tasks: "Add
  Account…", the pane's one filled button, and, on this computer's
  maild, "Connect to lodge…", which opens Settings on Server. The first
  look at a connection that has no account opens the account's dialog
  itself, once. In the release Flatpak beside a host install's mail the
  welcome gives the 3 commands that move it, and a dismissal is kept on
  the device (`cmd/frostmail-nsgo/welcome.go`).
  - Different from the MyGo client: the client is 1 window, so "Add
    Account…" and the first look open the account's dialog over the
    welcome, not a Settings window; and a dismissal of the note in the
    MyGo client is not read, so the note shows once more after the
    switch.
  - A fault of its first commit, fixed in its second: the welcome's
    condition made the list and its search field be built again on
    every change of the store. A test holds it
    (`TestStoreChangesLeaveTheListInPlace`).
  - `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11: 46 passed,
    6 of them new (the welcome and its 2 buttons; the note and its
    dismissal; a dismissed note stays away; a store with accounts shows
    no welcome; a store change leaves the list in place; the pictures).
    2 pictures are new, and the agent looked at 1, the welcome with the
    note.
- **Every dialog has a quiet way out (2026-11-24), built.** A dialog's
  first button is Cancel where another confirms or destroys, and Close
  where it only shows something (`withWayOut` in
  `cmd/frostmail-nsgo/actions.go`). The account's dialog, which the
  first run opens by itself, had neither and closed by Escape alone, as
  did others with buttons of their own.
- **The calendar's drags, first part (2026-11-24), built**
  ([specs/pim-ui.md](../specs/pim-ui.md#changing-events-in-the-views)).
  In the month and the week, a drag of an occurrence to another day
  moves it by whole days, keeping its times: an all-day one by its
  dates, a timed one by its wall time, so that a move across a change
  of the clocks keeps the hour it shows. Only an occurrence of a
  writable calendar that is no invitation moves. A drop is written as
  an edit is, after the scope's question for a series and the notice
  to invitees (`cmd/frostmail-nsgo/calendar_drag.go`).
  - `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11: 50 passed,
    3 of them new (the arithmetic; who may move; in the week of a real
    window, an event dragged to another day is in that day's cell after
    and not in its own).
- **The calendar's drags, second part (2026-11-24), built.** The day
  and the week are laid out by time, 0.8 points a minute as the MyGo
  client's are: a day is a column 24 hours high under a strip of its
  date and all-day occurrences, an occurrence a block from its start to
  its end, and blocks that overlap stand side by side
  (`cmd/frostmail-nsgo/calendar_time.go`). They were lists of an hour's
  or a day's events. On them a drag changes an occurrence in steps of 15
  minutes: its block moves, across days in the week; its foot resizes
  it, to no less than a step; and a drag on empty time draws a block and
  opens the editor with that range. Nothing is written until a drag
  ends, Escape puts everything back, and a drag that ends where it began
  is a press. A press on empty time makes an event of that hour.
  - Not built: the now line; on a touch screen, any of these drags (a
    drag there scrolls the day).
  - Seen, not changed: a reload of the calendar builds the grid again,
    so a drag in progress when one arrives loses its block. The tests
    wait for the grid to rest before each drag.
  - Escape during a drag has no test: the suite's drag is 1 step from
    press to release.
  - `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11: 54 passed,
    4 of them new (the steps' arithmetic; the layout of overlapping
    blocks; in a real window, a block drawn, moved 30 minutes, resized
    15 and moved a day in the week; the pictures). 2 pictures are new,
    the week and the day, and the agent looked at the week.
- **The switch (2026-11-24), built; no release holds it.** The NSGo
  client's package has the released app's id, `org.frostyard.Frostmail`
  (it was the preview's). The Flatpak's launcher starts it, and
  `FROSTMAIL_UI=mygo` starts the MyGo client. The README's and the
  metainfo's pictures are the NSGo suite's (plan 0016, Phase 4).
  - The Flatpak's smoke test holds the launcher: a start with no
    `FROSTMAIL_UI` runs the NSGo client and no other, and
    `FROSTMAIL_UI=mygo` the MyGo client, each with a notification's
    arguments. It runs in the Flatpak's build, in CI.
  - Not known, and what the gate is for: how the client behaves in the
    real Flatpak (its storage under the new id, a second start handed
    to the first, a notification's click, the tray), and a person's
    first run in it.
  - What is left of this phase is the owner's: a candidate tag, and
    plan 0013's clean-install gate on it.
- **Done when:** a fresh VM installs the candidate and its first run, an
  account's sign-in, a message read, written and sent, and a restart all
  happen in the NSGo client with no variable set.

## Phase 3 — FrostMail: MyGo is removed (M)

- The shared packages are split so that the NSGo client's module no
  longer requires MyGo or frostui's root module. MyGo's client, its
  tests, `mygo.json` and its CI go. `internal/workflowcheck`, the risk
  paths, Dependabot, `AGENTS.md` and the design docs follow.
- Windows: an installer of the NSGo client, with its notifier, checked
  on the Windows VM before MyGo goes.
- **A first look on Windows (2026-11-24).** The client, built for
  Windows from the branch that makes it the default
  (`scripts/build-nsgo-native.sh`, no CGO), ran in the Windows 11 VM
  `frostmail-win11`, in the owner's signed-in desktop session, on a
  maild of its own with the showcase's mail (its own pipe, sync off,
  every file under `C:\fm\nsgo`). The installed Frostmail and its maild
  were not used. What was seen, in pictures of the screen:
  - With no desktop session, from SSH, the client stayed up 12 seconds
    with no window and nothing on its error output.
  - In the desktop session its window opened, drawn in Frostyard light
    with its own title controls: the navigation's 2 accounts, All
    Inboxes with 4 unread, the connection's green dot, and 4 on its
    taskbar icon.
  - A click on a message chose it and showed its header and its HTML
    page in the reader's frame; the unread count and the taskbar's
    became 3.
  - The Calendar's month drew, and Compose opened with a saved draft.
  - The window was about 980 wide on a screen of 1280 by 800, not the
    1280 it asks for.
  - **It found a fault of #418:** the compose window had gained a Cancel
    button that left it without Discard or Save and close. Fixed, with a
    test, in the change that records this.
  - **Not tried:** the keyboard, a send, a notification, the tray, a
    second start, a link, an attachment, print, a sign-in, the kept
    window, and the installer, which still starts the MyGo client. The
    pictures were sent to the owner and are not kept.
- **A second look on Windows (2026-11-24).** The same build, of before
  #423 and the kit's `v0.5.0`, in the same VM and fixture, driven from
  the desktop session by scripts: another program's `MoveWindow` for
  the window, Windows' `SendKeys` for the keyboard, and a pointer's
  click. What was seen, in the window's frame as Windows gives it, the
  client's storage file and pictures of the screen:
  - **The window opened** 960 by 732 at the screen's top left, on a
    screen of 1280 by 800.
  - **The kept window comes back smaller.** Left 1000 by 640 at 60, 20,
    the storage held `window` as 984 by 631 at 60, 20, placed. After a
    restart the window was 984 by 631 at 60, 20: 16 narrower and 9
    shorter than it was left. The client keeps its content's size, and
    asks for it at a start as the window's. The kit's `v0.5.1` keeps
    the window's own size (frostyard/frostui#80). Seen with it, in the
    same VM: left 1040 by 660 at 80, 30, the window came back 1040 by
    660 at 80, 30, and the navigation's Red and Orange rows drew in
    their flags' colors.
  - **The least size holds.** Asked for 300 by 200, the window took 496
    by 369, and the storage held 480 by 360.
  - **A second start** ended by itself within 6 seconds with code 0,
    and 1 process with its window was left. Whether the window came to
    the front was not looked at.
  - **Keys.** Ctrl+K opened the palette, whose items show their keys
    as Windows writes them; Escape closed it; Ctrl+N opened Compose;
    Ctrl+comma opened Settings.
  - **Typing.** Text typed into Cc and Bcc showed there, Tab moved
    between the fields, and a word that is no address was named under
    them. Discard asked in an alert, and its Discard draft closed
    Compose.
  - **Compose opens with no field holding the keyboard:** text typed
    before a Tab went nowhere, 2 times of 2. No code of the client
    gives a field the focus there, so this is not Windows' alone.
  - **Seen once, and not explained:** after Escape closed Compose, the
    focus ring was on an account's header in the navigation, and
    Ctrl+N opened nothing, 2 presses. With a row of the navigation
    pressed first, it opened.
  - **Not tried:** a send, a notification, the tray, a link, an
    attachment, print, a sign-in, a maximized window, and the
    installer. The pictures are not kept.
- **The NSGo client's installer (2026-11-25), built; installed on no
  machine.** A second installer, whose program is the NSGo client, is
  built beside MyGo's by NSGo's own tool (`make nsgo-installer`,
  `scripts/nsgo-windows-installer.sh`;
  [design/desktop.md](../design/desktop.md#windows)). maild, mailctl, the
  notifier and the toast's icon ship beside `FrostMail.exe`. NSGo's NSIS
  script has no place for an app's own steps, so `tools/nsgoinstaller`
  writes them into it (`packaging/windows/nsgo-installer.nsh`): they are
  `installer.nsh`'s. MyGo's installer is not changed, and its program is
  still the MyGo client.
  - CI's Windows job on the change's first commit
    ([run 45553103042](https://github.com/frostyard/frostmail/actions/runs/45553103042))
    built `frostmail-0.9.0-windows-x64-setup.exe`, 19,565,808 bytes, and,
    under a check's names, `frostmail-check-0.9.0-windows-x64-setup.exe`,
    19,567,436 bytes. makensis gave 1 warning for each, that the version
    information has no `LegalCopyright`. The same job built MyGo's
    installer, 26.5 MB, and passed its install check. That run's log does
    not name the makensis that compiled the 2 new installers: the script
    printed none then.
  - `make check` passes; `tools/nsgoinstaller`'s tests run on the script
    that NSGo v0.3.0's build wrote on Linux.
  - Read in the code, and seen in no install: the uninstall entry is
    MyGo's (`org.frostyard.Frostmail`), and Windows' file names ignore
    case, so this installer installs into MyGo's folder over its files;
    the client starts `maild.exe` from beside it
    (`internal/desktop/conn`), maild runs `frostmail-notify.exe` from
    beside it, and the notifier's click runs `frostmail.exe` beside it,
    which `FrostMail.exe` answers to there.
  - Read in NSGo's script, and seen in no uninstall: its `CloseApp` stops
    every process whose program is in the install folder, and does not
    leave out the uninstaller. A silent uninstall run with `_?=`, as CI
    runs MyGo's, runs from that folder.
  - **Not done:** no installer of it was run. An install, an upgrade over
    MyGo's install and over a Tauri one, the toast and its click, the
    second start, and the uninstall are the Windows VM's check, which is
    still to do before MyGo goes.
- **Done when:** `go.mod` names neither `github.com/egoist/mygo` nor
  frostui's root module, `make ci` passes, and the owner releases the
  version (`make bump`, then Promote).

## Phase 3.5 — FrostMail in Lodge (M)

- The NSGo web build takes the React app's place in Lodge, as
  Frostnotes' did, after MyGo is gone.
- **Done when:** the `.deb` holds the NSGo web build and no React app,
  and `dev/lodge/nsgo-smoke.py` passes on what it installs.

## Phase 4 — PocketGo (L)

- The Linux and web gaps above, a test of the terminal's clipboard rule,
  and 1 place for the preferences with a move from the other.
- The iPad, iPhone and macOS clients are ported to NSGo's targets for
  them, which that repository has never built: the phone's and the
  tablet's rules (suspend and resume, no terminal, the compact tab bar)
  with them. MyGo goes when they work.
- MyGo is removed: `desktop/internal/app`, `desktop/cmd/pocket`,
  `mygo.json`, and what names them. Its first release is cut: there is
  none today.
- **Done when:** `desktop/go.mod` names neither MyGo nor frostui's root
  module, and the owner tags and promotes `v0.1.0`.

## Phase 5 — Frostnotes (XL)

- All 4 groups of MyGo's features are kept: version history with
  compare and revert, the merge view and the sync conflicts; attachments,
  images and files that are not Markdown; live preview's missing marks;
  the side panes, tabs and splits. Each is a change with its tests and
  pictures, as plan 0016's were. What the inventory lists outside the 4
  groups (a hub's devices, folder operations, daily notes and templates,
  a vault's settings, the editor's commands) has no answer yet: it is
  asked when the phase starts.
- MyGo is removed, and the React app is reduced to the NSGo build's
  loader or the loader is made without it.
- **Done when:** the Flatpak holds 1 client, the root `go.mod` names
  neither MyGo nor its fork's `replace`, and the owner releases the
  version.

## Later / ideas

- macOS builds of FrostMail and Frostnotes: neither has one today.
- A Windows build of Frostnotes: both of its clients fail to build there
  (`internal/desktop/system/lifetime.go`).

## The owner's answers

Brian answered these on 2026-11-24, each as a choice among options the
agent set out.

1. **Frostnotes keeps all 4 groups** of MyGo's features before MyGo goes:
   history and conflicts, attachments and images, the full live preview,
   and panes and tabs.
2. **PocketGo's iPad, iPhone and macOS clients are ported to NSGo.**
3. **FrostMail's NSGo client ships on Windows** in the release that
   removes MyGo, checked on the Windows VM.
4. **FrostMail builds 3 of the missing things** before the switch: the
   first-run welcome, the kept window, and the calendar's drags. The
   separate windows are not built. The problem record is ported whatever
   the answer.
5. **FrostMail's NSGo web build replaces the React app in Lodge,** after
   MyGo is gone.
6. **The order is FrostMail, PocketGo, Frostnotes.**

## Open questions

- **Frostnotes: what of MyGo's features outside the 4 groups is kept?**
  The owner, when Phase 5 starts.
- **NSGo's iOS and macOS targets for PocketGo:** whether they build and
  run it is not known; the first change of Phase 4's port finds out.

## References

- Follows: [plan 0016](0016-a-shared-look-for-the-nsgo-apps.md),
  [plan 0015](0015-mygo-desktop.md), [plan 0013](0013-m6-the-standalone-desktop-flatpak.md)
- Decided by: [ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md)
- Decisions it changes: [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
  [ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)
