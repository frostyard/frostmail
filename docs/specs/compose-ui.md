# Spec: the compose window and sending UI (M3)

The look and behavior of Frostmail's compose window (one Tauri window per
draft) and of the sending feedback in the main window: the undo toast and
the outbox. It binds the presentational components in
`app/src/features/compose/` and `app/src/features/outbox/`, the containers
that wire them to maild, and the app tests. Tokens, type, button styles and
the conventions for measurements and shortcuts are those of the reader UI
([ui.md](ui.md)); this spec adds no new tokens.

## Layout

```
┌──────────────────────────────────────────────────────────────┐
│ ➤   New Message                        📎  Aa  🗑    – □ ✕ │  toolbar 52
├──────────────────────────────────────────────────────────────┤
│      To: (Ann Example) (bob@x.test) typed…                    │  30+
│      Cc:                                                      │  30+
│ Subject: Lunch                                                │  30
│    From: Ann Example <ann@x.test>  ▾                          │  30 (several identities)
├──────────────────────────────────────────────────────────────┤
│ B I U S │ • 1. ❝ │ 🔗 ⌧                                        │  format bar 32 (toggled)
├──────────────────────────────────────────────────────────────┤
│ body (TipTap)                                                 │
│                                                               │
├──────────────────────────────────────────────────────────────┤
│ [📄 notes.pdf 1.2 MB ✕] [🖼 photo.jpg 3.4 MB ✕]   2 files, 4.6 MB │  attachments
└──────────────────────────────────────────────────────────────┘
```

- **Window:** default 720 × 560, minimum 480 × 360, no system title bar.
  Its title is the subject, or "New Message" when the subject is empty.
- **Toolbar (`ComposeToolbar`, 52, `--bg-toolbar`, 1px `--separator`
  bottom, drag region):** 8px side padding, 4px gaps.
  - Left: Send (Lucide `Send`), enabled only when the draft can be sent;
    enabled it is drawn in `--accent`, disabled at 40% opacity. Right after
    it, Send Later (M5): a 16 × 28 button with a 12px `chevron-down`,
    named "Send Later", enabled with Send, which opens the Send Later menu.
  - Then the title (toolbar title role, truncated) in the flexible middle,
    which is part of the drag region.
  - Right: Attach Files (`Paperclip`), Show Format Bar (`Type`, pressed
    while the format bar shows), Delete Draft (`Trash2`), then the window
    controls exactly as in the main toolbar.
  - Buttons are the main toolbar's: 28 × 28, 16px icons in
    `--text-secondary`, hover `--selection-inactive`, a tooltip naming the
    action and its shortcut.
- **Header (`ComposeHeader`):** rows in this order: To, Cc, Bcc (only while
  Bcc is shown), Subject, From (only when the account has more than one
  identity). Each row is at least 30 high with a 1px `--separator` bottom,
  16px left padding and 16px right padding; a 64px label column holds the
  label ("To:", "Cc:", "Bcc:", "Subject:", "From:") right-aligned in 13/18
  `--text-secondary` with 8px space after it.
  - To, Cc, Bcc: a `RecipientField`.
  - Subject: a borderless text input, 13/18 `--text-primary`.
  - From: a borderless `select` listing "Name <email>" per identity.
- **Recipient field (`RecipientField`):** the field's addresses as tokens,
  then a borderless text input (minimum 80 wide, flexible), wrapping onto
  more lines as needed with 4px gaps and 4px vertical padding.
  - A token is 22 high, 6px horizontal padding, rounded 4, 13/18; it shows
    the name, or the address when there is no name, and its tooltip is
    "Name <address>" (or the address).
  - Tokens are drawn in `--accent`; a selected token is `--accent` filled
    with `--accent-contrast` text; an invalid address is drawn in
    `--flag-1`.
  - **Suggestions:** a list below the field (`--bg-window`, 1px
    `--separator` border, rounded 6, shadow, at most 8 rows, above the
    editor). Each row is 36 high with 8px padding: the name (13/18
    `--text-primary`) over the address (12/16 `--text-secondary`), or only
    the address. The highlighted row is `--accent` filled with
    `--accent-contrast` text for both lines.
