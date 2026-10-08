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

### Design

- [Overview](design/overview.md): the entry point
- [Storage](design/storage.md)
- [Sync engine](design/sync.md)
- [Message rendering](design/rendering.md)
- [The app](design/app.md)
- [Drafts and sending](design/send.md)
- [Accounts, sign-in and providers](design/accounts.md)
- [Running on the desktop](design/desktop.md)
- [Testing](design/testing.md)
- [Agent workflow](design/agent-workflow.md)

### Specs

- [RPC protocol](specs/rpc-protocol.md): framing, connection setup, events
- [RPC API](specs/rpc-api.md): every method, type and event (generated)
- [Reader UI](specs/ui.md): tokens, layout, behavior and keyboard map
- [Compose UI](specs/compose-ui.md): the compose window, undo toast and outbox
- [Search language](specs/search.md): what the search field accepts
- [Settings UI](specs/settings-ui.md): accounts, signatures and the Google client

### Plans

- [0001 — M0 foundations](plans/0001-m0-foundations.md), with its evidence
- [0002 — Roadmap to Mail.app parity](plans/0002-roadmap.md)
- [0003 — M1 headless read path](plans/0003-m1-headless-read-path.md), done, with its evidence
- [0004 — M2 UI read path](plans/0004-m2-ui-read-path.md), done, with its evidence
- [0005 — M3 compose and send](plans/0005-m3-compose-and-send.md), done, with its evidence
- [0006 — M4 daily driver](plans/0006-m4-daily-driver.md)

## Conventions

- New docs start from their category's `TEMPLATE.md`.
- A new decision gets a new ADR with the next number; reversing one marks the
  old ADR `Superseded by NNNN` instead of editing it.
- Adding a doc means indexing it here and linking it from related docs.
