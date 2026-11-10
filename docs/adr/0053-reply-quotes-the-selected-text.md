# 0053 — Reply quotes the selected text, as plain text

- **Status:** Accepted
- **Date:** 2026-11-09

## Context

Mail.app quotes only the selected text when the user replies with text
selected in the message. Frostmail always quotes the whole message
(parity row P-508, [#201](https://github.com/frostyard/frostmail/issues/201)).

What the app can reach:

- In the desktop app, an HTML body shows in a `srcdoc` frame with
  `allow-same-origin` and no scripts
  ([ADR-0005](0005-html-mail-rendering.md)): the app can read the frame's
  selection from outside it, as the find bar reads its text
  ([ADR-0052](0052-find-in-the-message-shown.md)). A plain-text body is
  the app's own DOM.
- In lodge's web build, an HTML body loads from lodge's content origin
  in a frame with no same-origin access
  ([specs/lodge-app.md](../specs/lodge-app.md)): the app cannot read its
  selection.
- maild builds a reply's quote (`internal/compose`) from the sanitized
  rendering, and the app never sends maild HTML to put in a draft.
- A conversation shows several messages; the selection can be in an older
  one than the message selected in the list.

## Decision

- **The selection travels as plain text.** When Reply or Reply All runs,
  the desktop app reads the selection's text in the reader and sends it
  as `draft.create`'s new optional `quote`. maild quotes it as it quotes a
  text-only message: escaped, under the same attribution line, in the
  same cite blockquote. Nothing in it is HTML, so nothing is sanitized.
- **The selection picks the message.** Reply answers the message whose
  body holds the selection, an older message of the conversation too.
- **Anything else quotes the whole message,** as before: no selection,
  white space only, or a selection that is not inside 1 message body.
- **The web build quotes the whole message.** It cannot read the frame's
  selection, and it does not read a plain-text body's either, so the two
  kinds of message behave the same there.

## Consequences

- A reply to selected HTML loses the selection's formatting and its
  inline images: links, bold and lists come as their text.
- `draft.create` gains 1 optional parameter (protocol 1, additive). Any
  caller can send it; maild does not check that the text is in the
  source, because the draft is the user's to change in any case.
- The two builds differ: the web build always quotes the whole message.
- The command palette takes the selection when it opens, because its
  field takes the app document's selection.

## Alternatives considered

- **The selection's HTML, sanitized by maild:** keeps formatting, but
  sends HTML from the app into a draft, which then needs a second
  sanitizer pass and a rule for `mailpart:` images in a fragment. Plain
  text is what the issue needs, with nothing to sanitize.
- **Offsets into the rendering, resolved by maild:** exact, but the
  offsets of a DOM selection do not map to maild's HTML without the
  frame's document, and a plain-text body is rendered by the app.
- **The web build asks lodge's page for its selection:** the content
  frame runs no script by design (specs/lodge-app.md), so it cannot
  answer.

## References

- Shapes: [specs/ui.md](../specs/ui.md#reply-with-the-selected-text),
  [design/send.md](../design/send.md#replies-and-forwards) (Quoting the
  selected text), [specs/parity.md](../specs/parity.md) (P-508)
- Builds on: [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0052](0052-find-in-the-message-shown.md)
