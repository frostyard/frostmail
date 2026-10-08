# Plan 0005: M3 compose and send

M3 adds writing mail: compose windows with a rich-text editor, replies and
forwards with quoting, attachments, drafts saved locally and to the server,
an outbox with undo send that keeps working offline and never sends twice.
It follows [M2](0004-m2-ui-read-path.md) and comes before the daily-driver
milestone (M4) in the [roadmap](0002-roadmap.md). Work is split per
[ADR-0008](../adr/0008-local-executor-workflow.md); the lessons from M2's
executor record apply: lint the markup a contract prescribes, state the
framework's own behavior, assert every relationship the contract names.

## Decisions taken for M3

- [ADR-0010](../adr/0010-compose-and-send.md): maild owns drafts and the
  outbox; it builds every message; sending goes through an outbox with a
  10-second undo delay and crash recovery by state and by Message-ID in Sent;
  attachments travel as file paths; one Tauri window per draft; TipTap.
- [design/send.md](../design/send.md) describes the draft saver, reply and
  forward rules, the builder, the outbox worker and address suggestions.
- **API, protocol 1, additive:** `draft.*`, `outbox.*`, `address.suggest`,
  `identity.list` and `identity.update`; events `draft.changed` and
  `outbox.changed`.

## Phase 1 — Contracts and foundations (planner)

- ADR-0010, design/send.md, this plan.
- Schema, `make gen`, engine and store stubs; migration 0003 (`drafts`,
  `addresses`, a default identity per account).
- The test server allows 50 MB messages (`mailtest.sh`, snapshots).
- App: TipTap dependencies, `#/compose/<id>` routing, the bridge keeping one
  maild connection per window, the compose window container skeleton.
- Cards with stubs and given tests.
- **Done when:** `make check` and `make ui-check` are green and the cards
  are ready.

## Phase 2 — Executor batch (runs alongside Phase 3)

| Card | Area | What |
| --- | --- | --- |
| T-0037 | `internal/store` | Drafts: create, get, list, update, delete, attachments |
| T-0038 | `internal/store` | Addresses: record seen addresses, suggest by prefix |
| T-0039 | `internal/store` | Outbox rows: insert, due, transitions, cancel, list |
| T-0040 | `internal/compose` | Reply and forward rules: recipients, subjects, attribution, quoting |
| T-0041 | `cmd/mailctl` | `mailctl send` and `mailctl outbox` |
| T-0042 | `app/src/lib` | `addressParse`: typed text to addresses |
| T-0043 | `features/compose` | `RecipientField`: tokens and suggestions |
| T-0044 | `features/compose` | `ComposeHeader`: identity, To, Cc, Bcc, Subject |
| T-0045 | `features/compose` | `EditorToolbar` |
| T-0046 | `features/compose` | `ComposeAttachments` |
| T-0047 | `features/outbox` | `UndoToast` and `OutboxStatus` |

- **Done when:** every card is merged with `make check` and `make ui-check`
  green.

## Phase 3 — Building, sending and the compose window (planner)

- `internal/compose`: the message builder (headers with injection checks,
  alternative and mixed parts, attachments), golden tests.
- `internal/smtpx` over go-smtp: TLS and STARTTLS, AUTH PLAIN, SIZE.
- The outbox worker and the draft saver in the account actor; crash
  recovery; engine domains `draft`, `outbox`, `address`, `identity`.
- The compose window: TipTap editor, autosave, send with undo, attachments by
  file dialog and drop (tauri-plugin-dialog), reply/reply all/forward from
  the toolbar, the context menu and the keyboard (Ctrl+N, Ctrl+R,
  Ctrl+Shift+R, Ctrl+Shift+F), opening messages in Drafts.
- **Done when:** a message composed in the app (in the browser through the
  dev gateway) reaches another test account through Postfix and Dovecot.

## Phase 4 — Tests

- Go integration tests against the mail server: sent mail lands in the
  recipient's INBOX and in the sender's Sent; the outbox flushes after
  Postfix comes back; a 25 MB attachment arrives intact; drafts survive a
  maild restart and appear in the Drafts mailbox; a crash after SMTP
  acceptance does not send twice (fault hook in a test build).
- App test: compose, attach, send and undo through the built app.
- **Done when:** `make engine-it` and `make ui-e2e` pass.

## Phase 5 — Exit evidence

- **Done when**, each shown by a test or a recorded run:
  1. Mail composed in the app and sent through Postfix lands in Dovecot,
     with a copy in Sent.
  2. With Postfix stopped, sent mail waits in the outbox and goes out within
     a minute of Postfix returning.
  3. A 25 MB attachment arrives byte for byte.
  4. Drafts survive a maild restart and match the server's Drafts copy.
  5. Undo within 10 seconds cancels a send; a crash after acceptance does
     not send twice.

## Later / ideas

- Send later and scheduled sending (`scheduled` state): M5.
- Inline images in the editor (`multipart/related`).
- XOAUTH2 for Gmail and Microsoft: M4.
- A settings window for identities and signatures: M4.

## References

- Implements: [design/send.md](../design/send.md),
  [ADR-0010](../adr/0010-compose-and-send.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
