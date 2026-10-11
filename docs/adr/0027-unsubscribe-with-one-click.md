# 0027 — Unsubscribe with one click through the sender's server

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

Mail.app shows a banner on mail from a list, "This message is from a
mailing list", with an Unsubscribe button (parity row P-305). A list
message says how to leave in its `List-Unsubscribe` header (RFC 2369): a
`mailto:` URI, an `https:` URI, or both. RFC 8058 adds
`List-Unsubscribe-Post: List-Unsubscribe=One-Click`, meaning an HTTPS
`POST` of that body to the `https:` URI unsubscribes with no page to
visit; the sender must cover both headers with a DKIM signature. Gmail and
Yahoo require one-click unsubscribe of bulk senders since 2024, so most
list mail the user gets offers it.

Frostmail has never made a request to a sender's server. The app never
fetches remote content ([ADR-0005](0005-html-mail-rendering.md)); maild
talks only to the account's IMAP, SMTP, DAV and OAuth servers. maild
stores `List-Unsubscribe` and the first `Authentication-Results` header,
which the receiving provider adds above any a sender forged, with its
DKIM verdict. A forged message could name any URI, including one on the
user's own network.

The user chose one-click through the sender's server (2026-10-10), over
offering only the `mailto:` form.

## Decision

- **Which way.** maild works out a message's unsubscribe method from the
  stored message's headers:
  1. **One-click:** an `https:` URI in `List-Unsubscribe`, the header
     `List-Unsubscribe-Post: List-Unsubscribe=One-Click`, and `dkim=pass`
     in the message's first `Authentication-Results`. maild sends an HTTPS
     `POST` of `List-Unsubscribe=One-Click`
     (`application/x-www-form-urlencoded`) to the URI.
  2. **Mail:** otherwise, a `mailto:` URI. maild queues a message to that
     address through the outbox, from the account's identity, with the
     URI's subject and body if it has them, else the subject
     "unsubscribe". The undo delay applies.
  3. **Web page:** otherwise, an `https:` URI with no one-click. The app
     opens it in the user's browser, as it opens a link in a message.
  4. **None:** no banner.
- **The user confirms.** The banner's Unsubscribe asks first, naming the
  list and, for one-click, the host the request goes to. Nothing is sent
  without that click.
- **The request carries nothing of the user's.** HTTPS only, certificate
  checked; no cookies, credentials or `Referer`; a fixed
  `User-Agent: Frostmail`; redirects not followed; 10 seconds at most; the
  response body discarded. A 2xx status is success; anything else is shown
  as a failure, and the banner then offers the next method if the message
  has one.
- **Never inside the user's network.** maild refuses a host that resolves
  to a loopback, private, link-local, multicast or unspecified address. It
  checks the address it is about to connect to, so DNS rebinding does not
  get around it.
- **Remembered.** maild records the unsubscribe on the message and its
  `List-Id`. Later mail from that list shows "You unsubscribed from this
  list" instead of the button.

## Consequences

- One click leaves a list, as in Mail.app and Gmail, with the app closed
  or not, and the request comes from maild rather than the app's web view.
- maild gains its first outgoing HTTP client that is not for an account's
  own servers. It lives in one place, with the address rule and limits
  above, and has tests against a local server for each refusal.
- The sender learns the user's IP address and that the message was read,
  as with any click on a link. The confirmation says where the request
  goes.
- A forged message without the provider's `dkim=pass` gets the `mailto:`
  or web-page method, never the `POST`. Accounts whose server adds no
  `Authentication-Results` never get one-click.
- maild reads `List-Unsubscribe-Post` from the stored message, which the
  reader has fetched by the time it shows the banner. It is not added to
  the headers every sync fetches, which would change every pass's `FETCH`
  and the recorded replays.

## Alternatives considered

- **Only the `mailto:` form:** no new trust boundary, but many bulk
  senders offer one-click only, and a mail can take days to act on. The
  user preferred one-click.
- **Open every `https:` URI in the browser:** the page usually wants
  another click, and the browser sends cookies and more.
- **Verify DKIM in maild:** another DNS dependency and a second opinion
  that can disagree with the provider. The provider's verdict is what
  Gmail and iCloud already use to filter the same mail.

## References

- Shapes: [design/send.md](../design/send.md) (unsubscribe, written in
  M5's Phase 1), [specs/parity.md](../specs/parity.md) (P-305),
  [plans/0008](../plans/0008-m5-mail-app-parity.md)
- Builds on: [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0010](0010-compose-and-send.md) (the outbox)
- RFC 2369 (`List-Unsubscribe`), RFC 8058 (one-click), RFC 8601
  (`Authentication-Results`)
