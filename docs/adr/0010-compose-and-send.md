# 0010 — maild owns drafts and the outbox; the app edits them

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

M3 adds writing mail. Mail.app saves drafts continuously (to the server's
Drafts mailbox), sends after a short undo delay, keeps unsent mail in an
outbox while offline, and never sends a message twice. The app is a thin
client of maild ([ADR-0002](0002-daemon-and-thin-clients.md)) and may have
several compose windows open; maild keeps running when they close. The
schema already has `outbox` (with a send state machine) and `identities`
(addresses and signatures per account). Attachments reach 25 MB; JSON over
the socket is a poor carrier for that.

## Decision

- **Drafts live in maild.** A compose window edits a draft through
  `draft.update`, sending the whole structured content (identity,
  recipients, subject, HTML body, attachment list) when it changes, at most
  once a second. maild stores it locally at once, so a draft survives a crash
  or restart, and after 5 seconds without changes builds the message and
  replaces the draft's copy in the account's Drafts mailbox (APPEND the new
  version with `\Draft`, then delete the old UID).
- **maild builds every message** (`internal/compose`): RFC 5322 headers with
  RFC 2047 encoding, a generated Message-ID, `In-Reply-To` and `References`
  for replies, `multipart/alternative` with a plain-text part derived from the
  HTML, wrapped in `multipart/mixed` when there are attachments. Bcc never
  appears in headers. The HTML from the editor goes through maild's
  allowlist sanitizer before it is used.
- **Sending goes through the outbox.** `draft.send` turns a draft into an
  outbox row due after the undo delay (10 seconds by default);
  `outbox.cancel` before then turns it back into a draft. A per-account worker
  sends due rows over SMTP (go-smtp: implicit TLS or STARTTLS, AUTH PLAIN;
  XOAUTH2 in M4), then appends the message to Sent unless the provider saves
  sent mail itself. States: queued → sending → accepted → sent, or failed
  (permanent SMTP errors) with transient errors retried with backoff.
- **No duplicate sends after a crash.** A row is marked `accepted` in the same
  moment the SMTP server accepts the message, before the Sent copy. On
  startup, `accepted` rows only finish their Sent copy. Rows left in
  `sending` are first looked up in Sent by Message-ID; only if absent are
  they sent again.
- **Attachments travel as file paths.** The app hands maild a local path
  (`draft.attach`); maild copies the file into its blob store at once, so later
  edits to the file do not change the draft. Forwarding attaches the source
  message's parts by reference from its stored body.
- **One window per draft.** Compose windows are separate Tauri windows
  loading the same app at `#/compose/<draftId>`. Each window has its own
  maild connection, so the Rust bridge keeps connections per window.
- **The editor is TipTap** (ProseMirror) with a small formatting set: bold,
  italic, underline, strikethrough, lists, quote, link. The signature of the
  draft's identity is inserted when a draft is created.
- **Address suggestions come from mail already seen:** maild keeps an
  `addresses` table (address, name, how often and when last seen) updated as
  messages are stored, and answers prefix queries ranked by use.

## Consequences

- Drafts and queued mail are safe across app and maild restarts and across
  windows closing; a compose window is a view of maild state, not its owner.
- MIME and SMTP code has one implementation, in Go, tested against Postfix
  and Dovecot; the app never sees raw messages.
- Undo is a delay, not a recall: once SMTP accepts a message it is gone.
- Bridge connections per window add a little Rust state; JSON-RPC
  multiplexing stays in the webview.
- The Drafts mailbox on the server sees one replacement per quiet period, not
  per keystroke.

## Alternatives considered

- **Building MIME in the app:** duplicates go-message, ships raw messages over
  JSON, and makes crash-safe sending depend on a window staying open.
- **Sending immediately with no undo:** Mail.app's undo send is part of the
  parity target, and the delay costs nothing.
- **Recording sent Message-IDs only locally to prevent duplicates:** a crash
  between SMTP acceptance and the local write still resends; checking Sent
  and the `accepted` state close that gap except for a crash during the SMTP
  transaction itself.
- **A compose sheet inside the main window:** Mail.app composes in separate
  windows, and several drafts at once is common.

## References

- Implements: [design/send.md](../design/send.md),
  [plans/0005-m3-compose-and-send.md](../plans/0005-m3-compose-and-send.md)
- Related: [ADR-0009](0009-ui-architecture.md), [ADR-0005](0005-html-mail-rendering.md)
