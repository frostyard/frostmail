# 0005 — Render HTML mail in a sandboxed iframe under a strict CSP

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

HTML mail is untrusted input written for Outlook and Gmail: nested tables,
remote images, tracking pixels, CSS `url()`, and sometimes scripts and forms.
Mail.app shows it faithfully, blocks remote content until allowed, and never
runs scripts. The app's webview also hosts the trusted UI.

## Decision

Three independent layers:

1. **Sanitize in maild** (M2): an allowlist sanitizer over
   `golang.org/x/net/html` removes scripts, event handlers, forms,
   `meta refresh`, `@import` and `javascript:` URLs. It rewrites `cid:` to
   `mailpart://` and remote URLs to `mailpart://remote/<hash>`, and records
   tracking pixels.
2. **Isolate in the webview:** the message is shown in an
   `<iframe srcdoc sandbox="allow-same-origin">`, never with `allow-scripts`.
   Same-origin is granted only so the parent can size the frame and intercept
   clicks.
3. **Forbid the network in CSP:** the app CSP
   (`app/src-tauri/tauri.conf.json`) allows images only from `'self'`,
   `mailpart:` and `data:`, and has no `https:`. A srcdoc iframe inherits it.

Remote images are fetched only by maild, without cookies or a referer, once
the user allows a message or sender. Message parts reach the webview through
the `mailpart://` scheme, served from maild's parts cache with
`Content-Security-Policy: sandbox`. The main frame may only navigate to the
app itself (`on_navigation`); links open in the system browser.

## Consequences

- The M0 spike verified layers 2 and 3 on WebKitGTK 2.52 (host) and 2.54
  (nsl): a hostile message ran no script and made zero network requests
  ([plans/0001](../plans/0001-m0-foundations.md)).
- Images do not load over `mailpart://` inside the frame; they are embedded
  as `data:` URLs instead ([ADR-0014](0014-images-embedded-in-the-reader-frame.md)).
- `mailpart://` is a local scheme to WebKitGTK and does not load from the
  `http://127.0.0.1:5173` page that `tauri dev` uses. Built apps
  (`tauri://localhost`) load it. UI work in dev mode needs a substitute for
  parts (M2).
- Each layer is tested on its own: sanitizer golden tests and fuzzing, a
  hostile-HTML Playwright suite in WebKit, and the spike checks.

## Alternatives considered

- **HTML converted to native widgets:** cannot render real-world mail.
- **Sanitizer only:** a single bug would expose the user.
- **A separate webview process per message:** heavier, and the sandbox plus
  CSP already gives the isolation needed.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Evidence: [plans/0001-m0-foundations.md](../plans/0001-m0-foundations.md)
