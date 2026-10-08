# Spec: the reader UI (M2)

The look and behavior of Frostmail's main window: a Mail.app-style three-pane
reader (sidebar, message list, reader) with its toolbar. It binds the app's
CSS tokens (`app/src/styles/tokens.css`), the presentational components built
from task cards, and the app tests. Measurements are CSS pixels at 100% zoom.
Shortcuts use Ctrl where Mail.app uses ⌘ and Alt where it uses ⌥ or ⌃⌘.

## Tokens

Defined as CSS custom properties on `:root`, with dark values under
`@media (prefers-color-scheme: dark)`. Components MUST use tokens, never
literal colors.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--bg-window` | `#ffffff` | `#1e1e1e` | list and reader background |
| `--bg-sidebar` | `#f2f2f4` | `#2a2a2c` | sidebar |
| `--bg-toolbar` | `#f7f7f8` | `#2c2c2e` | toolbar and title bar |
| `--bg-banner` | `#f5f5f7` | `#2c2c2e` | remote-content banner, attachment chips |
| `--text-primary` | `#1d1d1f` | `#f5f5f7` | names, subjects, body |
| `--text-secondary` | `#6e6e73` | `#a1a1a6` | dates, previews, counts, headers |
| `--text-tertiary` | `#a1a1a6` | `#6e6e73` | placeholders, disabled |
| `--separator` | `#e3e3e6` | `#3a3a3c` | hairlines between rows and panes |
| `--accent` | `#007aff` | `#0a84ff` | unread dot, icons, focused selection |
| `--accent-contrast` | `#ffffff` | `#ffffff` | text on `--accent` |
| `--selection-inactive` | `#e4e4e7` | `#3a3a3c` | selection when its pane lacks focus |
| `--selection-sidebar` | `#dcdce0` | `#3a3a3c` | selected sidebar row without focus |
| `--badge-bg` | `#e5e5ea` | `#3a3a3c` | thread count badge |
| `--badge-text` | `#6e6e73` | `#a1a1a6` | thread count text |
| `--focus-ring` | `rgb(0 122 255 / 0.5)` | `rgb(10 132 255 / 0.6)` | keyboard focus outline |
| `--flag-1` … `--flag-7` | see flags | see flags | flag colors |
| `--quote-1` … `--quote-3` | `#2e7bd6` `#2ea44f` `#c2410c` | `#5aa0f0` `#4cc76a` `#f0884a` | quote bars by level, cycling |
| `--avatar-0` … `--avatar-7` | `#5e5ce6` `#0a84ff` `#30b0c7` `#34c759` `#ff9f0a` `#ff375f` `#bf5af2` `#8e8e93` | same | avatar circles, by `avatarTone` |

