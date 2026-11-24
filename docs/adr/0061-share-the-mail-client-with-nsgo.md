# 0061 — Share the mail client with NSGo

- **Status:** Proposed
- **Date:** 2026-11-23

## Context

The desktop and Lodge implement mailbox navigation, selection, composition and PIM workflows separately. Both speak the same maild protocol. Their message readers use different platform facilities to isolate email content.

## Decision

Build the shared view tree in Go with NSGo. Reuse the typed client, paged live views and domain models. Keep socket selection, Lodge admission and content URLs, file handling, window integration and the isolated message body behind platform-specific entry points. Use the versioned workspace components shared with PocketGo and FrostNotes.

## Consequences

The two clients can exercise the same commands against one fixture daemon. Linux, Windows and Lodge each require integration coverage for their host functions. Release promotion still uses the tested candidate's commit.

## Alternatives considered

Extending each existing frontend independently retains separate implementations of each interaction.

## References

- [App design](../design/app.md)
- [Native desktop](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- [Rendering](../design/rendering.md)
- [Lodge app contract](../specs/lodge-app.md)