- **Format bar (`FormatBar`, 32, `--bg-window`, 1px `--separator` bottom,
  8px side padding, 2px gaps):** toggles 28 × 24, rounded 4, 16px icons in
  `--text-secondary`, pressed toggles on `--selection-inactive` in
  `--text-primary`. Groups are split by a 1 × 16 `--separator` rule with
  4px margins:
  1. Bold (`Bold`, Ctrl+B), Italic (`Italic`, Ctrl+I), Underline
     (`Underline`, Ctrl+U), Strikethrough (`Strikethrough`).
  2. Bulleted List (`List`), Numbered List (`ListOrdered`), Quote
     (`TextQuote`).
  3. Link (`Link`, Ctrl+K), Clear Formatting (`RemoveFormatting`, not a
     toggle).
- **Body:** the TipTap editor, 16px vertical and 20px horizontal padding,
  14/21 `--text-primary`; it scrolls; it fills the space between the header
  (and format bar) and the attachments.
- **Attachments (`ComposeAttachments`, only when there are any):**
  `--bg-banner`, 1px `--separator` top, 8px vertical and 12px horizontal
  padding, chips wrapping with 8px gaps, then the total pushed to the right.
  - A chip is 32 high, rounded 6, `--bg-window` with a 1px `--separator`
    border, 8px horizontal padding, 6px gaps: a 16px icon in
    `--text-secondary` (`FileImage` for `image/*`, `FileText` for `text/*`
    and `application/pdf`, `FileArchive` for zip, gzip, tar and 7z, `File`
    otherwise), the file name (13/18, truncated at 200 wide), the size
    (12/16 `--text-secondary`), and Remove (`X` 14px).
  - A file still being attached is a chip with a spinning `LoaderCircle`
    instead of the icon, its name, no size and no Remove.
  - The total reads "1 file, 1.2 MB" or "3 files, 4.6 MB" in 12/16
    `--text-secondary`; over the size limit it is `--flag-1` and adds
    " — over the 25 MB limit" (the limit as given).
- **Undo toast (`UndoToast`, main window):** fixed 16px above the bottom
  edge, centered, 36 high, rounded 8, `--bg-toolbar`, 1px `--separator`
  border, shadow, 12px horizontal padding, 12px gaps, 13/18: "Sending
  “Subject”…" (the subject truncated at 280 wide, "(no subject)" when
  empty), then Undo (13/18 600 `--accent`) and the seconds left (12/16
  `--text-secondary`, tabular). One toast per queued message, newest on
  top, stacked 8px apart.
- **Outbox (`OutboxStatus`):** a section at the bottom of the sidebar,
  shown while any message is not yet sent: the heading "Outbox" (sidebar
  section role), then one row per message (8px vertical, 16px horizontal
  padding): the subject (13/18 600, truncated; "(no subject)" when
  empty), "To: " and the recipients (12/16 `--text-secondary`, at most 2
  names then "& N more"), the state line (12/16), and the row's buttons
  (12/16 `--accent`, 12px gaps).

## Behavior

### Recipients

- Typing `,` or `;` commits what was typed (the character is not
  inserted), as do Enter and Tab while no suggestion is highlighted, and
  leaving the field. Committing parses the text (`parseAddresses`): every
  address becomes a token, unless the field already has it (compared
  case-insensitively), and every piece that did not parse becomes an
  invalid token whose address is the piece as typed. Tab still moves focus
  when nothing was typed.
- Pasting text that contains `,`, `;` or a line break commits the pasted
  text at once; other pastes insert normally.
- Backspace in the empty input selects the last token; Backspace or Delete
  with a token selected removes it. Clicking a token selects it. Typing, or
  the input losing focus, clears the selection.
- While the trimmed text is not empty, the field asks for suggestions
  (`address.suggest`, through the container) and lists them, without the
  addresses already in the field. Results for text that has since changed
  are dropped. ArrowDown and ArrowUp move the highlight (none at first);
  Enter or Tab adds the highlighted address; a click adds the clicked one;
  Escape closes the list. Adding a suggestion clears the text.

