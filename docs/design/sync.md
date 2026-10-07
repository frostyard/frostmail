# Design: sync engine

Status: designed in M0, built in M1 (generic IMAP) and M4 (Gmail, Microsoft,
iCloud). Written by the planner, not by task cards
([ADR-0008](../adr/0008-local-executor-workflow.md)).

## Actors and connections

- One actor goroutine per account owns that account's connections and
  schedules work by priority: **P0** what the user is viewing (a body, an
  attachment) > **P1** replaying `pending_ops` > **P2** INBOX > **P3**
  incremental sync of other folders > **P4** backfill and prefetch.
- Up to 3 connections per account (Gmail allows 15): C1 runs commands; C2
  IDLEs on INBOX, re-issuing IDLE every 25 minutes and treating 10 silent
  minutes as dead; C3 handles bulk and body fetches. Other folders are polled
  every 5 minutes and whenever C2 wakes. Provider profiles can lower the cap.
- Connect: CAPABILITY, ID, `ENABLE CONDSTORE QRESYNC` (iCloud answers OK
  without the untagged ENABLED: treat as enabled),
  `LIST "" "*" RETURN (SPECIAL-USE SUBSCRIBED)`, with role fallback by name
  ("Sent Messages", "Deleted Messages" on iCloud).

## Initial sync

INBOX, then Sent, Drafts, then the rest. Fetch headers newest-first in UID
chunks of 500:
`UID FLAGS INTERNALDATE RFC822.SIZE ENVELOPE BODYSTRUCTURE MODSEQ
BODY.PEEK[HEADER.FIELDS (References In-Reply-To List-Id List-Unsubscribe
Authentication-Results)]`, plus `X-GM-MSGID X-GM-THRID X-GM-LABELS` on
Gmail. The preview comes from a partial fetch of the text part. Bodies are
fetched when opened (P0) and prefetched for INBOX and VIPs within 90 days
(P4). Headers are backfilled down to the configured window; search beyond it
falls back to server `UID SEARCH` (`X-GM-RAW` on Gmail).

## Incremental sync

- **QRESYNC:** `SELECT box (QRESYNC (uidvalidity modseq))` returns
  `VANISHED (EARLIER)` and changed flags; then fetch UIDs ≥ stored `uidnext`.
- **CONDSTORE only:** `FETCH 1:* (FLAGS) (CHANGEDSINCE modseq)`; detect
  expunges with ESEARCH `UID SEARCH RETURN (ALL) ALL` against local UIDs.
- **Neither:** compare flags and UIDs within the sync window; full reconcile
  nightly.
- A provider profile downgrades a server that breaks QRESYNC invariants (an
  unknown UID, a negative sequence number) to CONDSTORE-only.

## UIDVALIDITY change

Clear that mailbox's UIDs in `message_mailbox`, refetch UIDs with Message-ID
(and `X-GM-MSGID`), re-attach existing messages by (Message-ID, date, size)
so stored blobs are reused, and re-resolve queued ops the same way.
Unresolvable ops become conflicts shown to the user.

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
