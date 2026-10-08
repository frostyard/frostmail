# 0014 — Images reach the reader's frame as data: URLs

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

[ADR-0005](0005-html-mail-rendering.md) renders message HTML in a sandboxed
`srcdoc` iframe and has maild rewrite inline (`cid:`) and fetched remote
images to `mailpart://localhost/` URLs into its parts cache, which the app
serves through a custom URI scheme. On the user's first real accounts
(2026-10-08), "Load Remote Content" fetched the images into the cache but
the reader showed none. An end-to-end test in the built app on WebKitGTK
narrowed it down: `mailpart://` images load in the app's own page but fail
in any child frame (`srcdoc` or `about:blank`, sandboxed or not), while a
`data:` image loads in the same frame. Inline images had the same fault;
no earlier test displayed a `mailpart://` image inside the frame.

## Decision

- `message.render` embeds the images it resolves, inline parts and
  fetched remote images alike, as `data:` URLs in the HTML, so the frame
  loads nothing: no custom scheme and no network.
- Only the types the sanitizer admits in `data:` URLs are embedded (PNG,
  GIF, JPEG, WebP), up to 16 MiB of image data per rendering
  (`render.MaxEmbeddedBytes`); an image past the budget, or of another
  type, keeps its `mailpart://` URL and does not show.
- Parts are still decoded into the parts cache: attachments open from it
  (`message.part`, `open_part`), and remote images are fetched once.
- The other layers of ADR-0005 stand: the sanitizer, the frame's sandbox
  and the CSP, and maild as the only fetcher of remote images.

## Consequences

- Images show in the reader in the built app, in the dev gateway and in
  the Flatpak alike.
- Renderings with many images are larger on the socket, bounded by the
  budget; images are base64-encoded again on every render.
- `mailpart://` remains for opening parts and for the app's own pages.
- `app/e2e/remote.e2e.ts` checks that a remote image shows in the frame.

## Alternatives considered

- **blob: URLs made by the app:** the page would fetch each image over
  `mailpart://` and hand the frame blob URLs; more moving parts in the app,
  and the same WebKit behavior might apply to blob: in frames.
- **Serving the message document itself from `mailpart://`:** gives up
  `srcdoc` and the inherited CSP that ADR-0005 relies on.

## References

- Shapes: [design/rendering.md](../design/rendering.md)
- Builds on: [ADR-0005](0005-html-mail-rendering.md)
