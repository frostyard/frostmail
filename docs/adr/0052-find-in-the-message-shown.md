# 0052 — Ctrl+F finds in the message shown

- **Status:** Accepted
- **Date:** 2026-11-09

## Context

The reader has no way to find a word in the message it shows (parity row
P-309, [#197](https://github.com/frostyard/frostmail/issues/197)). Apple
Mail, Thunderbird and the browsers all have one. In Mail.app, ⌘F finds in
the message shown and ⌥⌘F goes to the mailbox search field.

Frostmail gives both Ctrl+F and Ctrl+Alt+F to the search field today
([specs/ui.md](../specs/ui.md#keyboard-map)). The parity checklist's rule
is that a shortcut Frostmail already uses keeps Frostmail's meaning
([specs/parity.md](../specs/parity.md#rules)).

What the app can reach:

- In the desktop app, an HTML body shows in a `srcdoc` frame with
  `allow-same-origin` and no scripts
  ([ADR-0005](0005-html-mail-rendering.md)): the app can read and mark
  the frame's document from outside it. A plain-text body
  (`PlainText.tsx`), the header and the banners are the app's own DOM.
- WebKitGTK and WebView2 have no find of their own that the app can open
  for the user.
- In lodge's web build, an HTML body loads from lodge's content origin in
  a frame with no same-origin access
  ([specs/lodge-app.md](../specs/lodge-app.md)): the app cannot search it.
  The browser's own find (Ctrl+F) searches the whole page, frames too.
- A conversation shows several messages at once, some collapsed
  ([specs/ui.md](../specs/ui.md#reader)).

## Decision

- **The keys follow Mail.app.** Ctrl+F finds in the message shown when the
  mail module's reader shows a message (one message selected). With no
  message shown (no selection, several selected, another module), Ctrl+F
  does what it does today: it focuses the search field, where there is
  one. Ctrl+Alt+F always focuses the search field.
  This is the one exception to the parity checklist's rule on shortcuts
  Frostmail already uses: the key that finds is the one every mail
  client and browser uses, and the search keeps Ctrl+F wherever no
  message is shown.
- **The desktop app has a find bar** at the top of the reader. It finds
  in everything the reader shows: the headers, the banners and the bodies
  of the expanded messages, plain text and HTML frames alike, in the
  order they show. It finds case-insensitively, also across inline
  elements. It marks every match with the CSS Custom Highlight API, in
  the app's document and in each frame's, and the current match more
  strongly; it scrolls the reader to the current match and shows the
  count ("3 of 12"). Enter and Shift+Enter go to the next and previous
  match, and Escape or Done closes the bar and removes the marks.
- **The web build has no find bar.** There, Ctrl+F with a message shown
  is left to the browser, whose find reaches the content-origin frame.
  The command palette does not list Find in the web build.
- Without the Highlight API, the bar still counts and scrolls; it marks
  nothing.

## Consequences

- Ctrl+F changes meaning while a message is shown; a user who pressed it
  to search the mailbox now presses Ctrl+Alt+F, or clicks the field. The
  keyboard map and the palette show both keys.
- The find bar reaches into the frame's document from the app's side, as
  the frame's sizing and link handling already do: the frame still runs
  no script.
- The frame's page style gains the two highlight rules, with literal
  colors, as its other page colors are.
- The two builds differ: the web build's find is the browser's, with its
  own look and its own count.
- A match is found again when the query changes or the user moves to the
  next or previous match, so a message that expands while the bar is open
  joins at the next move. The bar closes when the reader shows another
  selection.

## Alternatives considered

- **Keep Ctrl+F for the search field and find on another key:** keeps a
  habit, but departs from Mail.app, which Frostmail follows, and leaves
  the common key on the less frequent of the two actions while a message
  is open.
- **A find bar in the web build too:** the app cannot reach the
  content-origin frame (specs/lodge-app.md), so it could find only in
  plain text and headers; the browser's find is complete there.
- **Marks as `<mark>` elements in the frame's document:** changes the
  message's DOM, which the print and the frame's height then see; the
  Highlight API changes no node.

## References

- Shapes: [specs/ui.md](../specs/ui.md#find-in-the-message) (Reader: Find
  in the message; Keyboard map; Command palette),
  [specs/parity.md](../specs/parity.md) (P-309)
- Builds on: [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0009](0009-ui-architecture.md)
