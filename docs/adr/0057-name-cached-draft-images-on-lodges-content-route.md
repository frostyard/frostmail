# 0057 — Name cached draft images on Lodge's content route

- **Status:** Proposed
- **Date:** 2026-11-11

## Context

The native compose window can read local `mailpart:` images. Behind Lodge,
its message-part lookup cannot name other cached images, including images
previously fetched by maild from a remote sender. Those images show empty.

## Decision

Add `draft.image` for local and session callers. It names an existing cached
image referenced by an `img` element in the saved draft. It returns the
existing `PartFile` shape, including the signed content path when maild
serves Lodge. It never fetches a remote URL. The native client uses this
lookup for cache paths which do not identify a message part.

maild checks draft access, an exact image-source reference, a relative
cache path, containment including symlinks, a regular file, the renderer's
image size limit, and a sniffed image content type before signing a path.

## Consequences

Lodge compose can display cached remote images without exposing arbitrary
host files or fetching content merely because an editor opened. An image
removed from the cache or absent from the saved draft remains unavailable.
Existing message-part loading keeps its current RPC and behavior.

## Alternatives considered

- **Expose arbitrary cache paths:** does not tie the lookup to the draft
  being edited and needlessly broadens the file-reading interface.
- **Embed every image in `draft.get`:** increases every draft response,
  including callers that do not display images.

## References

- Shapes: [Lodge app spec](../specs/lodge-app.md).
- Builds on: [ADR-0041](0041-maild-runs-as-a-lodge-app.md).
