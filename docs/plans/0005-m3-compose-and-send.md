# Plan 0005: M3 compose and send

Status: **done** (2026-10-07), pending the native checks of the file
dialog and drag and drop on the user's desktop (WebDriver cannot drive
them).

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
| T-0045 | `features/compose` | `ComposeToolbar` and `FormatBar` |
| T-0046 | `features/compose` | `ComposeAttachments` |
| T-0047 | `features/outbox` | `UndoToast` and `OutboxStatus` |

- **Done when:** every card is merged with `make check` and `make ui-check`
  green.

## Phase 3 — Building, sending and the compose window (planner)

- `internal/compose`: the message builder (headers with injection checks;
  alternative, related and mixed parts; inline images and attachments),
  with round-trip tests.
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

Evidence, recorded 2026-10-07:

| # | How | Result |
| --- | --- | --- |
| 1 | The UI in the browser pane through `maild -devgw` on the Dovecot account test3: New Message, a recipient token, subject, a body with bold, Send. `make engine-it`: `TestPostfixDeliversAndSentIsFiled` (test4 to test5). | The message waited out the 10-second undo delay, went through Postfix (587, STARTTLS) and was in test2's INBOX with a copy in test3's Sent; the draft was gone. The run found two bugs (below). A forward of a message with two attachments carried both over. |
| 2 | Postfix stopped in the container (`systemctl stop postfix`), `mailctl send` through the dev maild, Postfix started again. `make engine-it`: `TestOutboxFlushesWhenTheServerReturns`. | While down the row was queued with attempt 1 and "connection refused"; it went out 32 s after Postfix returned and was delivered. The test passes with an unreachable server, then the real one. |
| 3 | `make engine-it`: `TestLargeAttachmentArrivesIntact` | 25,000,000 random bytes attached, sent through Postfix, fetched from Dovecot: the SHA-256 matches. |
| 4 | `make engine-it`: `TestDraftsSurviveARestart`; unit tests with the in-memory server (`TestDraftCopyIsSavedReplacedAndDeleted`, `TestOpenADraftFromTheDraftsMailbox`) | After a restart on the same data the draft is listed; editing it replaces its Dovecot copy (one copy, new subject). Copies keep Bcc, drop half-typed recipients, and are deleted with the draft. |
| 5 | `make ui-e2e`: `compose.e2e.ts` in the built app; unit tests `TestUndoCancelsASend`, `TestInterruptedSendIsSentOnce`, `TestInterruptedSendFoundInSentIsNotResent`, `TestCrashAfterAcceptanceSendsOnce` | In the built app a compose window opened from the main window, sent, Undo on the toast reopened the draft with nothing sent, and the second send delivered exactly one message. maild stopped after the server accepted a message (Sent unreachable) and restarted: one delivery, one Sent copy. |
| 6 | `make check` (222 Go tests), `make ui-check` (271 tests), `make ui-e2e` (5), `make engine-it`, `go test -race` five times over the send packages | All green; cards T-0037 to T-0047 merged. |

**Bugs found by using and testing it.** A new message started with a
hard break (TipTap read mail's `<p><br></p>` as a paragraph holding a
break), and blank lines left the editor as `<p></p>`, which mail clients
collapse; both directions are converted now. A browser that opened
`#/compose/<id>` in the same tab stayed on the main window. Draft edits in
the same millisecond as the last save were never copied (times have
millisecond resolution); `updated_at` now always moves forward. After a
restart, a Sent copy replayed before the first mailbox listing marked its
message sent without a copy; such ops now wait for the list. The smtpx
integration test delivered to the mailbox the Dovecot sync test counts.

**Executor record.** 11 cards (T-0037 to T-0047), every one passed through
taskrun on the first run, in 102 to 355 seconds. One defect slipped past a
given test (T-0043: suggestions arriving after a commit reopened the list);
the planner fixed it with a test. Cards carried a reference implementation
checked against the given test before handing over, plus Biome's verdict on
the prescribed markup, which removed the failure modes of M2.

## Later / ideas

- Send later and scheduled sending (`scheduled` state): M5.
- Pasting and inserting images in the editor (the builder already writes
  inline images as `multipart/related`).
- XOAUTH2 for Gmail and Microsoft: M4.
- A settings window for identities and signatures: M4.

## References

- Implements: [design/send.md](../design/send.md),
  [ADR-0010](../adr/0010-compose-and-send.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
