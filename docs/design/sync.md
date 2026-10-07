# Design: sync engine

Status: designed in M0, built in M1 (generic IMAP,
[plan 0003](../plans/0003-m1-headless-read-path.md)) and M4 (Gmail,
Microsoft, iCloud). Written by the planner, not by task cards
([ADR-0008](../adr/0008-local-executor-workflow.md)).

## Actors and connections

Code: `internal/mailsync` (`Manager`, `actor`, `reconcile`, `ops`) over
`internal/imapx.Session`.

- One actor goroutine per account owns that account's connections and
  schedules work by priority: **P0** what the user is viewing (a body, an
  attachment) > **P1** replaying `pending_ops` > **P2** INBOX > **P3**
  incremental sync of other folders > **P4** backfill and prefetch.
- Connections per account (Gmail allows 15): C1 runs commands, polling and
  body fetches; C2 IDLEs on INBOX, re-issuing IDLE every 25 minutes and
  treating 10 silent minutes as dead. M4 may add C3 for bulk fetches.
  Provider profiles can lower the count.
- Connect: CAPABILITY, ID; CONDSTORE needs no ENABLE because
  `SELECT … (CONDSTORE)` turns it on (RFC 7162 3.1);
  `LIST "" "*" RETURN (SPECIAL-USE SUBSCRIBED)`, with role fallback by name
  ("Sent Messages", "Deleted Messages" on iCloud).

## Initial sync

INBOX, then Sent, Drafts, then the rest. Fetch headers newest-first in UID
chunks of 500:
`UID FLAGS INTERNALDATE RFC822.SIZE ENVELOPE BODYSTRUCTURE MODSEQ
BODY.PEEK[HEADER.FIELDS (References In-Reply-To List-Id List-Unsubscribe
Authentication-Results)]`, plus `X-GM-MSGID X-GM-THRID X-GM-LABELS` on
Gmail. The preview comes from a partial fetch of the first text part chosen
from BODYSTRUCTURE: `BINARY.PEEK[part]<0.2048>` with BINARY, else
`BODY.PEEK[part]<0.2048>` decoded locally (`mimex.DecodePart`). Bodies are
fetched when opened (P0) and prefetched for INBOX and VIPs within 90 days
(P4). Headers are backfilled down to the configured window; search beyond it
falls back to server `UID SEARCH` (`X-GM-RAW` on Gmail).

## The reconcile pass

Every sync of a mailbox, initial or incremental, is one reconcile pass
(M1). It is safe to kill at any point and to run again.

1. `SELECT box (CONDSTORE)` (plain SELECT without CONDSTORE). Compare
   UIDVALIDITY with the stored value; on a change, see below.
2. **Fast path:** if UIDNEXT, HIGHESTMODSEQ and the message count all equal
   the stored state, the pass ends.
3. **Server UIDs:** if UIDNEXT or the count changed, ESEARCH
   `UID SEARCH RETURN (ALL) ALL` (plain `UID SEARCH ALL` without ESEARCH)
   gives the server set S; L is the local set.
4. **New:** fetch headers for S \ L, newest first, in chunks of 500. Each
   chunk is one transaction: insert (idempotent on `(mailbox, uid)`), thread,
   index, emit `message.changed`.
5. **Gone:** delete L \ S in one transaction and emit `message.removed`.
6. **Flags:** with CONDSTORE, `UID FETCH <L ∩ S> (FLAGS MODSEQ)
   (CHANGEDSINCE stored)`; without it, fetch FLAGS for the local window and
   compare. Flags with a pending local op are not overwritten.
7. Store UIDVALIDITY, UIDNEXT, HIGHESTMODSEQ and the count, which arms the
   fast path for the next pass.

QRESYNC (`SELECT … (QRESYNC …)` with `VANISHED (EARLIER)`) would replace
steps 3 and 5 with one round trip, but the go-imap client does not implement
it, and Gmail and Outlook do not offer it. It is a later patch to the fork for
Dovecot and Fastmail.

## Change detection

- INBOX: C2 IDLEs; any unsolicited EXISTS, EXPUNGE or FETCH ends the IDLE and
  queues a reconcile pass for INBOX (target: visible to clients within 5 s).
- Other mailboxes: `STATUS (MESSAGES UIDNEXT UIDVALIDITY HIGHESTMODSEQ)` on
  C1 every poll interval (default 5 minutes), and a pass for each mailbox
  whose status differs from the stored state. NOTIFY (Dovecot) can replace
  polling later.

## UIDVALIDITY change

M1 drops the mailbox's local messages and runs the pass as if new; bodies
are refetched on demand and land on the same blob (content addressing).
Later: refetch UIDs with Message-ID (and `X-GM-MSGID`), re-attach existing
messages by (Message-ID, date, size) to keep local IDs and avoid the
refetch, and re-resolve queued ops the same way.

## Gmail

Sync All Mail, Trash and Spam keyed by `gm_msgid`; labels from
`X-GM-LABELS` become `message_mailbox` rows (`\Inbox` is a label). Archive
removes `\Inbox`; move edits labels; delete moves to `[Gmail]/Trash`. Threads
come from `X-GM-THRID`. Never append to Sent (Gmail saves sent mail itself).

## Threading (non-Gmail)

Incremental JWZ over `thread_refs`: each Message-ID (seen or referenced) maps
to a thread; a message referencing two threads merges them. Subject grouping
(`mimex.NormalizeSubject`, compared case-insensitively) only joins a message
that has a `Re:` prefix and no references, within 7 days.

## Offline actions

M1 implements flags (`\Seen`, `\Flagged`, `\Answered`, Mail.app color bits),
moves, and deletes (to Trash; expunge when already in Trash or when the
account has no Trash). A refused op (a NO from the server) is marked failed
and its local change undone: moves return to the source mailbox, and flag
and expunge failures clear the mailbox's stored modseq so the next pass
refetches every flag.

One transaction: optimistic change, a `pending_ops` row and a durable event.
Replay resolves (mailbox, UIDVALIDITY, UID) at replay time; flags go as
`+FLAGS`/`-FLAGS` deltas, and incoming server flags do not overwrite a flag
with a pending op. Moves use MOVE, or COPY + `\Deleted` + `UID EXPUNGE` with
UIDPLUS, never a bare EXPUNGE; COPYUID/APPENDUID map new UIDs at once.
Repeated ops on one message collapse. Network errors back off; NO/BAD rolls
back the optimistic change and tells the user. Undo enqueues the inverse, or
cancels both if neither was sent.

## Sending (M3)

The outbox sets `send_at = now + undo delay` (or the send-later time) and
assigns the Message-ID at queue time. State becomes `sending` before DATA; on
restart a `sending` row first searches Sent for its Message-ID. SMTP 4xx
retries, 5xx fails. Append to Sent unless the account's `server_saves_sent`
(probed once: search Sent for the Message-ID 30 seconds after the first
send). Drafts: APPEND the new version, then delete the old UID.

## Auth (M4)

OAuth with a loopback redirect and PKCE through xdg-open or the OpenURI
portal; refresh tokens in Secret Service, access tokens in memory, refreshed
5 minutes before expiry; one retry on an auth failure, then `needs_reauth`.
Microsoft: `IMAP.AccessAsUser.All`, `SMTP.Send`, `offline_access`; work
tenants need admin consent for new apps (since November 2025). Gmail:
restricted scope; for personal use an unverified app in production status or
an app password (testing-status refresh tokens expire after 7 days).
