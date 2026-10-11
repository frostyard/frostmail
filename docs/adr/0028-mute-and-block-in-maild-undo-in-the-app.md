# 0028 — Mute and block are maild's; Undo is the app's

- **Status:** Accepted
- **Date:** 2026-10-11

## Context

Three of Mail.app's features are left in parity's Phase 6:

- **Mute** (P-410): a muted conversation's new messages never notify.
  Mail.app keeps mutes on the Mac and shows a crossed-out bell on the
  conversation.
- **Block** (P-411): mail from a blocked sender is either marked as
  blocked and left in the inbox, with a banner in the message, or moved to
  the Trash (Settings → Junk Mail → Blocked). Mail.app shares its blocked
  list with the system's other apps.
- **Undo** (P-406): Edit → Undo (⌘Z) takes back the last move, delete or
  flag change.

maild owns what happens when mail arrives (sync, rules ([ADR-0024](0024-rules-run-in-maild-on-new-mail.md)),
notifications) and keeps settings ([ADR-0026](0026-preferences-live-in-maild.md));
the app owns what the user just did in a window. A message's ID stays
the same across moves, so a move can be moved back. There is no
standard for either mutes or blocked senders that the mail servers
Frostmail uses keep: the `$Muted` keyword exists for JMAP and Fastmail's
IMAP, but neither Dovecot nor Gmail's IMAP does anything with it, and
Gmail's own mute is not visible over IMAP.

## Decision

- **Mute is a mark on a conversation, in maild.** `threads.muted`; a
  conversation merged with a muted one is muted. `message.mute {ids,
  muted}` mutes or unmutes the conversations of messages, and summaries
  say `muted`. A muted conversation's new mail never notifies, not even
  by a rule's Send Notification. It stays on this computer.
- **Blocked senders are a list in maild.** `blocked (address, at)`, with
  `block.list`, `block.add`, `block.remove` and `block.changed`, and the
  setting `blockAction`: `mark` (the default: leave the mail where it
  arrives, with a banner in the reader) or `trash` (move new mail from a
  blocked sender out of the inbox to the Trash as it arrives, before
  rules). Mail from a blocked sender never notifies. Mail that arrived
  before the block stays where it is. The list stays on this computer.
- **Undo is the app's.** The main window remembers the inverse of each
  move, delete, archive, junk, flag and read change it makes (where each
  message was, what flags each had), and Ctrl+Z sends the newest inverse
  to maild as ordinary operations. The window keeps the last 20; they are
  gone when it closes. A permanent delete (in the Trash) and what rules
  or other clients do are not undone.

## Consequences

- Mute and block work the same on every account, offline included, and
  maild applies them while the app is closed.
- Neither follows the user to another computer or client. When a server
  standard for them is used by the servers Frostmail supports, maild can
  sync it without the app changing.
- Undo needs nothing new in maild: an inverse is a move or a flag change
  maild already queues, replays and syncs. Undo after the app restarts is
  not possible, as in Mail.app.

## Alternatives considered

- **The `$Muted` keyword on messages.** Synced where a server keeps
  keywords, but maild would have to set it on each new message of a muted
  conversation, and no other client of Dovecot or Gmail honours it.
  Rejected for now.
- **Block by a rule.** Rules can already move mail by sender, but a
  rule's list is hard to manage and cannot mark mail for a banner.
  Rejected.
- **Undo in maild.** maild would need to know which window did what;
  the app already knows. Rejected.
