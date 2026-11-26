# Plan 0017: NSGo as the only client

- **Status:** Accepted in its order and its answers (Brian, 2026-11-24).
  FrostMail's Phase 2 is done, on the owner's report of its gate
  (2026-11-26, [Phase 2](#phase-2--frostmail-nsgo-is-the-default-m)),
  and `v0.10.0` holds it. Its Phase 3, MyGo's removal, is built, and
  its installer was checked on the Windows VM (2026-11-26); it waits on
  a release ([Phase 3](#phase-3--frostmail-mygo-is-removed-m)). Its Phase
  3.5, the NSGo web build in Lodge in place of the React app, is built:
  both pieces of its Done when were seen on a snapshot `.deb` of its
  change (2026-11-26), which lists 11 things the React app did that the
  web build does not; no release holds it
  ([Phase 3.5](#phase-35--frostmail-in-lodge-m)). The owner set its
  end on 2026-11-24: the NSGo clients are the default, MyGo is removed,
  and the new versions are tagged and published. He answered its 6 questions the same day
  ([The owner's answers](#the-owners-answers)). FrostMail's decision is
  [ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md);
  PocketGo's is its ADR-0071. On 2026-11-25 the owner took Frostnotes
  out of this plan: it has no desktop window, and Lodge serves its React
  app again ([Phase 5](#phase-5--frostnotes-xl)). On 2026-11-26 he
  retired PocketGo, which leaves this plan too
  ([Phase 4](#phase-4--pocketgo-l)). FrostMail is the plan's only app.

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
- **The gate (2026-11-26), done on the owner's word.** Brian tagged
  `v0.10.0-rc.1`, ran plan 0013's clean-install gate on it, then
  released `v0.10.0` from it and ran Promote (2026-11-26). His report
  of the gate is "clean install passed well" (Brian, 2026-11-26). It
  names none of the Done when's steps one by one (the first run, the
  sign-in, a message read, written and sent, the restart, no variable
  set), and records no times, counts or pictures. The phase is closed
  on that report of Brian's, without those pieces.
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
- **MyGo's removal (2026-11-26), built; no release holds it.** It was
  built before the Windows VM's check of the installer that this phase
  puts first, and its pull request waited on that check (below).
  - `cmd/frostmail`, `mygo.json`, MyGo's `installer.nsh` and script, and
    the MyGo-only packages (`internal/desktop/commands`, `e2e`, `mainwin`,
    `pim`, `uikit`) are gone, with MyGo's make targets (`desktop-build`,
    `desktop-build-windows`, `desktop-installer`, `desktop-e2e`) and its
    CI job.
  - The packages the NSGo client imports keep their paths and have no
    UI: `compose`, `organize` and `settings` keep their logic and its
    tests, `icons` the tray icon, and `mail` takes the reader's page and
    the thumbnail from `uikit`. Links and attachments are checked by
    `internal/desktop/opener` (frostui's `Opener`, without MyGo) and
    opened with NSGo's calls; `internal/desktop/trayhost` asks whether a
    panel hosts the tray (frostui's `TrayHosted`, over godbus). `go mod
    why` answers that neither module needs `github.com/egoist/mygo` or
    `github.com/frostyard/frostui`; the client's module needs
    `frostui/workspaceui`.
  - Seen, changed: the thumbnail decoded BMP through MyGo's package
    alone; it registers its 5 formats itself now, and a test decodes
    each. The MyGo print guard and print header go with the page they
    were for.
  - The Flatpak builds no MyGo client and no `libayatana-appindicator`
    modules, which only its tray opened. `FROSTMAIL_UI=mygo` is ignored:
    the launcher says so on its error output and starts the NSGo client,
    and the smoke test holds both, and that no `frostmail-app` runs.
  - Windows' installer is the NSGo client's (`make windows-installer`,
    `packaging/windows/installer.nsh`). CI's Windows job builds it and
    installs it: the refused folder, the files and keys, the installed
    client's window, and a silent uninstall that keeps the mail. The
    uninstall runs from NSIS's copy of the uninstaller, without `_?=`,
    since NSGo's `CloseApp` stops what runs from the install folder.
    On the change's first run
    ([run 45821693143](https://github.com/frostyard/frostmail/actions/runs/45821693143)),
    the job compiled `frostmail-0.9.0-windows-x64-setup.exe`, 19,123,381
    bytes, and the check's, 19,121,152 bytes, with the NSIS 3.13 it
    downloaded (1 warning each, no `LegalCopyright`), and the install
    check passed. The same run's Windows `go test` failed once: an
    opener test of the execute bit, which Windows lacks; the test now
    leaves that case out there.
  - Gone: the MyGo client's real-window suite, with its reader script
    probe (`TestReaderPageRunsNoScript`, which ran a page in a real web
    view). `TestMessagePageRunsNoScript` holds the NSGo client's message
    view to `NoJavaScript`, `Incognito`, `mail.PageCSP` first in its
    head, no base URL, and a window's link opened only on the user's
    press after the opener's check; it fails with `NoJavaScript` off or
    the press's check taken out. Nothing runs a page in a real web view
    to see that no script runs.
  - Run on Linux, each exiting 0: `make check`, `make nsgo-test`, `make
    nsgo-live` on Xvfb's X11 (58 passed, 0 failed), `make race` (65
    packages ok), `make flatpak-sources`, `make nsgo-build
    nsgo-build-windows`, and the smoke test on host builds of maild,
    mailctl and the launcher. No kept picture changed: the page and the
    thumbnail are the same code. `make
    windows-installer` on Linux, with a stand-in notifier, wrote the
    script with Frostmail's steps and stopped at makensis, which the
    host lacks. `make ui-check` was not run (no nsl machine of
    FrostMail's here).
- **The Windows VM's check of the installer (2026-11-26).** The installer
  of #432's head `625a8f04` (CI's artifact, 19,122,688 bytes), in
  `frostmail-win11`, in Brian's signed-in session, on his real install:
  the Tauri build of 2026-10-16 (version 0.0.0, in
  `%LOCALAPPDATA%\Frostmail`, its mail store of 6,326 files, about 2 GB,
  and 3 accounts in its `Data`). What was seen, in the registry, the
  folders, the process list, `mailctl` and pictures of the screen:
  - **Over the Tauri install,** a silent install in the desktop session
    put the client in `%LOCALAPPDATA%\Programs\FrostMail`, took away the
    Tauri files and uninstall entry (the `Data` and `Cache` folders
    stayed), and pointed the Run key, the Start menu's and the desktop's
    shortcuts and the `frostmail-toast` handler at the new folder. The
    maild it started (`b276073`) listed the 3 accounts.
  - **The client** opened on that mail: All Inboxes 15267 unread, 99+ on
    its taskbar icon, Local. A message's HTML page drew, with "9 remote
    images are not shown" and the mailing list's banner.
  - **A second start** left 1 FrostMail process within 2 seconds.
    `FrostMail.exe --open-message ID`, the notifier's `--activate` and the
    shell's open of a `frostmail-toast:` URI each brought that message
    into the open window.
  - **The toast** showed under FrostMail's name and icon. 2 clicks of mine
    (`mouse_event` from a script) closed it and started nothing. A third
    toast Brian clicked himself, from the notification centre: "it opened
    the window", on the message the toast named (Brian, 2026-11-26).
  - **A silent uninstall** exited 0 in 2 seconds: the folder, the uninstall
    entry, the Run key, the shortcuts, `frostmail-toast` and the
    AppUserModelId's key were gone, maild was stopped, and the 6,326 files
    of the mail were kept.
  - **Over MyGo's install:** main's MyGo installer (`e910f05f`, CI's
    artifact) installed into the same folder; its client opened on the
    same mail. With it open, the NSGo installer exited 0 in 3 seconds:
    the MyGo client and its maild stopped, the files were replaced,
    `frostmail-nsgo.exe` was deleted, and a new maild started; the NSGo
    client opened on the same mail.
  - **Seen and fixed:** after that upgrade, the uninstall left MyGo's
    `.mygo-files`, and the folder with it. The installer deletes it now,
    and CI's install check puts both of MyGo's files in the folder first
    and requires the uninstall to take the folder away. The installer
    with the fix was not run on the VM; CI's check runs it.
  - The VM keeps the NSGo client installed, with maild from the Run key.
  - **Not done:** the release.
- **Done when:** `go.mod` names neither `github.com/egoist/mygo` nor
  frostui's root module, `make ci` passes, and the owner releases the
  version (`make bump`, then Promote).

## Phase 3.5 — FrostMail in Lodge (M)

- The NSGo web build takes the React app's place in Lodge, after MyGo
  is gone.
- **What the React app did that the NSGo web build does not (2026-11-26).**
  Read side by side in the code, the React app's (its paths below are
  under `app/src/`, as on main at `29e5865f`) and the NSGo client's
  (`cmd/frostmail-nsgo`), against
  [lodge-app.md's web build](../specs/lodge-app.md#the-web-build) as it was.
  None of these was built here; the first 3 are the ones a person would
  miss most.
  1. **A calendar reminder does not notify a hidden page.** The React app
     showed each new due reminder as a browser notification while the page
     was hidden, and its click opened the reminders (`app/announce.ts`,
     `remind`). The NSGo client shows a notice in its own window, "View
     reminders" (`commands.go`, `reminderNotice`), which a person in
     another tab does not see.
  2. **A notification's click does not bring Mail forward in Lodge.** The
     React app called `lodge.open("/mail" + route)` on a click
     (`app/announce.ts`, `show`). The NSGo client's click focuses the page
     (NSGo's `showWebNotification`) and opens the message in the mail
     frame (`commands.go`, `OnNotificationPress`); its `OrderFront` has no
     web implementation in NSGo v0.3.0, and nothing calls `lodge.open`.
     With another Lodge app in front, the message opens in the frame
     behind it. Not tried in a browser.
  3. **The reader frame's own sandbox is wider.** The React app's iframe
     set `sandbox="allow-popups allow-popups-to-escape-sandbox"` and
     `referrerpolicy="no-referrer"` (`features/reader/ContentFrame.tsx`).
     NSGo's web view sets `allow-forms allow-popups
     allow-popups-to-escape-sandbox allow-modals allow-downloads
     allow-same-origin` for a page of another origin and no
     `referrerpolicy` (NSGo's `internal/platform/web/webview.go`,
     `loadFrame`). Seen in headless Firefox on this change's `.deb`: the
     frame's attribute was exactly that list, `referrerpolicy` was absent,
     and inside the frame `window.origin` was `"null"` with 0 scripts,
     so the page's own policy (maild's and lodge-content's sandbox) held
     it to an opaque origin. No form, modal or download was tried in it.
  4. **Links in mail's own UI** (an unsubscribe page, a sign-in page) open
     with NSGo's `OpenURL`, a new tab, not `lodge.open` (`reader.go`,
     `accounts.go`): a canonical Lodge URL opens in a tab, not in the shell
     in place. An unsubscribe page opens after `message.unsubscribe`
     answers, not in the click; whether a browser blocks that tab was not
     tried.
  5. **Files dropped on Compose are not attached:** Attach's picker is the
     only way (`compose.go`; the React app took drops,
     `app/ComposeWindow.tsx`).
  6. **`/compose/<id>?fresh` is gone:** the route the client reports has no
     `fresh`, so a new draft reopened by its route (a reload, Back) is kept
     when it closes untouched (`compose.go`, `presentDraft`; `routes.go`).
  7. **Raw Source has no Save As** of its own for a source longer than the
     sheet shows (`reader.go`, `sourcePanel`); the message's Save As
     downloads the whole.
  8. **The socket stays open while the page is hidden,** and pings every
     30 s (`internal/desktop/conn`); the React app opened it and pinged
     only while the page was visible.
  9. **maild does not count the client as the app:** it says `rpc.hello`
     as `FrostMail NSGo` (`main.go`), without the `frostmail-app` prefix
     that `internal/rpcserver` counts (`AppClient`). Behind Lodge this
     changes nothing seen today (no desktop notifier there); on the
     desktop it means maild's own reminder notifications are not held
     back while the client is open. Read in the code only.
  10. **Saved layout:** the client keeps its state under NSGo's
      `nsgo:org.frostyard.Frostmail:` keys; the React app's `mail:` keys
      (pane widths, the sidebar, conversations) are not read.
  11. **Tests:** the React app's tests went with it. 62 rows of
      [parity.md](../specs/parity.md) name `.test.ts(x)` files that no
      longer exist; the NSGo suite has no web run
      ([design/testing.md](../design/testing.md#the-nsgo-clients-live-suite)),
      so the browser path is the smoke test's 6 steps.
  The client's other differences from the MyGo client
  ([What is true today](#what-is-true-today)) hold in Lodge too: no
  Keyboard Shortcuts dialog, print of the text, 1 snooze.
  The same, read: what the React app did that the client also does. The
  routes of the spec, the Message-ID permalink, the narrow window's Back,
  the shell's context, the theme, attachments and Save As by download,
  print, and new mail's browser notifications.
- **The switch (2026-11-26), built; no release holds it.** `make
  web-build` builds the NSGo client for `js/wasm` with NSGo's tool and
  the loader in `app/` (`index.html`, `src/nsgo-lodge.ts`) with Vite, into
  `build/web`, which `make deb` installs; `make nsgo-lodge-app` stages the
  same output for the smoke test and CI's NSGo job, so no workflow
  changes ([design/app.md](../design/app.md)). The React app, its tests
  and its dependencies are gone from `app/`, which keeps the loader,
  `src/permalink.ts` and its test, on `@frostyard/lodge-client`; `rpcgen`
  writes no TypeScript client. `packaging/deb-check.py` gains `--glob`,
  `--absent` and `--no-text`: `make deb-check` requires `index.html`,
  `nsgo.json`, `icon.svg`, the loader's `index-*.js`, the client's
  `app.*.wasm`, `nsgo.*.js` and `wasm_exec.*.js`, and refuses a
  stylesheet, a `.woff2` font or a file holding `__reactFiber` or
  `react.transitional.element`. `dev/lodge/nsgo-smoke.py --deb DEB` runs
  on what the `.deb` installs (dpkg-deb extracts it into the disposable
  Lodge root; the package's own maild and web build run).
  - The new deb-check, on the React app's `.deb` built from main at
    `29e5865f`, failed with 14 errors: no `nsgo.json`, nothing for 3 of
    the 4 patterns, 1 stylesheet, 7 fonts, and `__reactFiber` and
    `react.transitional.element` in its `index-*.js`.
  - On this change's commit `7b1de05b`, each exiting 0: `make check` (69
    packages ok, 0 failed, the linter's "0 issues" twice), `make
    nsgo-test`, `make ui-check IN_NSL='sh -c'` (Biome on 5 files, 2 tests
    passed in 1 file), `make web-build IN_NSL='sh -c'`, and `make deb`:
    `frostyard-maild_0.0.0-7b1de05b-snapshot_amd64.deb`, 23,131,514
    bytes, "is ok", whose mail app is `app.json` and 8 files of the web
    build (the Wasm 37,458,878 bytes).
  - `python3 dev/lodge/nsgo-smoke.py TOOLS --deb` on that `.deb`, run 2
    times: each listed `mail` at version `0.0.0-7b1de05b-snapshot`, up,
    printed "PASS: inbox, isolated reader, settings, Back, refresh and
    editable draft permalink", and exited 0, in 5 seconds each.
  - Without `--deb`, on `make nsgo-lodge-app IN_NSL='sh -c'`'s staged
    directory (its `app.json` at `0.0.0-dev`), 1 run: the same "PASS"
    line, exit 0.
  - Not run: `make nsgo-live` (the client's look did not change), any
    browser but headless Firefox, a real Lodge host, and CI.
- **Done when:** the `.deb` holds the NSGo web build and no React app,
  and `dev/lodge/nsgo-smoke.py` passes on what it installs. Both were seen
  on this change's snapshot `.deb` (above); no release holds it.

## Phase 4 — PocketGo (L)

- **Left this plan on 2026-11-26.** The owner retired PocketGo ("PocketGo
  is retired"). This phase is not done and will not be: no more of it is
  built, and PocketGo cuts no release. What frostyard/pocketgo holds
  stays there as it is. The items below are the phase as it was planned.
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

- **Left this plan on 2026-11-25.** The owner decided that Frostnotes
  needs no desktop window: Obsidian edits a desktop's vault folder,
  notesd in the Flatpak syncs it with the hub, and Lodge serves the React
  app again. Both of its desktop clients, MyGo and NSGo, are removed
  ([frostyard/frostnotes's ADR-0050](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0050-frostnotes-has-no-desktop-window.md)
  and its [plan 0011](https://github.com/frostyard/frostnotes/blob/main/docs/plans/0011-nsgo-only-client.md)).
- Before that, on 2026-11-25, its ADR-0049 made the NSGo client the only
  client, the owner kept everything outside the 4 groups too, and 4
  changes were merged (frostyard/frostnotes#288 to #291: the editor's
  plain logic out of MyGo's packages, source mode on NSGo's code editor).
  They go with the NSGo client.

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

On 2026-11-25 he replaced answer 1: Frostnotes has no desktop window, and
Lodge serves its React app ([Phase 5](#phase-5--frostnotes-xl)).

## Open questions

- None. The one about NSGo's iOS and macOS targets for PocketGo went
  with PocketGo (2026-11-26).

## References

- Follows: [plan 0016](0016-a-shared-look-for-the-nsgo-apps.md),
  [plan 0015](0015-mygo-desktop.md), [plan 0013](0013-m6-the-standalone-desktop-flatpak.md)
- Decided by: [ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md)
- Decisions it changes: [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
  [ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)
