# Drafts and sending

Living document. Rationale: [ADR-0010](../adr/0010-compose-and-send.md).
Contracts: [specs/rpc-api.md](../specs/rpc-api.md) (`draft.*`, `outbox.*`,
`address.suggest`).

## Overview

```
compose window ── draft.update ──► drafts (store) ──5 s quiet──► Drafts mailbox (APPEND, replace)
               ── draft.send ────► outbox row (queued, due in 10 s) ──► worker ──► SMTP
                                         ▲ outbox.cancel                     │
                                         └────────── draft ◄────────────────┘ accepted ──► Sent (APPEND) ──► sent
```

## Drafts

- A **draft** is structured content in the `drafts` table: account,
  identity, To/Cc/Bcc, subject, HTML body, the attachments (blob ID, name,
  type, size), the reply context (`In-Reply-To`, `References`, the source
  message), a Message-ID chosen at creation, and the server copy's UID.
- `draft.create` makes an empty draft with the identity's signature, or a
  reply, reply-all or forward of a message (below). `draft.update` replaces
  the content and moves `updated_at` forward: to the clock, or 1 ms past its
  old value when the clock has not moved on (times have millisecond
  resolution, and an edit must always be newer than the copy saved before
  it). Every change emits `draft.changed`.
- **Server copy:** the account's IMAP actor looks for drafts quiet for 5
  seconds whose server copy is older than their content (`saved_at <
  updated_at`); it builds the message (Bcc kept, half-typed recipients left
  out), APPENDs it to the Drafts mailbox with `\Draft` and `\Seen`, records
  the new UID (UIDPLUS `APPENDUID`, else a search for the Message-ID) with
  `saved_at` = the `updated_at` it built from, and queues the previous
  copy's deletion (`\Deleted`, then `UID EXPUNGE` with UIDPLUS) as an
  offline op. Offline, it waits. A new draft counts as saved until its first
  edit, so drafts opened and closed untouched never reach the server.
  Deleting a draft queues its copy's deletion the same way. A copy the
  server refuses is retried after the draft changes again.
- `draft.open` on a message (normally in Drafts) returns the draft with the
  same Message-ID if there is one; otherwise it parses the message into a
  new draft (recipients including Bcc, subject, HTML or text body,
  attachments) that keeps the Message-ID and adopts the message as its
  server copy, so the next save replaces it.

## Replies and forwards

- **Reply:** To is the source's Reply-To, else From. **Reply all** adds the
  source's To and Cc to Cc, without the account's own addresses and without
  duplicates. Subject gets `Re: ` unless it already starts with a reply
  prefix. `In-Reply-To` is the source's Message-ID; `References` is the
  source's References plus its Message-ID.
- **Forward:** no recipients; subject gets `Fwd: `; the body starts with a
  header block (From, Date, Subject, To) and the source's text; the source's
  attachments are attached again.
- **Quoting:** replies quote the source's HTML (or its text, escaped and with
  line breaks) inside `<blockquote type="cite">` after an attribution line
  "On <date>, <name> wrote:". The quoted HTML is the sanitized rendering, so
  remote images stay out.

## Building a message

`internal/compose` turns a draft into RFC 5322 bytes:

- Headers: `Date`, `From` (identity), `To`, `Cc`, `Subject` (RFC 2047 when
  needed), `Message-ID`, `In-Reply-To`, `References`, `MIME-Version`.
  `Bcc` is written only in the Drafts copy, so the recipients survive on the
  server; a sent message's Bcc recipients only go into the SMTP envelope.
  Before writing anything, the builder rejects CR, LF or NUL in any header
  value (header injection), addresses that are not bare addr-specs, and
  malformed message or content IDs.
- Body: `multipart/alternative` with `text/plain; charset=utf-8` (made from
  the HTML with `mimex.HTMLToText`) and `text/html; charset=utf-8` (the
  draft's HTML in a minimal document), both quoted-printable. The HTML part
  sits in `multipart/related` with the inline images when there are any,
  and the alternative in `multipart/mixed` with the attachments when there
  are any. Attachments are base64 with the file name in
  `Content-Disposition` (RFC 2231) and in `Content-Type`'s `name` (RFC 2047,
  for older readers); `multipart/*` and `message/*` types are sent as
  `application/octet-stream`. The result is 7-bit with CRLF line endings.
- **Inline images:** a quoted message's inline images appear in the draft's
  HTML as `mailpart:` URLs. When building, maild replaces each with a
  `cid:` URL and adds the part as an inline image; remote images were never
  in the quoted rendering.
- The same builder makes the Drafts copy and the sent message.

## Outbox

