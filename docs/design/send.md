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
  the content and bumps `updated_at`; it emits `draft.changed`.
- **Server copy:** the account's draft saver (part of the account actor)
  looks for drafts quiet for 5 seconds whose server copy is older than their
  content; it builds the message, APPENDs it to the Drafts mailbox with
  `\Draft` and `\Seen`, records the new UID (UIDPLUS `APPENDUID`, else a
  search for the Message-ID), and deletes the previous UID (`\Deleted` and
  `UID EXPUNGE`, else `EXPUNGE`). Offline, it waits. Deleting a draft queues
  the server copy's deletion like any offline action.
- A draft opened from the Drafts mailbox (a message there that is not in the
  `drafts` table) becomes a draft by parsing it: recipients, subject, HTML or
  text body, attachments.

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
  total size within the account's limit), builds the message, stores it as a
  blob, and inserts an outbox row: state `queued`, `send_at` = now + the undo
  delay, the envelope recipients (To, Cc, Bcc), the Message-ID. The draft
  row is kept, linked to the outbox row, until the message is accepted.
- `outbox.cancel` on a `queued` row deletes it and returns the draft.
- **Worker** (one per account, inside the account actor): picks due `queued`
  rows oldest first; marks `sending`; SMTP: connect (implicit TLS or
  STARTTLS), AUTH, `MAIL FROM` (identity) with `SIZE` when offered, `RCPT TO`
  each recipient, `DATA`. On acceptance it marks `accepted` in the same
  transaction that deletes the draft. Then it APPENDs the message to Sent
  (unless the account kind saves sent mail itself: Gmail) and marks `sent`.
- **Errors:** connection and 4xx errors put the row back to `queued` with
  `send_at` pushed out (1, 2, 5, 15, 60 minutes, then hourly) and
  `last_error` set; 5xx errors mark it `failed`, keep the draft, and emit
  `outbox.changed` so the app can tell the user and reopen it.
- **After a crash:** at startup, `accepted` rows resume at the Sent copy;
  `sending` rows are looked up in Sent by Message-ID and become `sent` if
  found, otherwise `queued` again.

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

## Operational notes

- `mailctl send` builds and sends a draft from flags and stdin, for tests and
  scripts.
- The test server's Postfix allows 50 MB messages (`message_size_limit`),
  so 25 MB attachments fit after base64.
