# Message rendering

Living document. Rationale: [ADR-0005](../adr/0005-html-mail-rendering.md).
Contracts: [specs/rpc-api.md](../specs/rpc-api.md) (`message.render`,
`message.part`), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
(`mailpart://`).

## Overview

`message.render` turns a stored message into something the reader can show
without touching the network: sanitized HTML whose every resource is a
`mailpart://localhost/` URL into maild's parts cache, plus the readable
text. `internal/render` does the work; the engine calls it after the sync
engine has fetched the body.

```
blob (raw .eml) ─► mimex: pick the HTML part ─► sanitize ─► rewrite URLs ─► Rendering
                                                   │            │
                                    inline parts ──┘            └── remote images: blocked,
                                    into parts/m/<id>/              or fetched into parts/r/
```

## Design

- **Choosing the body.** The HTML part `mimex.BodyText` would derive text
  from (the first `text/html` outside attachments) is rendered. A message
  without one returns `html: ""` and the app shows `text` itself.
- **Sanitizer** (`internal/render/sanitize.go`): parses with
  `golang.org/x/net/html` and rebuilds the tree from an allowlist.
  - Kept elements: structure and text (`div span p br hr h1–h6 blockquote
    pre code ul ol li dl dt dd table thead tbody tfoot tr td th caption col
    colgroup center font b strong i em u s strike sub sup small big tt
    abbr address cite q mark ins del`), `a`, `img`, `style`. The content of
    `html`, `head` and `body` is kept without the tags themselves, so
    head `<style>` blocks survive.
  - Dropped with their content: `script noscript iframe frame
    frameset object embed applet form input button select textarea option
    svg math template link meta base title audio video source track canvas
    portal dialog`. Unknown elements are unwrapped (children kept).
  - Kept attributes: `align valign width height border cellpadding
    cellspacing bgcolor color face size dir lang title alt colspan rowspan
    nowrap start type class id style`, plus `href` on `a`, `src` on `img`,
    `background` on table elements. Every `on*` attribute and every other
    attribute is dropped.
  - URLs: `href` keeps `http`, `https` and `mailto` (anything else removes
    the attribute); image URLs (`src`, `background`, CSS `url()`) follow the
    URL rules below.
  - CSS (`style` attributes and `<style>` text) goes through a tokenizer:
    `@import`, `@font-face`, `@namespace`, `expression(`, `behavior`,
    `-moz-binding` and `javascript:` are removed; `url()` follows the image
    rules; everything else is kept, so layout survives.
- **Image URLs:**
  - `cid:X` → the part with Content-ID X is decoded into
    `parts/m/<messageID>/<part>.<ext>` and the URL becomes
    `mailpart://localhost/m/<messageID>/<part>.<ext>`. Unknown CIDs are
    removed.
  - `data:image/(png|gif|jpeg|webp);base64,…` is kept; other `data:` URLs
    are removed.
  - `http(s)://…`: if it is a **tracker**, it is removed and counted in
    `trackers`. Otherwise, without `remote`, it is removed and counted in
    `remote`; with `remote`, it is fetched into `parts/r/<sha256 of the
    URL>.<ext>` and rewritten, or removed and counted in `remote` when the
    fetch fails.
- **Trackers** are images with `width` and `height` of at most 2 (attributes
  or inline style), images hidden by inline style (`display: none`,
  `visibility: hidden`, `opacity: 0`), and images whose host is on the
  built-in tracker list (`internal/render/trackers.go`). They are never
  fetched.
- **Remote fetcher** (`internal/render/remote.go`): an `http.Client` without
  a cookie jar, sending no `Referer` and a generic User-Agent. Its dialer
  checks every resolved address before connecting and refuses loopback,
  private, link-local, multicast and unspecified ones, so a message cannot
  reach the local network (DNS rebinding included). At most 3 redirects, a
  10-second deadline per image, 5 MB per image, 32 images per message, 4 at a
  time. The response must be `image/*` by header and by
  `http.DetectContentType`. Results are cached by URL hash.
- **`message.part`** decodes one part into `parts/m/<messageID>/` with a
  safe file name (the part's name with `/`, `\`, control characters and
  leading dots removed, or `part-<path>.<ext>` from its type) and returns
  the relative path.
- **Parts cache** files are written to a temp file in the same directory and
  renamed. In M2 nothing evicts them; M4 adds a size cap with LRU eviction.

## Operational notes

- `render` needs the body: like `message.body`, it fails with
  `unavailable` while the account is offline and the body was never fetched.
- Tests: golden files for the sanitizer (`internal/render/testdata/*.html`
  and `*.golden.html`); a fuzz test asserting that output reparses to
  allowlisted elements and attributes only; fetcher tests against
  `httptest` servers through an injected resolver, including refused private
  addresses; and the app's hostile-HTML suite (zero network requests).
