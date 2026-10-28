# Documentation

Docs are split by the question they answer (frostyard/core ADR-0025):

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Architecture Decision Records; immutable once accepted |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions |
| [plans/](plans/) | **When/in what order** do we build? | Milestone plans with "done when" outcomes |
| [tasks/](tasks/) | **What does the executor do next?** | Task cards for the local executor model ([agent workflow](design/agent-workflow.md)) |

Org-wide decisions that bind this repository: [org-adrs.md](org-adrs.md).

## Index

### Decisions (ADRs)

- [0001 — Record architecture decisions](adr/0001-record-architecture-decisions.md)
- [0002 — A headless daemon with thin clients](adr/0002-daemon-and-thin-clients.md)
- [0003 — SQLite store with a single writer and a durable event log](adr/0003-sqlite-store.md)
- [0004 — Fork go-imap for Gmail extensions](adr/0004-go-imap-fork.md)
- [0005 — Render HTML mail in a sandboxed iframe under a strict CSP](adr/0005-html-mail-rendering.md)
- [0006 — Development environment: host engine, nsl app toolchain, incus mail server](adr/0006-development-environment.md)
- [0007 — A YAML IDL generates the RPC contract](adr/0007-rpc-contract.md)
- [0008 — Claude plans and verifies; a local model executes task cards](adr/0008-local-executor-workflow.md)
- [0009 — Build the app as a thin client over live views](adr/0009-ui-architecture.md)
- [0010 — maild owns drafts and the outbox; the app edits them](adr/0010-compose-and-send.md)
- [0011 — maild signs in with the user's own OAuth client and keeps secrets in the Secret Service](adr/0011-sign-in-and-credentials.md)
- [0012 — Gmail is synced as one store with labels as memberships](adr/0012-gmail-labels-as-memberships.md)
- [0013 — The app installs as a local Flatpak; maild stays on the host](adr/0013-app-as-a-local-flatpak.md)
- [0014 — Images reach the reader's frame as data: URLs](adr/0014-images-embedded-in-the-reader-frame.md)
- [0015 — Use a Frostyard mail icon](adr/0015-use-a-frostyard-mail-icon.md)
- [0016 — Sync a window of recent mail](adr/0016-sync-a-window-of-recent-mail.md)
- [0017 — Sync contacts, calendars and tasks with CardDAV, CalDAV and Google Tasks](adr/0017-sync-contacts-calendars-and-tasks.md)
- [0018 — Keep each contact, event and task as the server sent it](adr/0018-keep-contacts-events-and-tasks-as-sent.md)
- [0019 — Answer invitations through the server's scheduling, else by mail](adr/0019-answer-invitations-through-the-server.md)
- [0020 — One window for mail, calendar, people and tasks](adr/0020-one-window-for-mail-calendar-people-and-tasks.md)
- [0021 — Run task cards with Codex](adr/0021-run-task-cards-with-codex.md)
- [0022 — Mail the reply the server will not send](adr/0022-mail-the-reply-the-server-will-not-send.md)
- [0023 — One condition language for smart mailboxes and rules](adr/0023-one-condition-language-for-smart-mailboxes-and-rules.md)
- [0024 — Rules run in maild on new mail](adr/0024-rules-run-in-maild-on-new-mail.md)
- [0025 — maild keeps Send Later, Remind Me and Undo Send](adr/0025-maild-keeps-send-later-remind-me-and-undo-send.md)
- [0026 — Preferences live in maild](adr/0026-preferences-live-in-maild.md)
- [0027 — Unsubscribe with one click through the sender's server](adr/0027-unsubscribe-with-one-click.md)
- [0028 — Mute and block are maild's; Undo is the app's](adr/0028-mute-and-block-in-maild-undo-in-the-app.md)
- [0029 — Show no mail categories](adr/0029-show-no-mail-categories.md)
- [0030 — Edit contacts and events as patches, and re-apply a refused change](adr/0030-edit-contacts-and-events-as-patches.md)
- [0031 — Edit a recurring event in three scopes](adr/0031-edit-a-recurring-event-in-three-scopes.md)
- [0032 — Undo restores what a change replaced](adr/0032-undo-restores-what-a-change-replaced.md)
- [0033 — Write new events with their zone's VTIMEZONE](adr/0033-write-new-events-with-their-zones-vtimezone.md)
- [0034 — Invite from the organizer's copy, and ask before telling invitees](adr/0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md)
- [0035 — Invite through the provider's scheduling, and confirm before it sends](adr/0035-invite-through-the-providers-scheduling.md)
- [0036 — Links in the reader's frame open as new-window requests](adr/0036-reader-links-open-as-new-window-requests.md)
- [0037 — Fetch what the reader shows, and prefetch it](adr/0037-fetch-what-the-reader-shows.md)
- [0038 — Windows is a second platform, with maild and the app as on Linux](adr/0038-windows-as-a-second-platform.md)
- [0039 — Microsoft accounts: mail over IMAP and SMTP, contacts and calendars over Graph](adr/0039-microsoft-accounts-over-imap-smtp-and-graph.md)
- [0040 — Store Graph's events and contacts as iCalendar and vCard derived from its JSON](adr/0040-graph-events-and-contacts-as-derived-sources.md)
- [0041 — maild runs as a lodge app](adr/0041-maild-runs-as-a-lodge-app.md)
- [0042 — Agents get calendar, task and contact tools](adr/0042-agents-get-calendar-task-and-contact-tools.md)
- [0043 — Finish an OAuth sign-in with a pasted address when the browser cannot reach maild](adr/0043-finish-a-sign-in-with-a-pasted-address.md)
- [0044 — On a server, maild seals its secrets file with a key from a systemd credential](adr/0044-seal-the-servers-secrets-with-a-systemd-credential.md)
- [0045 — The desktop app builds as a Flatpak in CI](adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)
- [0046 — The desktop Flatpak carries its own maild; lodge is a connection the user chooses](adr/0046-the-desktop-flatpak-carries-its-own-maild.md)

### Design

- [Overview](design/overview.md): the entry point
- [Storage](design/storage.md)
- [Sync engine](design/sync.md)
- [Message rendering](design/rendering.md)
- [The app](design/app.md)
- [Drafts and sending](design/send.md)
- [Accounts, sign-in and providers](design/accounts.md)
- [Running on the desktop](design/desktop.md)
- [Contacts, calendars and tasks](design/pim.md)
- [Organizing mail](design/organize.md): conditions, smart mailboxes, rules, VIPs, flags, Remind Me (M5)
- [Testing](design/testing.md)
- [Agent workflow](design/agent-workflow.md)

### Specs

- [RPC protocol](specs/rpc-protocol.md): framing, connection setup, events
- [RPC API](specs/rpc-api.md): every method, type and event (generated)
- [Reader UI](specs/ui.md): tokens, layout, behavior and keyboard map
- [Compose UI](specs/compose-ui.md): the compose window, undo toast and outbox
- [Search language](specs/search.md): what the search field accepts
- [Settings UI](specs/settings-ui.md): accounts, signatures and the Google client
- [People, calendar and tasks UI](specs/pim-ui.md): the module bar, the People module, the contact card (M4.5)
- [Mail.app parity](specs/parity.md): every Mail feature, its status and its test (M5)
- [Condition editor and smart mailboxes](specs/organize-ui.md): the editor rules share, the smart mailbox sheet (M5)
- [maild as a lodge app](specs/lodge-app.md): what maild installs for lodge and serves on its app socket, the web build (M8), and agents' calendar, task and contact tools (M8.5)

### Plans

- [0001 — M0 foundations](plans/0001-m0-foundations.md), with its evidence
- [0002 — Roadmap to Mail.app parity](plans/0002-roadmap.md)
- [0003 — M1 headless read path](plans/0003-m1-headless-read-path.md), done, with its evidence
- [0004 — M2 UI read path](plans/0004-m2-ui-read-path.md), done, with its evidence
- [0005 — M3 compose and send](plans/0005-m3-compose-and-send.md), done, with its evidence
- [0006 — M4 daily driver](plans/0006-m4-daily-driver.md), done, with its evidence
- [0007 — M4.5 people, calendar and tasks](plans/0007-m4.5-people-calendar-tasks.md), done, with its evidence
- [0008 — M5 Mail.app parity](plans/0008-m5-mail-app-parity.md), done, with its evidence
- [0009 — M5.5 edit events and contacts](plans/0009-m5.5-edit-events-and-contacts.md)
- [0010 — M7 Microsoft accounts and Windows](plans/0010-m7-microsoft-and-windows.md)
- [0011 — M8 Mail in lodge](plans/0011-m8-mail-in-lodge.md)
- [0012 — M8.5 Calendar, tasks and contacts for agents](plans/0012-calendar-tasks-and-contacts-for-agents.md)
- [0013 — M6 The standalone desktop Flatpak](plans/0013-m6-the-standalone-desktop-flatpak.md)

## Conventions

- New docs start from their category's `TEMPLATE.md`.
- A new decision gets a new ADR with the next number; reversing one marks the
  old ADR `Superseded by NNNN` instead of editing it.
- Adding a doc means indexing it here and linking it from related docs.
