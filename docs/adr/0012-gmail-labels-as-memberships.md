# 0012 — Gmail is synced as one store with labels as memberships

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

Gmail's IMAP shows each label as a folder: one message appears in INBOX,
`[Gmail]/All Mail` and every label folder, with a different UID in each.
Syncing those folders like any other server fetches every message several
times and gives deletes the wrong meaning: expunging a message from a label
folder only removes that label. Gmail identifies a message across folders
with `X-GM-MSGID`, its conversation with `X-GM-THRID`, and lists its labels
in `X-GM-LABELS`; all three are in the forked go-imap
([ADR-0004](0004-go-imap-fork.md)), as are `STORE ±X-GM-LABELS` and
`X-GM-RAW` searches. The schema already has `messages.gm_msgid` (unique per
account), `threads.gm_thrid`, `mailboxes.is_gmail_label`, and
`message_mailbox` rows without a UID for label memberships. Gmail offers
CONDSTORE but not QRESYNC; label changes raise a message's MODSEQ. The user
reads a personal Gmail account daily in M4.

## Decision

- **Three folders are synced:** the mailboxes with special-use `\All`
  (`[Gmail]/All Mail`), `\Junk` (Spam) and `\Trash`. Their messages are
  stored once per account, keyed by `X-GM-MSGID`, with UID memberships in
  the folder that holds them. A message moved to Trash or Spam leaves All
  Mail and keeps its row.
- **Every other mailbox is a label** (`is_gmail_label`): INBOX, Sent,
  Drafts, Starred, Important and the user's labels. Labels are never
  selected for sync; their memberships (without UIDs) come from the
  `X-GM-LABELS` fetched with each All Mail message, mapping `\Inbox`,
  `\Sent`, `\Draft`, `\Starred` and `\Important` to those mailboxes and
  other names to user labels. Views and counts of a label read its
  memberships.
- **Changes come from All Mail:** headers, flags and labels are fetched
  together, and incremental passes use CONDSTORE (`CHANGEDSINCE`), so a
  label edited in Gmail's web UI arrives with the next pass. IDLE watches
  All Mail for new mail; Gmail's IDLE does not report label or flag changes
  made elsewhere (observed 2026-10-08), so All Mail, Spam and Trash are
  polled with STATUS every minute and any change runs a pass.
- **Operations act on All Mail UIDs** (or the Spam/Trash UID):
  - flags: `UID STORE` on the message's folder; Starred follows `\Flagged`;
  - archive: `-X-GM-LABELS \Inbox`;
  - move to a label: `+X-GM-LABELS` the target and, when moving out of a
    label (INBOX included), `-X-GM-LABELS` the source;
  - delete: `UID MOVE` to Trash; delete in Trash: expunge (permanent);
  - junk / not junk: `UID MOVE` to Spam / back to All Mail with `\Inbox`.
- **Threads come from `X-GM-THRID`**, not JWZ threading.
- **Sent mail is not appended** (Gmail files it, as in M3); draft copies
  are appended to the Drafts label folder, and a replaced copy is moved
  from it to Trash by UID and expunged there: the one place a label folder
  is selected.

## Consequences

- Each message is fetched once, however many labels it has; labels cost one
  membership row each.
- The sync engine and offline operations gain a Gmail path selected by the
  account kind; the generic path stays as it is.
- Gmail's own IMAP settings (auto-expunge, what happens to expunged
  messages) do not matter: Frostmail moves to Trash explicitly and only
  expunges from Trash.
- Replay tests recorded from a throwaway Gmail account guard the Gmail path,
  since no local server speaks Gmail's extensions.

## Alternatives considered

- **Every folder synced, deduplicated by `X-GM-MSGID`:** less new code, but
  each message is fetched once per label and every operation needs the UID
  of the folder it happens in, with Gmail's per-folder delete semantics.
- **The Gmail API instead of IMAP:** another protocol and quota system for
  one provider, and the same OAuth verification; IMAP keeps one engine.
