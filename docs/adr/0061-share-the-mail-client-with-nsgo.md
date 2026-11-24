# 0061 — Share the mail client with NSGo

- **Status:** Proposed
- **Date:** 2026-11-23

## Context

The desktop and Lodge implement mailbox navigation, selection, composition and PIM workflows separately. Both speak the same maild protocol. Their message readers use different platform facilities to isolate email content.

## Decision

Build the shared view tree in Go with NSGo. Reuse the typed client, paged live views and domain models. Keep socket selection, Lodge admission and content URLs, file handling, window integration and the isolated message body behind platform-specific entry points. Use the versioned workspace components shared with PocketGo and FrostNotes.

## Consequences

The two clients can exercise the same commands against one fixture daemon. Linux, Windows and Lodge each require integration coverage for their host functions. Release promotion still uses the tested candidate's commit.

### Host behavior and verification

- On Linux and Windows the client renders the workspace with NSGo. The selected HTML email uses an isolated platform WebView. Native print opens a text version with the message headers. Find and quote-selection also use maild’s text rendering.
- In Lodge the client uses the current origin for RPC and Lodge’s isolated content URL for the HTML reader. The shell bridge maps mail, draft and calendar routes to browser history. File downloads go through Lodge.
- Calendar, contact, task and compose edits use dialogs in the shared workspace. Opening, snoozing and dismissing reminders works from the Calendar view. The desktop message text size preference changes the message list and reader. Shell controls use NSGo’s typography.
- From the repository root, stage the app with `make nsgo-lodge-app`. Run `python3 dev/lodge/nsgo-smoke.py /path/to/tools` with the Lodge test shell, binaries and Firefox layout described in that script. It starts a disposable authenticated Lodge and maild, opens a fixture message, navigates Back, refreshes the reader and reopens an edited draft through its permalink.

## Alternatives considered

Extending each existing frontend independently retains separate implementations of each interaction.

## References

- [App design](../design/app.md)
- [Native desktop](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- [Rendering](../design/rendering.md)
- [Lodge app contract](../specs/lodge-app.md)
- [The client's look and the kit's home](0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