**Flags.** `flagColor` 1–7 (the API's numbering, `$MailFlagBit0-2` + 1):

| Color | Name | Light | Dark |
| --- | --- | --- | --- |
| 1 | Red | `#ff3b30` | `#ff453a` |
| 2 | Orange | `#ff9500` | `#ff9f0a` |
| 3 | Yellow | `#ffcc00` | `#ffd60a` |
| 4 | Green | `#34c759` | `#30d158` |
| 5 | Blue | `#007aff` | `#0a84ff` |
| 6 | Purple | `#af52de` | `#bf5af2` |
| 7 | Gray | `#8e8e93` | `#98989d` |

**Type.** `font-family: "Inter Variable", system-ui, sans-serif`; base 13px.
Dates and counts use `font-variant-numeric: tabular-nums`.

| Role | Size / line height | Weight |
| --- | --- | --- |
| list sender | 13 / 18 | 600 |
| list subject | 13 / 18 | 400 |
| list preview | 12 / 16 | 400 |
| list date, sidebar count | 12 / 16 | 400 |
| sidebar row | 13 / 18 | 400 |
| sidebar section | 11 / 14 | 600 |
| reader sender | 14 / 20 | 600 |
| reader subject | 13 / 18 | 600 |
| reader address lines, date | 12 / 16 | 400 |
| reader plain-text body | 14 / 21 | 400 |
| toolbar title | 13 / 16 | 600 |
| toolbar subtitle | 11 / 14 | 400 |

## Layout

```
┌──────────────┬───────────────────────┬──────────────────────────────────┐
│ ▢  ⟳         │ Inbox            ✎    │ ⌫  ▣  ⚑▾  ✉  ⇥▾       ⌕ Search   – □ ✕ │  toolbar 52
├──────────────┼───────────────────────┼──────────────────────────────────┤
│ Favorites    │ ● Sender        9:41  │ (A) Sender                 date  │
│  All Inboxes │   Subject          3  │     Subject                      │
│  Flagged     │   preview preview…    │     To: …                        │
│ Account      ├───────────────────────┤ ──────────────────────────────── │
│  Inbox    12 │   …                   │ body                             │
│  …           │                       │                                  │
└──────────────┴───────────────────────┴──────────────────────────────────┘
   sidebar 220      list 360                  reader (rest)
```

- **Window:** default 1200 × 780, minimum 800 × 500. No system title bar.
- **Panes:** sidebar default 220 (min 160, max 320); list default 360
  (min 280, max 560); reader takes the rest (min 360). Panes are separated by
  a 1px `--separator` line with a 6px invisible drag handle. Widths persist
  per window in local storage. Ctrl+Alt+S hides and shows the sidebar.
- **Toolbar (52, `--bg-toolbar`, 1px `--separator` bottom):** one strip
  split at the pane boundaries. It is the window's drag region; double-click
  on empty toolbar space toggles maximize.
  - Over the sidebar: sidebar toggle, Get Mail (spins while any account
    syncs).
  - Over the list: the source title (13/600) above a subtitle (11/400
    secondary): "1,234 messages, 12 unread", or "Searching…", or "N results".
  - Over the reader: Delete, Archive, Flag (menu of the 7 colors and Clear
    Flag), Mark (read/unread toggle), Move (menu of the account's mailboxes);
    Reply, Reply All, Forward and Compose are present but disabled until M3.
    Then the search field (220 wide, 28 high, rounded 6) and the window
    controls (minimize, maximize/restore, close: 28 × 28 targets, 16px
    icons).
  - Buttons are 28 × 28 with 16px Lucide icons in `--text-secondary`;
    hover background `--selection-inactive`; disabled at 40% opacity. Every
    button has a tooltip naming it and its shortcut.

### Sidebar

- Background `--bg-sidebar`, 8px top padding.
- **Sections:** "Favorites" (All Inboxes, Flagged), then one section per
  account titled with the account name. Section header: 26 high, 11/600
  `--text-secondary`, 12px left padding; a chevron appears on hover and
  collapses the section.
- **Rows:** 28 high, left padding 12 + 16 × depth, 16px icon in `--accent`,
  8px gap, label 13/400 truncated, unread count right-aligned 12/400
  `--text-secondary` with 12px right padding, hidden when 0.
- **Mailbox order:** Inbox, Drafts, Sent, Junk, Trash, Archive, then the rest
  by name, nested by the hierarchy delimiter. A parent that is not selectable
  shows but cannot be selected.
- **Icons by role:** inbox `inbox`, drafts `file`, sent `send`, junk
  `shield-alert`, trash `trash-2`, archive `archive`, flagged `flag`, all
  inboxes `inbox`, other `folder` (Lucide names).
- **Selection:** the selected row has a 6px-rounded background inset 8px
  each side: `--accent` with `--accent-contrast` text and icon while the
  sidebar has focus, `--selection-sidebar` otherwise.
- **Sync state:** while an account syncs, a 12px spinner follows the section
  title; when it is offline or unauthorized, an alert icon does, with the
  error as tooltip.

### Message list

- Rows are **84 high**, fixed: 8 padding top and bottom, a 24px gutter on the
  left and 12 on the right; a 1px `--separator` at the bottom inset 24 from
  the left.
- **Line 1:** sender (display name, else address) 13/600 truncated; at the
  right a 12px paperclip when `hasAttachments`, then the date 12/400
  `--text-secondary`.
- **Line 2:** subject 13/400 truncated ("(No Subject)" in
  `--text-tertiary` when empty). In conversation mode with `threadCount > 1`,
  a badge at the right: 16 high, 5px horizontal padding, 8px radius, 10/600
  `--badge-text` on `--badge-bg`, showing the count.
- **Lines 3–4:** preview 12/16 `--text-secondary`, clamped to 2 lines.
- **Gutter:** a 9px `--accent` dot centered on line 1 when unseen; a 12px
  filled flag in `--flag-N` centered on line 2 when flagged.
- **Selection:** `--accent` background with every text, icon and dot in
  `--accent-contrast` while the list has focus; `--selection-inactive`
  otherwise. Multiple selection uses the same colors.
- **Dates:** today, the short time ("9:41 AM" in the locale); yesterday,
  "Yesterday"; within the last 6 days, the weekday ("Tuesday"); otherwise the
  locale's short date ("10/3/26").
- **Empty:** "No Messages" centered, 15/400 `--text-secondary`.

### Reader

- Background `--bg-window`. **Empty:** "No Message Selected" centered,
  15/400 `--text-secondary`; with several messages selected, "N Messages
  Selected".
- **Conversation:** the messages of the selected message's thread, newest at
  the top, as a stack separated by 1px `--separator` lines. Messages that are
  in Trash are omitted unless the selected one is.
- **Header (per message, 16 / 20 padding):** a 40px avatar circle with up to
  two initials (13/600 white) on one of eight tones chosen by a stable hash of
  the address; to its right, line 1 the sender name (14/600) with the date
  right-aligned (12/400 `--text-secondary`, long form: "October 7, 2026 at
  9:41 AM"); line 2 the subject (13/600); then "To:" and, when present,
  "Cc:" lines (12/400 `--text-secondary`, names, else addresses, comma
  separated, truncated with "& N more" after 3).
- **Remote content banner** (between header and body, `--bg-banner`, 12/400):
  "This message contains remote content." with a "Load Remote Content"
  button; when trackers were blocked, " N trackers blocked." follows. Hidden
  when there is nothing remote.
- **Body:** HTML in the sandboxed frame on a white page with 20px padding,
  in both color schemes, sized to its content (no inner scroll); otherwise
  plain text 14/21 with 20px padding, `white-space: pre-wrap`, URLs and email
  addresses as links. Quoted lines (`>` prefixes) render without the
  prefixes, indented 10px per level with a 2px left bar in `--quote-N`.
  Quoted blocks longer than 4 lines collapse to their first 2 lines with a
  "See More" link. A signature after a line of exactly `-- ` renders in
  `--text-secondary`.
- **Attachments:** below the body, wrapping chips 32 high (`--bg-banner`,
  6 radius, 8px padding): a 16px `file` icon, the filename (truncated at
  200px) and the size ("12 KB", "1.4 MB"). Clicking opens the file with the
  system's default application.
- **Links** open in the system browser; only `http`, `https` and `mailto`
  open. Others do nothing.

## Behavior

- **Source:** the sidebar selects the source of the list: a mailbox, All
  Inboxes (role inbox, every account) or Flagged (flagged, every account).
  Searching replaces the source until the search is cleared.
- **Conversation mode** is on by default: one row per thread (its newest
  message in the source), with the badge.
- **Opening** a message (selecting exactly one) marks every message the
  reader then shows seen at once: the whole conversation, up to the first 20.
- **Selection** follows the selected IDs through deltas. If a selected row
  disappears, the row that took its index is selected (the one below), or the
  last row if it was last.
- **Search:** the field filters as typed after 250 ms of no typing. A scope
  bar under the toolbar (28 high) offers "All Mailboxes" (default) and the
  current mailbox. Escape in the field clears the search and returns to the
  previous source.
- **Context menu** on list rows (right click or the Menu key): Mark as
  Read / Mark as Unread; Flag ▸ the 7 colors and Clear Flag; Move to ▸ the
  account's selectable mailboxes; Archive (when the account has an archive
  mailbox); Delete. The menu acts on the selection when the clicked row is
  part of it, otherwise on the clicked row alone.
- **Focus:** Tab and Shift+Tab move between sidebar, list and reader. The
  focused pane shows its selection in `--accent`.

### Keyboard map

| Keys | Command | Where |
| --- | --- | --- |
| ↑ / ↓ | previous / next row | list, sidebar |
| Shift+↑ / Shift+↓ | extend the selection | list |
| Home / End | first / last row | list |
| Ctrl+A | select all | list |
| Space / Shift+Space | page the reader down / up | list, reader |
| Delete, Backspace | delete selection | list |
| Ctrl+Alt+A | archive selection | anywhere |
| Ctrl+Shift+U | toggle read | anywhere with a selection |
| Ctrl+Shift+L | toggle flag (red when setting) | anywhere with a selection |
| Ctrl+Alt+F, Ctrl+F | focus the search field | anywhere |
| Escape | clear search, close a menu | search field, menus |
| Ctrl+Shift+N | get new mail | anywhere |
| Ctrl+1 | All Inboxes | anywhere |
| Ctrl+Alt+S | toggle sidebar | anywhere |
| Tab / Shift+Tab | next / previous pane | anywhere |

Ctrl+N, Ctrl+R, Ctrl+Shift+R, Ctrl+Shift+F and Ctrl+Shift+J are reserved for
compose, reply, reply all, forward and junk (M3–M4).

## Rules

- No component draws literal colors or fonts; tokens only.
- No view of messages is built in the app except through `view.open`.
- The reader frame never has `allow-scripts`; the app never fetches remote
  content (ADR-0005).
- Every interactive element is reachable by keyboard and has an accessible
  name.

## References

- Rationale: [ADR-0009](../adr/0009-ui-architecture.md),
  [ADR-0005](../adr/0005-html-mail-rendering.md)
- Context: [design/app.md](../design/app.md)