### Formatting

- The format bar's toggles reflect the marks and nodes at the selection.
  Pressing a control runs the matching editor command and leaves focus and
  the selection in the editor.
- Link asks for a URL; an empty answer removes the link. Only `http:`,
  `https:` and `mailto:` URLs are accepted.

### Saving and sending

- Each edit updates the draft (`draft.update`) at most every 500 ms and on
  close; maild saves the server copy after 5 seconds of quiet. Closing the
  window keeps the draft; Delete Draft (Ctrl+Backspace) discards it, at
  once when it is empty and after a confirmation otherwise.
- Send (Ctrl+Enter) is enabled when the draft has at least one valid
  recipient and no invalid one, and nothing is still being attached. Sending
  closes the window at once and shows the undo toast in the main window;
  a Send Later message (`scheduled`) shows none.
- Undo, before the delay runs out, cancels the send (`outbox.cancel`) and
  reopens the draft in its compose window. When the delay runs out the
  toast leaves.
- A message that failed shows in the Outbox with its error (Send Later
  messages show in their own section until they go); Retry queues it
  again and Edit opens its draft. State lines: queued with no attempt
  "Waiting to send"; queued after a failed attempt "Retrying in N min: " and
  the error (N at least 1, rounded up); sending "Sending…"; accepted
  "Saving to Sent…"; failed "Not sent: " and the error, in `--flag-1`.
  Queued messages offer Edit; failed ones offer Retry and Edit.

### Send Later

- The Send Later menu (`menu`, as the main window's menus) holds the send
  choices (`sendChoices`, [ui.md](ui.md#times-m5): "Send 9:00 PM Tonight"
  before 9 PM, "Send 8:00 AM Tomorrow"), a separator, and "Send Later…",
  which opens the time sheet titled "Send Later", starting tomorrow at
  8:00 AM.
- Choosing a time sends as Send does, with `draft.send {id, sendAt}`: the
  window closes, and the message waits in the main window's Send Later
  section ([ui.md](ui.md#sidebar)) instead of showing an undo toast.
- maild builds the message with that time as its `Date` and sends it then,
  with the app closed or not; a time already past sends it now
  ([send.md](../design/send.md#outbox)).

### Attachments

- Attach Files (Ctrl+Shift+A) opens the system file dialog; files dropped
  anywhere on the window are attached as well. Each file shows as being
  attached until maild has stored it.
- Remove takes the file off the draft (`draft.detach`).

### Entry points

- Compose (Ctrl+N), Reply (Ctrl+R), Reply All (Ctrl+Shift+R) and Forward
  (Ctrl+Shift+F) in the main window create a draft and open its window,
  from the toolbar, the message context menu or the keyboard; Reply, Reply
  All and Forward act on the selected message (the last one of a selected
  conversation).
- Opening a message in a Drafts mailbox opens it in a compose window.

### Keyboard (compose window)

| Keys | Action |
| --- | --- |
| Ctrl+Enter | Send |
| Ctrl+Shift+A | Attach Files |
| Ctrl+Shift+B | Show or hide the Bcc field |
| Ctrl+Backspace | Delete Draft |
| Ctrl+W | Close the window (keeps the draft) |
| Ctrl+B, Ctrl+I, Ctrl+U, Ctrl+K | Bold, Italic, Underline, Link |

## Rules

- Components use tokens only and import nothing from `app/src/data/` or
  `app/src/rpc/` except generated types.
- The compose window never loads remote content: the editor's paste
  handling drops remote images, and quoted messages are maild's sanitized
  rendering.
- Every control is reachable by keyboard and has an accessible name; format
  toggles expose `aria-pressed`; the recipient input is a combobox
  (`aria-expanded`, `aria-controls`, `aria-activedescendant`).

## References

- Rationale: [ADR-0010](../adr/0010-compose-and-send.md),
  [ADR-0009](../adr/0009-ui-architecture.md)
- Context: [design/send.md](../design/send.md),
  [design/app.md](../design/app.md), [ui.md](ui.md)