- `draft.send` validates (at least one recipient, every address parses,
  attachments within the limit), builds the message, stores it as a blob,
  and inserts an outbox row: state `queued`, `send_at` = now + the undo
  delay (10 seconds; `FROSTMAIL_UNDO_DELAY`), the envelope recipients (To,
  Cc, Bcc), the Message-ID. A draft already queued, sending or accepted is a
  conflict; a failed row of the same draft is replaced. The draft row is
  kept, linked to the outbox row, until the message is accepted.
- **The undo delay** is the `undoDelay` setting (0, 10, 20 or 30 seconds,
  10 by default; [organize.md](organize.md#settings)). 0 queues the row due
  at once. `FROSTMAIL_UNDO_DELAY`, when set, overrides it (tests).
- **Send Later** ([ADR-0025](../adr/0025-maild-keeps-send-later-remind-me-and-undo-send.md)):
  `draft.send {id, sendAt}` with a time later than the undo delay builds
  the message with `sendAt` as its `Date` and queues the row `scheduled`,
  due at `sendAt`. `outbox.reschedule {id, sendAt}` rebuilds a scheduled
  row's message with the new `Date` and moves `send_at`; a `sendAt` within
  the undo delay sends it now. `OutboxItem.scheduled` tells the app which
  rows are Send Later rather than in their undo window.
- `outbox.cancel` on a `queued` row deletes it, emits `outbox.changed` with
  `deleted`, and returns the draft; for a scheduled row this is Edit, and
  the app reopens the draft. `outbox.retry` queues a `failed` row again,
  now.
- **Sender** (one goroutine per account beside its IMAP actor, so mail goes
  out while IMAP is down): wakes when a row is queued and when the next
  falls due; claims due rows oldest first (`queued` → `sending`, compare and
  set); SMTP (`internal/smtpx`): connect (implicit TLS or STARTTLS), AUTH
  PLAIN or LOGIN, the announced `SIZE` checked, `MAIL FROM` (identity),
  `RCPT TO` each recipient (any refusal aborts the whole message), `DATA`.
- **Accepted:** one transaction marks the row `accepted`, deletes the draft
  and queues its server copy's deletion, counts the recipients for
  suggestions, and queues the Sent copy as an offline op. The IMAP actor
  replays it: unless Sent already holds the Message-ID, it APPENDs the
  message with `\Seen`, then marks the row `sent`. Gmail files sent mail
  itself, so its rows go straight to `sent`; without a Sent mailbox, or when
  the server refuses the copy, the row is marked `sent` too (the message did
  go out).
- **Errors:** an unreachable server or broken connection retries after a
  minute (so queued mail goes out soon after the server returns); a 4xx
  reply retries after 1, 2, 5 and 15 minutes by attempt, then hourly;
  `last_error` is set either way. A 5xx reply, a rejected login or a
  message over the server's `SIZE` marks the row `failed` and keeps the
  draft, for the app to show with Retry and Edit.
- **After a crash or restart:** `accepted` rows need nothing, since their
  Sent copy is a durable op. Rows left in `sending` (found at startup, or
  stopped mid-send by an account restart) are settled after the account's
  next full pass has synced Sent: found there by Message-ID, they are
  accepted without a new copy; otherwise they are queued again at once. A
  crash between the server's acceptance and maild's commit can therefore
  send twice on servers that do not file sent mail themselves; the window
  is the time to commit one transaction.

## Attachments

- `draft.attach {id, path}` reads a local file (the app's file dialog or a
  drop), stores it in the blob store, and adds `{blob, name, type, size}`
  to the draft; the type comes from the file name, then sniffing.
- A draft's attachments and its built message count against the account's
  size limit: the SMTP server's `SIZE` when known, else 25 MB of attachments.
  `draft.send` refuses larger messages with `invalidParams`.

## Addresses

`addresses (address, name, count, last_seen)` is updated by `InsertHeaders`
for every From, To and Cc address and by sending. `address.suggest {prefix,
limit}` matches the address or any word of the name by prefix,
case-insensitively, ranked by `count` and then `last_seen`.

## Compose windows

- The main window opens `#/compose/<draftId>` in a new Tauri window labeled
  `compose-<draftId>` (an existing window for that draft is focused instead).
- The compose UI: identity menu (when the account has several), To, Cc and
  Bcc fields with address tokens and suggestions, subject, a TipTap editor
  with a formatting bar, an attachment strip, Send, and close (which keeps the
  draft; Delete Draft discards it).
- Each window has its own bridge connection to maild; closing the window
  closes it.

## Unsubscribing

[ADR-0027](../adr/0027-unsubscribe-with-one-click.md) decides the methods
and limits; Phase 6 of [plan 0008](../plans/0008-m5-mail-app-parity.md)
designs the mechanism here.

## Operational notes

- `mailctl send` builds and sends a draft from flags and stdin, for tests and
  scripts.
- The test server's Postfix allows 50 MB messages (`message_size_limit`),
  so 25 MB attachments fit after base64.
