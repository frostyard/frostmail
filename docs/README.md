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

### Design

- [Overview](design/overview.md): the entry point
- [Storage](design/storage.md)
- [Sync engine](design/sync.md)
- [Testing](design/testing.md)
- [Agent workflow](design/agent-workflow.md)

### Specs

- [RPC protocol](specs/rpc-protocol.md): framing, connection setup, events
- [RPC API](specs/rpc-api.md): every method, type and event (generated)

### Plans

- [0001 — M0 foundations](plans/0001-m0-foundations.md), with its evidence
- [0002 — Roadmap to Mail.app parity](plans/0002-roadmap.md)
- [0003 — M1 headless read path](plans/0003-m1-headless-read-path.md), done, with its evidence

## Conventions

- New docs start from their category's `TEMPLATE.md`.
- A new decision gets a new ADR with the next number; reversing one marks the
  old ADR `Superseded by NNNN` instead of editing it.
- Adding a doc means indexing it here and linking it from related docs.
