# Plan 0014: Small features from the nexmail review

Four small features to build now, and four ideas kept on the backlog.
Brian chose this split on 2026-11-05 (T929a, "4 small now, rest
backlog"), after Odrade's review of nexmail, a self-hosted mail client.

- **Ideas only.** nexmail is AGPL-3.0 and Frostmail is MIT. Nobody who
  builds these phases opens nexmail's code, and no code, test or text is
  copied or transcribed from it. The specs in this plan are Frostmail's
  own.
- **Each phase is one GitHub issue**, with its acceptance criteria and a
  targeted test per behavior. Its work is in task cards of size S
  ([ADR-0021](../adr/0021-run-task-cards-with-codex.md)), 1 to 3 per
  phase.
- **The phases do not depend on each other.** The suggested order is 2,
  1, 3, 4: the app-only reminder first, and last the one change that
  deletes mail on its own.
- **Gates for every phase:** `make check` for Go; `make ui-test` in the
  nsl machine for the app; `make engine-it` where a phase names an
  integration test.

Status (2026-11-07):
- **Phase 0 is done:** PR #167 merged on 2026-11-05; ADRs 0050 and 0051
  are Accepted.
- **Phase 2 is built** in [PR #168](https://github.com/frostyard/frostmail/pull/168)
  (issue [#160](https://github.com/frostyard/frostmail/issues/160)).
- **Phase 1 is built** in [PR #169](https://github.com/frostyard/frostmail/pull/169)
  (issue [#159](https://github.com/frostyard/frostmail/issues/159)).
- **Phase 3 is built** in the PR that closes
  [#161](https://github.com/frostyard/frostmail/issues/161).
- **Phase 4 waits** for a day shift.

## Phase 0 — The specs and the decisions (planner, small)

- [ADR-0050](../adr/0050-warn-when-the-providers-sender-checks-fail.md)
  (sender check) and
  [ADR-0051](../adr/0051-erase-old-trash-and-junk-when-the-user-asks.md)
  (erase old Trash and Junk), Accepted.
- The specs: [design/rendering.md](../design/rendering.md#sender-check),
  [specs/ui.md](../specs/ui.md#reader) (Sender check banner),
  [specs/compose-ui.md](../specs/compose-ui.md) (Attachment reminder,
  Snippets), [design/send.md](../design/send.md#snippets),
  [specs/settings-ui.md](../specs/settings-ui.md) (Snippets pane,
  `AccountForm`'s Mailbox Behaviors),
  [design/organize.md](../design/organize.md#erase-old-mail),
  [design/storage.md](../design/storage.md),
  [specs/parity.md](../specs/parity.md) (P-503, P-114).
- **Done when:** Brian merges the PR, which accepts ADRs 0050 and 0051.

## Phase 1 — Sender check banner (S; [#159](https://github.com/frostyard/frostmail/issues/159))

- **Status:** built (#159's PR), 2026-11-05.
- [ADR-0050](../adr/0050-warn-when-the-providers-sender-checks-fail.md);
  the rules in [design/rendering.md](../design/rendering.md#sender-check);
  the banner in [specs/ui.md](../specs/ui.md#reader).
- Card (a), maild: `internal/authres` (the parser and verdict);
  `Message.senderCheck` in `schema/rpc/message.yaml` (an optional enum
  `SenderCheck`: `pass`, `fail`), `make gen`; `message.get` fills it.
- Card (b), the app: the banner, with Show Details.
- **Done when:** a stored message whose header has `dmarc=fail` shows the
  banner in the reader, and one with `dmarc=pass` or no header shows
  none.

## Phase 2 — Attachment reminder (S; [#160](https://github.com/frostyard/frostmail/issues/160))

- [specs/compose-ui.md](../specs/compose-ui.md#attachment-reminder-plan-0014-phase-2).
- One card, the app only: `app/src/features/compose/attachmentReminder.ts`
  (`ownText`, `mentionsAttachment`), the dialog, and the check before
  `draft.send` in the compose window.
- **Done when:** "I attached the file" above a quote with nothing attached
  asks before sending, and "attached" only in the quote or the signature
  does not.

## Phase 3 — Snippets (S; [#161](https://github.com/frostyard/frostmail/issues/161))

- **Status:** built (#161's PR), 2026-11-07.
- [design/send.md](../design/send.md#snippets),
  [specs/settings-ui.md](../specs/settings-ui.md#snippets-pane),
  [specs/compose-ui.md](../specs/compose-ui.md#snippets-plan-0014-phase-3);
  a list preference as [ADR-0026](../adr/0026-preferences-live-in-maild.md)
  has them, so no new ADR.
- Card (a), maild: the migration, `schema/rpc/snippet.yaml`
  (`snippet.list`, `create`, `update`, `delete`, `snippet.changed`),
  `make gen`, the engine.
- Card (b), the app: the Snippets pane in Settings.
- Card (c), the app: Insert Snippet in compose.
- P-503 becomes Have, with the tests.
- **Done when:** a snippet made in Settings goes in at the cursor from
  compose's Insert Snippet menu, and is sent as part of the message.

## Phase 4 — Erase old Trash and Junk (S; [#162](https://github.com/frostyard/frostmail/issues/162))

- [ADR-0051](../adr/0051-erase-old-trash-and-junk-when-the-user-asks.md);
  [design/organize.md](../design/organize.md#erase-old-mail);
  [specs/settings-ui.md](../specs/settings-ui.md#accountform).
- Card (a), maild: the migration (`message_mailbox.added_at`,
  `accounts.erase_trash_days`, `erase_junk_days`), the `Account` fields
  and `account.update`, `added_at` set on every insert and on every move
  of a row to another mailbox,
  `mailctl account set --erase-trash-days --erase-junk-days`.
- Card (b), maild: `Manager.EraseOld`, its run from the reminder
  scheduler, and `expungeOp`'s `uidValidity` check.
- Card (c), the app: `AccountForm`'s Mailbox Behaviors.
- P-114 becomes Have, with the tests.
- **Done when:** on the test Dovecot, with Erase deleted messages set to
  After One Week, a message 8 days in Trash (by the test's clock) is gone
  from the server, and one 6 days in Trash and an old one in a user
  mailbox stay.

## Later / ideas

The backlog Brian chose, one issue each, with no spec work yet. Each
needs a plan (or a spec change) before it is built.

- Server search on demand, with a line that says how far the search
  looked (P-604): [#163](https://github.com/frostyard/frostmail/issues/163).
- Import mbox and `.eml` files (duplicates skipped by Message-ID), and
  export a mailbox as mbox (P-955): [#164](https://github.com/frostyard/frostmail/issues/164).
- A command palette, on a chord other than Ctrl+K, which is Link in
  compose: [#165](https://github.com/frostyard/frostmail/issues/165).
- Optional writing help from an AI endpoint the user brings: P-510 is
  Out, so this needs a new ADR, or goes to Porter: [#166](https://github.com/frostyard/frostmail/issues/166).

## Open questions

- None. ADRs 0050 and 0051 are Accepted (PR #167, 2026-11-05).

## References

- Implements: [design/rendering.md](../design/rendering.md),
  [design/send.md](../design/send.md),
  [design/organize.md](../design/organize.md),
  [specs/ui.md](../specs/ui.md), [specs/compose-ui.md](../specs/compose-ui.md),
  [specs/settings-ui.md](../specs/settings-ui.md),
  [specs/parity.md](../specs/parity.md)
- Decisions: [ADR-0050](../adr/0050-warn-when-the-providers-sender-checks-fail.md),
  [ADR-0051](../adr/0051-erase-old-trash-and-junk-when-the-user-asks.md),
  [ADR-0026](../adr/0026-preferences-live-in-maild.md)
