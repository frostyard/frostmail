# 0037 — Fetch what the reader shows, and prefetch it

- **Status:** Accepted
- **Date:** 2026-10-15

## Context

maild fetches a message's body when the user opens it: `UID FETCH
BODY.PEEK[]` over the account's body connection (C3), the whole message,
attachments included, before `message.render` can start. On 2026-10-15
the user reported messages taking tens of seconds to open. Timed against
their accounts, rendering a message whose body was not yet stored took
0.3–2.5 s for most, 7–9 s for 4–7 MB messages, and 20 s for a 14 MB one
with a PDF. The main Gmail had 83 messages over 1 MB and 529 recent ones
with no body stored. The body connection serves one request at a time,
so a click on another message waits behind a long download.

Timing the IMAP commands themselves showed two more costs:

- C3 ran `EXAMINE` before every fetch, and Gmail took about 1.1 s to
  examine All Mail, so even small Gmail messages took over a second.
- Fetching only the shown parts takes two round trips. That was 0.3–0.8 s
  on both providers, against 0.15–0.45 s for a whole small message. For
  the 14 MB message it was 0.3 s against 17.4 s.

Gmail sent whole messages at about 0.8 MB/s, iCloud at over 10 MB/s.

`docs/design/sync.md` planned bodies prefetched for INBOX and VIPs within
90 days (P4), which was never built. Mail.app and Thunderbird show a
message's text without its attachments, and fetch an attachment when it
is opened.

## Decision

- **A display copy.** Opening a message fetches the parts a reader shows,
  not the whole message: one `UID FETCH` of its `BODYSTRUCTURE` and
  header, then one of the content and MIME headers of its parts.
  `imapx.Session.FetchDisplay` reassembles a message with the original's
  structure and boundaries, which parses as the original does. A part is
  shown, and its content fetched, when it is:
  - a text part that is not an attachment (by the rule sync uses for
    `has_attachments`: disposition `attachment`, or a file name without
    `inline`);
  - an invitation: `text/calendar`, `application/ics` or a `.ics` file,
    attached or not;
  - a part with a Content-ID, or an inline image, of 4 MB or less;
  - a `message/*` part of 1 MB or less.

  Every other part keeps its MIME header, with `X-Frostmail-Omitted: <size>`
  added, and an empty body. A structure that cannot be rebuilt (a
  multipart without a boundary) is fetched whole.
- **Small messages whole.** A message of 256 KB or less (`RFC822.SIZE`)
  is fetched whole: at that size one round trip is quicker than two on
  both providers.
- **C3 keeps its mailbox.** The body connection examines a mailbox only
  when the message is in another one. When a UID is not found in the
  mailbox it kept, it examines that mailbox again and retries once.
- **Stored beside the whole message.** The display copy goes into the
  blob store, and `messages.display_sha` names it. `blob_sha`,
  `body_state = 'full'` and `message.get`'s `bodyFetched` keep meaning
  the whole message.
- **What reads which.** Rendering, `message.body`, a reply's quote, the
  invitation card and unsubscribe information read the whole message
  when it is stored, else the display copy, fetching that when neither
  is. Raw Source, Save, forwarding, redirecting, reopening a saved draft
  and an omitted part's `message.part` fetch the whole message.
- **Prefetch (P4).** When a pass settles, the body connection fetches,
  by the rules above, the bodies of INBOX messages and VIPs' messages
  within 90 days that have neither copy, newest first and one at a time.
  A request from the user always goes first, so a click waits for at
  most one prefetch. A message that fails is not tried again until maild
  restarts, and a connection failure stops prefetch until the next pass.
- **No fetch twice.** The body connection checks the store again before
  each fetch, so two requests for the same message download it once.

## Consequences

- A message opens in about the time its text takes, whatever its
  attachments weigh, and recent INBOX mail usually opens from the store.
  A Gmail message no longer waits a second for `EXAMINE` when C3 already
  has its mailbox open.
- On 15 of the user's messages, Gmail and iCloud, with up to 16 MB of
  attachments, the display copy gave the same text, HTML and inline
  images as the whole message.
- Opening an attachment, Raw Source, Save and Forward of a big message
  still wait for the whole download, as they did.
- Prefetch costs bandwidth and disk for the text of recent INBOX and VIP
  mail (tens to hundreds of MB on a large inbox), on a connection the
  user's own requests come before.
- An inline image over 4 MB shows as missing until the whole message is
  fetched.
- Two copies of a message may be stored once its attachments are opened:
  the blob store addresses content, so their shared parts are not shared.

## Alternatives considered

- **Prefetch whole messages:** most clicks would be instant, but the first
  open of a big message stays slow, and attachments fill the disk.
- **Fetch shown parts without reassembling:** the renderer, the invitation
  and unsubscribe readers would each need a parts-based input instead of
  one message.
- **More body connections:** a big download would no longer block others,
  but it would stay slow itself, and providers cap connections.

## References

- Shapes: [design/sync.md](../design/sync.md),
  [design/rendering.md](../design/rendering.md)
- Builds on: [ADR-0016](0016-sync-a-window-of-recent-mail.md)
- RFC 3501 §6.4.5 (`BODYSTRUCTURE`, `BODY[<section>.MIME]`)
- Code: `internal/imapx/display.go`, `internal/mailsync/bodies.go`
