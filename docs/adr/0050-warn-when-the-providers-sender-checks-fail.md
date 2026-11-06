# 0050 — Warn when the provider's sender checks fail

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

A forged sender is the start of most phishing. The receiving provider
checks every message it accepts: DKIM signatures, SPF and the DMARC
policy of the domain in `From`, and writes its verdicts into an
`Authentication-Results` header (RFC 8601) above the headers that came
with the message. Gmail, iCloud, Outlook.com and Fastmail all add one.
maild already stores the first `Authentication-Results` header of every
message (`messages.auth_results`, [sync.md](../design/sync.md)), and
[ADR-0027](0027-unsubscribe-with-one-click.md) trusts its `dkim=pass` for
one-click unsubscribe. It does not verify DKIM itself and chose not to
(ADR-0027, Alternatives).

The reader shows nothing about these checks. Mail.app shows nothing
either; Gmail shows a warning on mail that fails them. Brian asked for a
warning banner on 2026-11-05 (T929a, from Odrade's review of nexmail).

Two facts limit what the header proves:

- maild cannot tell the provider's header from one the sender wrote.
  The first header is the provider's when the provider adds one; on a
  server that adds none (a plain Dovecot), the first header can be the
  sender's own.
- Verdicts other than pass and fail are common and mean nothing about
  the sender: `none` (no signature or no policy), `neutral`,
  `temperror`, `permerror`, `policy`.

## Decision

- **maild works out a sender check** for each message from its stored
  first `Authentication-Results` header, with the rules of
  [design/rendering.md](../design/rendering.md#sender-check), and returns
  it on `message.get` as `Message.senderCheck`: `pass`, `fail`, or
  absent for none. It verifies nothing itself and makes no network
  request.
- **DMARC decides; DKIM decides only without DMARC.** `dmarc=pass` is a
  pass and `dmarc=fail` a fail. When the header has no DMARC result, any
  `dkim=pass` is a pass, and a `dkim=fail` with no `dkim=pass` is a fail.
  Everything else is none. SPF alone never decides: it checks the
  envelope sender, not the `From` the reader shows.
- **The reader warns on a fail, and says nothing on a pass.** A fail
  shows a "Sender not verified" banner over the message
  ([specs/ui.md](../specs/ui.md#reader)). A pass shows no badge or mark,
  so a forged header can never add trust: the worst it can do is hide a
  warning on a server that adds no header of its own, where there would
  be none anyway.
- **Advisory only.** The check moves nothing, blocks nothing, and changes
  no other behavior (rules, notifications, Junk). ADR-0027's one-click
  rule stays as it is.

## Consequences

- Phishing that the provider caught but still delivered gets a visible
  warning, from data maild already has.
- A mailing list that changes a message can break its DKIM signature.
  With no DMARC result that is a fail, and the banner shows on list mail
  that is genuine. Providers that report DMARC (all of the user's) mostly
  avoid this, because lists that change messages rewrite `From`.
- A domain with `p=none` still gets `dmarc=fail` from most providers when
  its mail fails alignment. The banner shows for it: the check failed,
  whatever the domain asks receivers to do about it.
- On a server that adds no `Authentication-Results`, the sender controls
  the header, so the banner shows only on mail whose sender says it
  failed. The banner is never a guarantee, and its text does not say
  that other mail is safe.
- `Message` gains one optional field within protocol 1 (additive), and
  the parser has its own tests over real header shapes.

## Alternatives considered

- **Verify DKIM and DMARC in maild:** a DNS dependency, a second verdict
  that can disagree with the provider's, and the provider has already
  filtered on its own. ADR-0027 refused this for the same reasons.
- **A "verified" badge on a pass:** a forged header could earn it on a
  server that adds no header, and a badge invites trust the check cannot
  give.
- **Warn on any DKIM fail, with or without DMARC:** a message often
  carries several signatures, and one broken signature next to a DMARC
  pass is not a forgery. The warning would show on too much genuine mail
  to be read.
- **Check the provider's `authserv-id`:** it would reject a sender's
  forged header, but maild does not know each provider's id, and an
  unknown id would hide the warning on genuine provider headers.

## References

- Shapes: [design/rendering.md](../design/rendering.md#sender-check),
  [specs/ui.md](../specs/ui.md#reader) (the Sender check banner),
  [plans/0014](../plans/0014-small-features-from-the-nexmail-review.md)
  (Phase 1)
- Builds on: [ADR-0027](0027-unsubscribe-with-one-click.md) (the stored
  header and the provider's verdict), [ADR-0005](0005-html-mail-rendering.md)
- RFC 8601 (`Authentication-Results`), RFC 6376 (DKIM), RFC 7489 (DMARC)
