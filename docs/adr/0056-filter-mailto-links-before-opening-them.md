# 0056 — Filter mailto links before opening them

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

A message can put fields such as `attach` in a `mailto` URL. A system mail
client may interpret those fields when the browser opens the URL.
Frostui v0.4.0 filters them for the native app. The web app's `openLink`
passes links to Lodge or the browser. Lodge's HTML reader uses a separate
content-origin frame whose links open directly, without that callback.

## Decision

Filter outgoing mailto links in Frostmail, as the owner chose for
[#262](https://github.com/frostyard/frostmail/issues/262). Apply the rule in
`openLink` before either Lodge or the browser opens the link, and in
maild's HTML sanitizer before the content-origin reader receives it.

Keep the address and only fields named `to`, `cc`, `bcc`, `subject` and
`body`, matching names in ASCII case. Preserve retained fields' spelling,
values, encoding, duplicates and order. Drop every other field, fields
without `=`, the fragment and an empty `?`. Decode nothing. A mailto URL
left with neither an address nor fields does not open. The web helper
refuses whitespace or control characters in mailto URLs, including forms
whose scheme a browser would recognize after removing tabs or newlines.

## Consequences

The HTML reader is protected even when its sandbox prevents the app from
intercepting clicks. Plain text, People and Unsubscribe share the web
helper. The native opener remains a further check.

Go and TypeScript implement the same rule and share a table of regression
cases adapted from frostui's `mailtoCases`. Future changes must keep both
implementations consistent. Filtering cannot control how an external mail
client interprets the retained address and fields.

## Alternatives considered

- **Filter only in Lodge's shell:** the owner chose Frostmail, and direct
  links from Lodge's content-origin reader do not pass through the shell.
- **Filter only in `openLink`:** misses that content-origin reader.

## References

- Shapes: [app design](../design/app.md), [rendering design](../design/rendering.md),
  [lodge app spec](../specs/lodge-app.md), [UI spec](../specs/ui.md).
- Builds on: [ADR-0036](0036-reader-links-open-as-new-window-requests.md),
  [ADR-0041](0041-maild-runs-as-a-lodge-app.md).
