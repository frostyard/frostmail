# 0004 — Fork go-imap for Gmail extensions

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

emersion/go-imap v2 is the strongest Go IMAP client (IDLE, MOVE, UIDPLUS,
CONDSTORE, ESEARCH, SPECIAL-USE; not QRESYNC), but it is still beta
(v2.0.0-beta.8) and its FETCH parser fails the whole response on any
attribute it does not know. Gmail sync needs `X-GM-MSGID` (a stable ID across
labels), `X-GM-THRID` (threads), `X-GM-LABELS` (labels as memberships) and
`X-GM-RAW` (server search).

## Decision

Vendor go-imap v2.0.0-beta.8 in `third_party/go-imap` with a `replace`
directive, and patch in X-GM-EXT-1 support plus skipping of unknown FETCH
attributes. The patches are listed in `third_party/go-imap/FROSTMAIL-PATCHES.md`
and tested in `imapclient/gmail_test.go`. Only `internal/imapx` imports go-imap.

## Consequences

- Gmail sync can key messages by `X-GM-MSGID` and avoid downloading each
  message once per label.
- Upgrading means re-applying a small, documented patch set; the patches go
  upstream.
- go-imap's in-memory server has no CONDSTORE, so that path is tested
  against Dovecot and recorded transcripts instead
  ([design/testing.md](../design/testing.md)).

## Alternatives considered

- **Wait for upstream:** blocks Gmail, one of the four v1 account types.
- **Raw commands beside go-imap:** responses would still break the parser.
- **A different IMAP library:** none in Go is as complete.

## References

- Shapes: [design/sync.md](../design/sync.md)
- Patches: [third_party/go-imap/FROSTMAIL-PATCHES.md](../../third_party/go-imap/FROSTMAIL-PATCHES.md)
