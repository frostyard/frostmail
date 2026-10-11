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
  on empty toolbar space toggles maximize (Tauri's drag-region handling does
  this; the app adds nothing).
  - Over the sidebar: sidebar toggle, Get Mail (spins while any account
    syncs).
  - Over the list: the source title (13/600) above a subtitle (11/400
    secondary): "1,234 messages, 12 unread", or "Searching…", or "N results".
    A mailbox of a read-only account adds " · Read-only" to its subtitle,
    and opening its messages does not mark them read.
  - Over the reader: Delete, Archive, Flag (menu of the 7 colors and Clear
    Flag), Mark (read/unread toggle), Move (menu of the account's mailboxes),
    Reply, Reply All and Forward (on the selection); New Message sits over
    the list.
    Then the search field (220 wide, 28 high, rounded 6), Settings
    (`Settings`, Ctrl+,) and the window controls (minimize,
    maximize/restore, close: 28 × 28 targets, 16px icons).
  - Buttons are 28 × 28 with 16px Lucide icons in `--text-secondary`;
    hover background `--selection-inactive`; disabled at 40% opacity. Every
    button has a tooltip naming it and its shortcut.

### Sidebar

- Background `--bg-sidebar`, 8px top padding.
- **Sections:** "Favorites", then one section per account titled with the
  account name. Favorites holds All Inboxes; then All Drafts, All Sent, All
  Junk, All Trash and All Archives, each only when two or more accounts
  have a mailbox of that role (unread counts summed); then VIPs, when there
  is at least one VIP; then Flagged. Section header: 26 high, 11/600
  `--text-secondary`, 12px left padding; a chevron appears on hover and
  collapses the section.
- **VIPs** (M5, [organize.md](../design/organize.md#vips)): the row (key
  `vips`, icon `star`) lists `vip is true`; under it at depth 1, one row
  per VIP person, in `vip.list` order, grouped by `personId` (one row for
  all of a person's addresses, labeled with the name), else one per
  address (labeled with the name, else the address): key `vip:<address>`
  of the group's first address, icon `user`, listing `from is` each of the
  group's addresses (`match: any`). Counts are unread.
- **Flagged** (M5): the row (key `flagged`) lists every flagged message
  and shows the number of them, read or not, as Mail.app does. Under it at
  depth 1, one row per flag color with at least one flagged message, in
  color order: key `flag:<color>`, the flag's name (`Settings.flagNames`,
  else Red, Orange, Yellow, Green, Blue, Purple, Gray), icon `flag` in
  `--flag-<color>` instead of `--accent`, listing `color is <color>`, with
  its number of flagged messages.
- **Smart Mailboxes** (M5, Phase 3): a section after Favorites, key
  `smart`, titled "Smart Mailboxes", shown even when empty. Its header
  shows, on hover and while focused, a 14px `plus` button named "New Smart
  Mailbox", which opens the smart mailbox sheet
  ([organize-ui.md](organize-ui.md)). A row per smart mailbox in
  `smart.list` order: key `smart:<id>`, its name, icon `folder-cog`,
  listing `smartMailboxId`, with its unread count. Right click (or the
  Menu key) on one opens a menu: Edit Smart Mailbox… (the sheet, filled)
  and Delete Smart Mailbox (`smart.delete`; the source becomes All Inboxes
  when it was that one). Its title is its name.
- **Send Later** (M5, [compose-ui.md](compose-ui.md#send-later)): after
  the Outbox section, a section of the same look, titled "Send Later",
  shown while some outbox message is `scheduled` and queued. A row per
  such message, soonest first: the subject, "To: " and the recipients,
  "Sends " and the time (`whenText`), then the actions Edit
  (`outbox.cancel`, then the draft opens in its compose window), Send Now
  (`outbox.reschedule {id, sendAt: now}`) and Change Time… (the time sheet
  titled "Send Later", starting at its time; OK calls
  `outbox.reschedule`). The Outbox section leaves these messages out.
- **Mailboxes** (M5, [organize.md](../design/organize.md#mailboxes)): an
  account's section header shows, on hover and while focused, a 14px `plus`
  named "New Mailbox" (not on a read-only account), which opens the
  mailbox sheet to make one at the top level. Right click (or the Menu
  key) on one of an account's mailbox rows opens, in this order:
  - New Mailbox… (the sheet, inside this mailbox), Rename Mailbox…, Move
    Mailbox… and Delete Mailbox…; the last three are disabled for a
    mailbox with a role. Delete asks first, as removing an account does
    (title "Delete Mailbox?", text `Delete the mailbox "<name>" and the
    messages in it?`, OK labeled Delete), then calls `mailbox.delete`;
    the source becomes All Inboxes when it was that mailbox.
  - Then, after a separator, Use This Mailbox For ▸ Drafts, Sent, Junk,
    Trash, Archive (the mailbox's role checked; `mailbox.setRole`), not
    for the inbox nor on Gmail (label) mailboxes; and for a trash mailbox
    Erase Deleted Items…, for a junk mailbox Erase Junk Mail…, which ask
    first (title "Erase Deleted Items?" or "Erase Junk Mail?", text
    `Erase the <total> messages in "<name>"? They cannot be recovered.`,
    OK labeled Erase), then call `mailbox.erase`.
  - Then, after a separator, Add to Favorites or Remove from Favorites.

  On a read-only account every item but the favorites one is disabled.
- **The mailbox sheet** (`app/src/features/sidebar/MailboxSheet.tsx`): the
  smart mailbox sheet's frame, 400 wide, titled "New Mailbox", "Rename
  Mailbox" or "Move Mailbox". "Name:" (a text input, at most 100
  characters; not when moving) and "Location:" (a `select`: "Top Level"
  valued `""`, then the account's mailboxes by path, valued by ID; not
  when renaming); then maild's error (`ALERT`), Cancel and OK. OK is
  disabled while the name is blank or a save is under way. OK calls
  `mailbox.create {accountId, name, parentId?}`, `mailbox.rename {id,
  name}` or `mailbox.move {id, parentId?}` (no `parentId` for the top
  level); the sheet closes when it succeeds and shows maild's error when
  it does not.
- **Favorites** (M5): after All Inboxes, the unified rows, VIPs and
  Flagged, the `favorites` setting's mailboxes in order: key
  `favorite:<id>`, the mailbox's name, its role's icon, its unread count;
  choosing one shows that mailbox. Right click on one: Remove from
  Favorites, Move Up and Move Down (disabled at the ends), each a
  `settings.set {favorites}`.
- **Drag and drop** (M5): message rows drag; the dragged messages are the
  selection when the row is in it, else the row. Dropping them on a
  mailbox row (an account's or a favorite) of their own account moves
  them there, as Move to does; with Alt held (as in Mail.app) or Ctrl
  (the Linux habit) it copies them, as Copy to does. A row under a drag that would take them shows
  `--selection-inactive`. Nothing happens on a mailbox of another account
  or of a read-only account.
- **Counts of built-in sources:** one `view.count` for VIPs, each VIP row,
  Flagged, colors 1–7 and each smart mailbox, when the window connects and
  300 ms after the last `mailbox.changed`, `vip.changed`,
  `settings.changed` or `smart.changed`.
- **Rows:** 28 high, left padding 12 + 16 × depth, 16px icon in `--accent`,
  8px gap, label 13/400 truncated, unread count right-aligned 12/400
  `--text-secondary` with 12px right padding, hidden when 0.
- **Mailbox order:** Inbox, Drafts, Sent, Junk, Trash, Archive, then the rest
  by name, nested by the hierarchy delimiter. A parent that is not selectable
  shows but cannot be selected.
- **Icons by role:** inbox `inbox`, drafts `file`, sent `send`, junk
  `shield-alert`, trash `trash-2`, archive `archive`, flagged `flag`, all
  inboxes `inbox`, a unified row its role's icon, other `folder` (Lucide
  names).
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
- **Sort and view options** (M5, P-202, P-203, P-209): at the right of
  the filter bar's row (outside its toolbar), a button "Sort by <Field>"
  with a 12px `chevron-down`, 12/16 `--text-secondary`, opening a menu:
  Date, From, To, Subject, Size, Flags, Unread, Attachments (the current
  one checked); a separator; Ascending and Descending (the current
  direction checked); a separator; Conversations and Contact Photos (each
  checked when on). Choosing a field sets its usual direction: Date,
  Size, Flags, Unread and Attachments descending, From, To and Subject
  ascending. The sort, the direction and both options are kept with the
  window's layout; the list's query carries `sort` and `ascending` unless
  they are Date, descending (the default), and `threads` with
  Conversations.
- **Contact photos** (M5): with Contact Photos on, each row shows, left of
  its two lines and 8px from them, a 32px circle: the sender's person's
  photo (`people.senders` for the rows' senders, then `people.photo`,
  kept for the session), else the reader header's initials on its tone.
- **Line 1:** sender (display name, else address) 13/600 truncated; at the
  right a 12px paperclip when `hasAttachments`, a 12px `alarm-clock` in
  `--text-secondary` when the message has a pending reminder (M5,
  `remindAt`; `role="img"` named "Reminder " and the time as `whenText`
  says it, also its tooltip), then the date 12/400 `--text-secondary`. When the sender's address is a VIP (M5), a 10px
  filled `star` in `--text-secondary` (`--accent-contrast` on a selected
  row in a focused list) comes before the name, 4px from it, named "VIP".
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
- **Empty:** "No Messages" centered, 15/400 `--text-secondary`. With a
  filter (below) it names the filter: "No Unread Messages", "No Flagged
  Messages", "No Messages with Attachments", with a "Show All" button
  (12/16 `--accent`, no background) under it that sets the filter to All.
- **Filter bar** (M4.5): a 32-high strip at the top of the list column,
  `--bg-window` with a 1px `--separator` bottom, 12px side padding, a
  `toolbar` named "Filter messages" holding four buttons 22 high, 4px apart,
  rounded 6, 8px horizontal padding, 12/16: All, Unread, Flagged,
  Attachments. The chosen one (`aria-pressed`) is `--text-primary` on
  `--selection-inactive`; the others `--text-secondary` with no background,
  `--text-primary` on hover. The filter narrows whatever the list shows,
  the source or a search, through the view query (`unread`, `flagged`,
  `hasAttachments`); All is no filter. A fifth button, More (`chevron-down`
  after its label, `aria-haspopup="menu"`, named "More filters"), opens a
  menu of To Me, Cc Me and From VIPs (M5), each a condition in the view's
  `filter` (`tome`, `ccme`, `vip` is true), so it combines with a source's
  own conditions; while one of them is chosen, More shows its label,
  pressed, and their empty lists read "No Messages to You", "No Messages
  Cc'd to You" and "No Messages from VIPs". It lasts until changed (not across
  restarts), is the same for every source, and changing it clears the
  selection. A row read while the Unread filter shows it stays until the
  filter or the source changes (maild keeps it: `ViewQuery.unread`); an
  unflagged row leaves the Flagged filter at once.
- **Row actions** (M4.5): while the pointer is over a row of a writable
  account, line 1's paperclip and date give way to three 22 × 18 buttons,
  rounded 4, each a 14px Lucide icon in `--text-secondary`
  (`--accent-contrast` on a selected row in a focused list), with a
  `--selection-inactive` hover background (`--accent-contrast` at 20% on
  such a selected row): Flag (`flag`; filled and named "Unflag" when the
  message is flagged), Archive (`archive`; only when the account has an
  archive destination, as for the menu) and Delete (`trash-2`). They act on
  that row's message alone, as Toggle Flag, Archive and Delete do, without
  selecting it or changing the selection, and the pointer press does not
  move focus. They are not in the tab order (the keys do the same); each
  has its name as tooltip with the shortcut ("Archive (Ctrl+Alt+A)").

### Reader

- Background `--bg-window`. **Empty:** "No Message Selected" centered,
  15/400 `--text-secondary`; with several messages selected, "N Messages
  Selected".
- **Conversation:** the messages of the selected message's thread, newest at
  the top, as a stack separated by 1px `--separator` lines. Messages that are
  in Trash are omitted unless the selected one is.
- **Header (per message, 16 / 20 padding):** a 40px avatar circle with up to
  two initials (13/600 white) on one of eight tones chosen by a stable hash of
  the address; to its right, line 1 the sender name (14/600), followed by a
  12px filled `star` in `--text-secondary` named "VIP" 4px after it when the
  sender is a VIP (M5), with the date
  right-aligned (12/400 `--text-secondary`, long form: "October 7, 2026 at
  9:41 AM"); line 2 the subject (13/600); then "To:" and, when present,
  "Cc:" lines (12/400 `--text-secondary`, names, else addresses, comma
  separated, truncated with "& N more" after 3).
- **Reminder banner** (M5, between header and the remote content banner,
  `--bg-banner`, 12/400, `role="status"`, as that banner is drawn): when
  the message has a pending reminder, "Remind Me: " and the time
  (`whenText`), then at the right two of that banner's buttons, Change…
  (the time sheet titled "Remind Me", starting at the reminder's time; OK
  calls `message.remind {ids: [id], at}`) and Clear (`message.remind
  {ids: [id]}`), disabled on a read-only account.
- **More Actions** (M5, P-304, P-307, P-308): at the right of each
  message's header, a 28px button named "More Actions" (Lucide
  `ellipsis`, 16px, `--text-secondary`) opening a menu: Show All Headers
  (or Hide All Headers), Raw Source…, a separator, Save As…, Print….
  - **All headers:** the message's header section (`message.source`'s
    `headers`) in a `section` named "All Headers" under the header: a
    `pre`, 11/16 monospace
    `--text-secondary`, `white-space: pre-wrap`, 16px side padding, until
    hidden or another message is shown.
  - **Raw Source:** a sheet (the smart mailbox sheet's frame, 720 wide,
    titled "Raw Source") with the whole message (`text`) in a 12/18
    monospace `pre` 480 high that scrolls, "The message is longer than
    what is shown." under it when `truncated`, and Close.
  - **Save As:** the save dialog (title "Save Message", default name the
    subject with `/` and `\` replaced by `-`, "(no subject)" when blank,
    plus `.eml`, filter "Email Message" `eml`), then `message.save {id,
    path}`; nothing when the dialog is cancelled; outside Tauri,
    nothing.
  - **Print:** the webview's print (`window.print()`). Print styles show
    only the reader pane, unscrolled and without toolbars or banners'
    buttons; the print dialog's Print to File is Export as PDF.
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
  current mailbox, and at its right a Save button (12/16 `--accent`, no
  background, 12px right margin) named "Save as Smart Mailbox" (M5), which
  opens the smart mailbox sheet with `smart.fromSearch`'s conditions for
  the search (and its mailbox, when the scope is the current mailbox).
  Escape in the field clears the search and returns to the previous
  source.
- **Context menu** on list rows (right click or the Menu key), in this
  order, with separators between the groups:
  - Reply, Reply All, Forward (on the last message of the menu's set).
  - Archive (only when the account has an archive destination), Delete,
    and Mark as Spam, which moves to the account's junk mailbox; when every
    message is already there it reads Not Spam and moves them to the
    inbox. Disabled without a junk mailbox (or an inbox, for Not Spam).
  - Move to ▸ and Copy to ▸, each the account's mailboxes except those
    holding the first message (Copy to on a Gmail account: only labels, and
    not Starred), by path; disabled when empty.
  - Flag or Unflag (as Toggle Flag), Flag Color ▸ the 7 colors (the first
    message's checked) and Clear Flag, then Mark as Read or Mark as Unread.
    The colors are labeled with the flag names, as the sidebar's are (M5);
    so is the toolbar's Flag menu. A flag glyph's accessible name keeps the
    color's own name ("Flagged Red").
  - Remind Me ▸ (M5), at the end of the Flag group, after Mark as Read or
    Mark as Unread: the reminder choices (`remindChoices`: Remind Me in 1
    Hour; Remind Me Tonight, at 9:00 PM, before then; Remind Me
    Tomorrow, at 8:00 AM), a separator, Remind Me Later… (the time sheet
    titled "Remind Me", starting tomorrow at 8:00 AM), and, when a message
    of the menu's set has a pending reminder, Clear Reminder. Each calls
    `message.remind` on the menu's messages, with the time or, for Clear
    Reminder, without.
  - Apply Rules (M5), only when a rule is enabled: `rule.apply` on the
    menu's messages, which runs the enabled rules on them now, wherever
    they are. It has no shortcut: Mail.app's Option-Command-L would be
    Ctrl+Alt+L, which locks the screen on most Linux desktops.

  Every item but Reply, Reply All and Forward is disabled when the
  account is read-only. The menu acts on the selection when the clicked
  row is part of it, otherwise on the clicked row alone, which it selects.
- **Focus:** Tab and Shift+Tab move between sidebar, list and reader. The
  focused pane shows its selection in `--accent`.

### Times (M5)

`app/src/lib/later.ts` says and chooses the times of Send Later and
Remind Me, in the user's time zone and locale:

- `whenText(at, now, timeZone, locale)`: "Today at 9:00 PM", "Tomorrow at
  8:00 AM", else the short weekday, month and day ("Mon, Oct 12 at 8:00
  AM"), with the year when it is not this year ("Mon, Jan 4, 2027 at 8:00
  AM"). The time is `hour: "numeric", minute: "2-digit"`.
- `atLocal(date, hour, minute, timeZone)`: the instant of a local
  `YYYY-MM-DD` at a wall-clock time.
- `sendChoices(now, timeZone, locale)`: "Send <9:00 PM> Tonight" (today at
  21:00) while it is before 21:00, and "Send <8:00 AM> Tomorrow"
  (tomorrow at 8:00), the times written as `whenText` writes them.
- `remindChoices(now, timeZone)`: "Remind Me in 1 Hour" (now plus an
  hour), "Remind Me Tonight" (today at 21:00) while it is before 21:00,
  and "Remind Me Tomorrow" (tomorrow at 8:00).

**The time sheet** (`app/src/features/later/TimeSheet.tsx`) asks for a
time: the smart mailbox sheet's frame ([organize-ui.md](organize-ui.md)),
360 wide, titled by its caller, with a date input named "Date" and a time
input named "Time" (the local date and time in the zone of the starting
time), then Cancel and OK. OK is disabled while the chosen time is not
after now, and gives the chosen instant.

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
| Ctrl+Shift+J | mark as spam, or not spam in the junk mailbox | anywhere with a selection |
| Ctrl+Alt+F, Ctrl+F | focus the search field | anywhere |
| Escape | clear search, close a menu | search field, menus |
| Ctrl+Shift+N | get new mail | anywhere |
| Ctrl+1 | All Inboxes | anywhere |
| Ctrl+Alt+S | toggle sidebar | anywhere |
| Tab / Shift+Tab | next / previous pane | anywhere |
| Enter, double-click | open a message in Drafts as a draft | list |
| Ctrl+N | new message | anywhere but a text field |
| Ctrl+R / Ctrl+Shift+R | reply / reply all to the selection | Ctrl+R: not in a text field |
| Ctrl+Shift+F | forward the selection | anywhere |
| Ctrl+, | open the settings window ([settings-ui.md](settings-ui.md)) | anywhere |

The compose window's own keys are in [compose-ui.md](compose-ui.md).

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
