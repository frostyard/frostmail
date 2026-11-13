# Spec: the reader UI (M2)

The look and behavior of Frostmail's main window: a Mail.app-style three-pane
reader (sidebar, message list, reader) with its toolbar. It binds the app's
CSS tokens (lodge-ui's `tokens.css`), the presentational components built
from task cards, and the app tests. Measurements are CSS pixels at 100% zoom.
Shortcuts use Ctrl where Mail.app uses ⌘ and Alt where it uses ⌥ or ⌃⌘.

## Tokens

Defined by [lodge's UI package](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md)
(`@frostyard/lodge-ui`) as CSS custom properties on `:root`, with dark
values under `@media (prefers-color-scheme: dark)` and under
`[data-theme="dark"]`. The values below are Frostmail's copy of
that contract: the app imports the package's `tokens.css` and `accents.css`, and
its own `styles/tokens.css` is gone. Components MUST use tokens, never
literal colors.

The package is the shared look of frostmail, frostnotes and frostread
([lodge ADR-0018](https://github.com/frostyard/lodge/blob/main/docs/adr/0018-a-ui-package-beside-the-client.md)).
[ADR-0009](../adr/0009-ui-architecture.md) adds a runtime dependency only
through the planner: the planner approved `@frostyard/lodge-ui` on
2026-10-21, and it is pinned in `app/pnpm-lock.yaml` to a lodge commit.

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

The domain tokens (`--flag-*`, `--quote-*`, `--avatar-*`) come from the
package's `accents.css`; an app that does not use them does not import it.

**The desktop app** (`internal/desktop`, [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md))
does not read these tokens. It draws in frostui's Frostyard look
(frostui's ADR-0011, from frostui v0.5.0): the Frostyard theme in a dark
desktop and Frostyard light otherwise, its colors from
`frostui.FrostyardTheme` and `ColorsOf`, Inter at frostui's text sizes
(which follow the desktop's text size), nearly square corners, hairlines
and no shadows. Where this spec names a token, a size in CSS pixels or a
rounded corner, the desktop app draws the Frostyard role that plays its
part ([plan 0015, The Frostyard look](../plans/0015-mygo-desktop.md#the-frostyard-look));
the layout, the order and the behavior bind both builds. The flag colors
are the same in both.

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
  a 1px `--separator` line with a 6px invisible drag handle, which turns
  `--accent` while it is dragged or focused. The splitter is `Splitter` from
  `@frostyard/lodge-ui/react`
  ([lodge ADR-0018](https://github.com/frostyard/lodge/blob/main/docs/adr/0018-a-ui-package-beside-the-client.md)):
  a focusable `separator` that reports its pane's share of the window as
  `aria-valuenow`, and that the arrow keys move 16 pixels at a time. Widths
  persist per window in local storage. Ctrl+Alt+S hides and shows the
  sidebar.
- **Toolbar (52, `--bg-toolbar`, 1px `--separator` bottom):** one strip
  split at the pane boundaries. It is the window's drag region; double-click
  on empty toolbar space toggles maximize.
  - Over the sidebar: sidebar toggle, Get Mail (spins while any account
    syncs).
  - Over the list: the source title (13/600) above a subtitle (11/400
    secondary): "1,234 messages, 12 unread", or "Searching…", or "N results".
    The desktop app shows the list's count once, at the status bar's end,
    and its subtitle is the source's unread count ("12 unread"), none
    while it has none or during a search (frostyard/frostmail#270).
    A Drafts list counts its drafts not yet saved ([Message
    list](#message-list)): "3 drafts not yet saved" when it holds no
    message, else after the messages, "2 messages, 0 unread · 1 draft not
    yet saved".
    A mailbox of a read-only account adds " · Read-only" to its subtitle,
    and opening its messages does not mark them read.
  - Over the reader: Delete, Archive, Flag (menu of the 7 colors and Clear
    Flag), Mark (read/unread toggle), Move (menu of the account's mailboxes),
    Reply, Reply All and Forward (on the selection); New Message sits over
    the list.
    Then the search field (220 wide, 28 high, rounded 6), Settings
    (`Settings`, Ctrl+,) and the window controls (minimize,
    maximize/restore, close). The window controls are `WindowControls` from
    `@frostyard/lodge-ui/react`
    ([lodge's UI package spec](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md)):
    the toolbar passes `maximized` and its own command callback, and keeps
    Settings beside them. They are the desktop app's: the web build has
    none, since the browser or lodge's shell holds the window.
    They are whose buttons the platform has, since Windows became a second
    platform ([ADR-0038](../adr/0038-windows-as-a-second-platform.md)): the
    toolbar reads `windowPlatform()` once and passes it as the group's
    `platform`.
    - On **Linux** the three buttons keep the look they had: lodge-ui's
      own, 28 × 28 targets with 16px icons in `--text-secondary`, 4px
      apart, and the segment keeps its 8px padding at both ends.
    - On **Windows** they are Windows' caption buttons: flat, 46 wide each,
      with 16px icons, the strip's full 52px height, no gap between them
      and none after Close, so Close reaches the window's right edge,
      where the pointer always finds it. Hover is `--selection-inactive`,
      except Close, which is red (`#c42b1c`) with white text, as Windows
      draws it. For them the segment keeps its 8px left padding only, and
      the row holding Settings and the buttons fills the strip's height.
      The strip's drag region stays on that row and never on a button. A
      build that draws no buttons, as a browser on Windows does, keeps the
      Linux padding, so Settings stays off the window's edge.
    In a window too narrow for
    the segment, the search field shrinks from 220 to 120. Then the message
    actions that do not fit move into a » menu from their end, Move first,
    as in Mail.app. The » is `chevrons-right`, named "More Toolbar Items",
    and shows only while an action is hidden; its menu lists the hidden
    actions in toolbar order, with Flag and Move as submenus. At the smallest widths the search gives up the rest of the
    room it needs. Settings and the window controls never shrink and stay
    in the window.
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
- **Follow Up**: after the mailbox tree and before the Outbox, a
  section of the same look, titled "Follow Up", with the tooltip "Sent
  mail with no reply for 3 days or more". It calls `thread.awaitingOthers
  {limit: 25, waitingDays: 3}`, which maild answers with the conversations
  whose newest message is the user's and waits on someone else: when the
  window connects, 1000 ms after the last `mailbox.changed`,
  `message.changed`, `message.removed`, `vip.changed`, `account.changed`
  or `settings.changed`, and every 15 minutes; an answer to an older
  request is dropped, and a failed refresh keeps the list it had. The
  section is hidden while the call has not answered and when no
  conversation waits. A row per conversation, in maild's order (those with
  a VIP recipient first, then the newest first): the subject (13/400
  truncated, "(No Subject)" when blank) after a 12px `star` in
  `--accent` named "VIP" when a recipient is one; then "To: " and the
  first two recipients, then " & N more" for the rest (`recipientCount`);
  then "Waiting 1 day" or "Waiting N days" 12/400 `--text-secondary`.
  When `total` is larger than the rows, a line after them says "Showing N
  of TOTAL". The header's chevron collapses the list. Clicking a row
  shows its message where it lives and selects it (`revealMessage`,
  `app/src/app/openMessage.ts`), which drops a list filter that would hide
  it. The selection stays on the message even when the list has not loaded
  its row; the list scrolls to the row when it is already loaded. Commands
  and the toolbar act on the pinned message as on a loaded row; moving or
  deleting it out of the source releases the pin, and so does removing the
  message. From a pinned message that the list has not loaded, the arrow
  keys go to the first row. The selection goes back to the list's rules
  when the user selects, changes the source or the filter, or the message
  is removed. In a narrow window the click closes Mailboxes. The section
  asks maild the same in the desktop app and in lodge's web build, whose
  caller is `session` instead of `local`. In the desktop app it is a
  section of the sidebar's source list, after the accounts, with the
  mailboxes' rows, hover, selection, keys and collapse
  (frostyard/frostmail#271): a row is the subject after an `alarm` icon
  (the `star` for a VIP recipient) with its wait as a tag ("4 days"), and
  its spoken name adds "VIP", the recipients and "waiting 4 days"; "Showing
  N of TOTAL" is a dim row. A click, or Enter, opens the message; the
  sidebar's choice stays on the row until the source shown changes. The
  source list's header has no tooltip, so the desktop app's lacks the
  rule's.
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
  right a 12px `bell-off` in `--text-secondary` when the conversation is
  muted (M5, `muted`; `role="img"` named "Muted", also its tooltip), a
  12px paperclip when `hasAttachments`, a 12px `alarm-clock` in
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
- **Drafts not yet saved** (M8.5): a Drafts mailbox lists its account's
  drafts that no stored message holds yet, and All Drafts every
  account's (`draft.unsaved`): drafts maild has whose server copy has not
  arrived, with sync off, offline, or within the quiet period, whether
  the user or an agent made them. So no draft is out of sight. They head
  the list, newest first, under the heading "Not Yet Saved" (28 high,
  11/600 `--text-secondary`, as the sidebar's section titles), which names
  the `group` of their rows, each the height of a message's:
  - line 1 is "To: " and the recipients (`formatAddressList`), or "No
    Recipients" in `--text-tertiary`, with the date it last changed;
  - line 2 is the subject, as a message's, then a "Not Saved" badge in the
    thread count's style;
  - lines 3–4 are the draft's own text, without its quote.

  A click, Enter or Space opens the draft in its compose window, or its
  `/compose/<id>` route in lodge. They are left out while searching or
  filtering. They refresh on `draft.changed` and `outbox.changed`, and
  as the list's count changes, so a draft gives way to its copy when that
  arrives. A list shows its own scope's alone: from All Drafts to one
  account's Drafts, the other accounts' are gone at once, before that
  account's arrive. When maild cannot list them, the heading stands over a
  line saying so (`role="alert"`) in place of an empty list. A Drafts list
  with no message but these shows no empty text.
- **Empty:** "No Messages" centered, 15/400 `--text-secondary`. With a
  filter (below) it names the filter and the source it narrows, titled as
  the toolbar titles it: "No Unread Messages in All Inboxes", "No Flagged
  Messages in Work", "No Messages with Attachments in Archive". While
  searching it names no source, and the Flagged filter in Flagged reads
  "No Flagged Messages". Under it, a "Show All" button (12/16 `--accent`,
  no background) sets the filter to All; under the Flagged filter outside
  Flagged, a "Show All Flagged" button in the same style, 12px after it,
  opens Flagged with the filter set to All, since flagged mail filed away
  is not in the source.
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
  menu. When the bar is narrower than its tabs, tabs move into this menu
  from the end: Attachments, then Flagged, then Unread; All and More stay
  in the bar, and More fits with its current label. The bar never scrolls,
  and no tab reaches under Sort. The menu holds the hidden tabs in bar
  order, then a separator if any, then To Me, Cc Me and From VIPs (M5),
  each a condition in the view's `filter` (`tome`, `ccme`, `vip` is true),
  so it combines with a source's own conditions. While any filter in the
  menu is chosen, More shows its label, pressed. The empty lists of the
  three read "No Messages to You", "No Messages Cc'd to You" and "No
  Messages from VIPs". The filter lasts until changed (not across
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
  They are pointer-only (`aria-hidden`, no button role): an option may not
  hold a control, and the keys reach the same actions.

### Reader

- Background `--bg-window`. **Empty:** "No Message Selected" centered,
  15/400 `--text-secondary`; with several messages selected, "N Messages
  Selected".
- **Conversation:** the messages of the selected message's thread, newest at
  the top, as a stack separated by 1px `--separator` lines. Messages that are
  in Trash are omitted unless the selected one is.
  - **Expanded and collapsed** (P-310, as Mail.app and Gmail show a
    conversation): a conversation of 1 message shows it expanded. In a
    conversation of 2 or more, the selected message and each message that
    was unread when the conversation opened show expanded; every other
    message shows collapsed. What is expanded stays expanded while the
    conversation is open, also when opening marks it read (Behavior).
  - **A collapsed message** is 1 row, a button (`aria-expanded="false"`)
    named by the sender's name and the long date ("Ann Smith, October 7,
    2026 at 9:41 AM"), 10 / 20 padding, at least 44 high: a 24px avatar
    circle (the header's initials and tone, 10/600 white), 8px, the sender
    name (list sender) truncated, the message's preview (list preview,
    `--text-secondary`) on the rest of the line, truncated with an
    ellipsis, and the date right-aligned in the list's short form (list
    date, `--text-secondary`). A click, Enter or Space expands it. It
    calls `message.get` and `message.render` only when it expands, so a
    long conversation loads only what it shows in full.
  - **Collapsing:** in a conversation of 2 or more, a click on an expanded
    message's header collapses it again, except a click on a button in it
    (an address, More Actions) or a click that ends a text selection. The
    collapse is for the pointer; from the keyboard, Tab reaches the next
    message's row. The narrow layout and lodge's web build show the same
    rows.
  - A More Actions request for the selected message by its keys
    (Ctrl+Shift+H, Ctrl+Alt+U) expands it first when it is collapsed.
- **Header (per message, 16 / 20 padding):** a 40px avatar circle with up to
  two initials (13/600 white) on one of eight tones chosen by a stable hash of
  the address; to its right, line 1 the sender name (14/600), followed by a
  12px filled `star` in `--text-secondary` named "VIP" 4px after it when the
  sender is a VIP (M5), with the date
  right-aligned (12/400 `--text-secondary`, long form: "October 7, 2026 at
  9:41 AM"); line 2 the subject (13/600); then "To:" and, when present,
  "Cc:" lines (12/400 `--text-secondary`, names, else addresses, comma
  separated, truncated with "& N more" after 3).
- **Sender check banner** (plan 0014 Phase 1,
  [ADR-0050](../adr/0050-warn-when-the-providers-sender-checks-fail.md);
  the first banner under the header, before the reminder banner, drawn as
  the remote content banner is, `role="status"`): when the message's
  `senderCheck` is `fail`
  ([rendering.md](../design/rendering.md#sender-check)), a 14px Lucide
  `shield-alert` in `--flag-1`, 6px before the text "**Sender not
  verified.** This message says it is from <domain>, but <domain> did not
  vouch for it. Be careful with its links and attachments." ("Sender not
  verified." 12/600; <domain> is the sender address's domain, lowercased;
  with no sender address, the second sentence reads "Its sender did not
  pass the provider's checks."),
  then at the right a button Show Details, which shows the message's All
  Headers as More Actions' Show All Headers does, and reads Hide Details
  while they show. No banner for `pass` or an absent `senderCheck`, and
  nothing else in the reader marks a pass. The banner prints with the
  message, without its button.
- **Reminder banner** (M5, between header and the remote content banner,
  `--bg-banner`, 12/400, `role="status"`, as that banner is drawn): when
  the message has a pending reminder, "Remind Me: " and the time
  (`whenText`), then at the right two of that banner's buttons, Change…
  (the time sheet titled "Remind Me", starting at the reminder's time; OK
  calls `message.remind {ids: [id], at}`) and Clear (`message.remind
  {ids: [id]}`), disabled on a read-only account.
- **Blocked banner** (M5, P-411, [ADR-0028](../adr/0028-mute-and-block-in-maild-undo-in-the-app.md);
  after the reminder banner, drawn as the remote content banner is): when
  the sender's address is blocked (the window's blocked list:
  `block.list`, again on `block.changed`; addresses compared
  lowercased), "This message is from a blocked sender." and at the right
  an Unblock button, `block.remove {addresses: [address]}`.
- **Unsubscribe banner** (M5, P-305, [ADR-0027](../adr/0027-unsubscribe-with-one-click.md);
  after the reminder banner, drawn as the remote content banner is): for a
  message whose `listUnsubscribe` is not empty, `message.unsubscribeInfo
  {id}`. When it offers a method, "This message is from the mailing list
  <list>." and at the right an Unsubscribe button; when `done`, "You
  unsubscribed from <list>." and no button. No banner when it offers
  nothing and is not done.
  - The methods are tried best first, as maild orders them; on a
    read-only account mail is left out, and with nothing left the button
    is disabled.
  - Unsubscribe asks first: a dialog (the time sheet's frame) titled
    "Unsubscribe from <list>?" saying what the first method does
    (oneclick: "Frostmail will ask <host> to take you off the list.";
    mail: "Frostmail will send a message to <address> asking to take you
    off the list."; web: "The list's page on <host of url> will open in
    your browser."), with Cancel and Unsubscribe, which has focus;
    Escape cancels.
  - Then `message.unsubscribe {id, method}` for each method in turn
    until one succeeds; a web result's `url` opens as links do. The
    button reads "Unsubscribing…", disabled, meanwhile. Afterwards,
    `message.unsubscribeInfo` again. When every method fails, the
    banner's text ends with " Couldn't unsubscribe." until the next try.
- **More Actions** (M5, P-304, P-307, P-308): at the right of each
  message's header, a 28px button named "More Actions" (Lucide
  `ellipsis`, 16px, `--text-secondary`) opening a menu: Show All Headers
  (or Hide All Headers), Raw Source…, a separator, Save As…, Print…,
  showing their keys (Ctrl+Shift+H, Ctrl+Alt+U, Ctrl+P; M5).
  - **All headers:** the message's header section (`message.source`'s
    `headers`) in a `section` named "All Headers" under the header: a
    `pre`, 11/16 monospace
    `--text-secondary`, `white-space: pre-wrap`, 16px side padding, until
    hidden or another message is shown.
  - **Raw Source:** a sheet (the smart mailbox sheet's frame, 720 wide,
    titled "Raw Source") with the whole message (`text`) in a 12/18
    monospace `pre` 480 high that scrolls, "The message is longer than
    what is shown." under it when `truncated`, then Save As… (as the
    menu's, for the whole message) and Close. Tab moves between the two.
  - **Save As:** the save dialog (title "Save Message", default name the
    subject with `/` and `\` replaced by `-`, "(no subject)" when blank,
    plus `.eml`, filter "Email Message" `eml`), then `message.save {id,
    path}`; nothing when the dialog is cancelled. In the Flatpak the file
    chooser portal's save dialog takes the dialog's place, with the same
    title, name and filter
    ([desktop.md](../design/desktop.md#the-flatpaks-own-maild)). On a maild
    behind lodge the app downloads the message there instead
    ([design/desktop.md](../design/desktop.md#on-a-maild-behind-lodge)),
    and in lodge's web build the browser downloads it
    ([lodge-app.md](lodge-app.md#the-web-build)).
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
- **Picture attachments** (the desktop app;
  [#205](https://github.com/frostyard/frostmail/issues/205)): a PNG, JPEG,
  GIF, WebP or BMP attachment shows as a thumbnail in place of its chip,
  as Mail.app shows a picture in the message.
  - **Which:** by the part's content type (`image/png`, `image/jpeg`,
    `image/gif`, `image/webp`, `image/bmp`), or by its file name's
    extension when the type is `application/octet-stream` or empty. Never
    SVG. The part's encoded size is at most 16 MiB, and a message gets at
    most 12 thumbnails, its first; the rest stay chips.
  - **The thumbnail** fits 220 × 120, keeps the picture's shape, is never
    larger than the picture, and has the theme's radius. A JPEG shows
    upright, as its EXIF orientation says; a GIF shows its first frame. It
    is named "Preview of <filename>". Under it, the filename (truncated)
    and the size, 12px, no narrower than 96.
  - **It acts as its chip:** a click opens the file, and its menu has Open
    and Save As….
  - **Making it:** when the message opens, the app asks `message.part`
    for each such part, one after another, and scales the picture down to
    at most 440 × 240 pixels off the UI thread, one picture at a time. A
    picture whose header declares more than 40,000,000 pixels is not
    decoded. Until its thumbnail is made, and when the part cannot be
    read or decoded, the attachment is its chip. The bytes are the
    message's own, from maild (over lodge's content node when the app is
    a lodge client): nothing is fetched from anywhere else.
  - The web build shows chips only.
- **Links** open in the system browser; only `http`, `https` and `mailto`
  open. Others do nothing. `mailto` links keep only the address and the
  `to`, `cc`, `bcc`, `subject` and `body` fields, in any ASCII case, without
  decoding them; other fields and the fragment go
  ([ADR-0056](../adr/0056-filter-mailto-links-before-opening-them.md)).

### Find in the message

Ctrl+F finds in the message shown, as Mail.app's ⌘F does (P-309,
[ADR-0052](../adr/0052-find-in-the-message-shown.md)).

- **When:** the mail module's reader shows a message (one message
  selected). Ctrl+F then opens the find bar, or focuses its field and
  selects its text when it is open. With no message shown, Ctrl+F focuses
  the search field, as Ctrl+Alt+F always does (Keyboard map). The palette's
  Find in Message does the same, including focusing search with no message shown.
- **The bar** (desktop app only): a row at the top of the reader, above
  the part that scrolls, so it stays in view; 8 / 12 padding,
  `--bg-window`, a 1px `--separator` line under it. From the left: a field
  named "Find in Message" (placeholder "Find in Message", the search
  field's look, at most 280 wide); 8px; the count (12/400
  `--text-secondary`, `aria-live="polite"`): "3 of 12", "No Matches" when
  the text finds nothing, nothing while the field is empty; then the
  buttons Previous Match (`chevron-up`) and Next Match (`chevron-down`),
  disabled with no match; and at the right a button Done. The bar is not
  part of the print.
- **Finding:** in the text the reader renders, in the order it shows it:
  the headers, the banners and the bodies of the expanded messages, a
  plain-text body and an HTML frame's document alike; not a collapsed
  message's row, not text the page hides (`display: none`, the `hidden`
  attribute, a `visibility` of `hidden` or `collapse`), not text in a
  `style`, `script`, `noscript` or `template` element. Case-insensitive (a
  final sigma matches a sigma), and across inline elements ("New
  <b>York</b>" matches "new york"). Every run of whitespace matches 1
  space, in the text and in the query, and a line break ends a match: a
  `<br>`, a block's edge, and a newline in text whose white-space keeps
  newlines. Matches do not overlap. The first match after the text changes
  is the first one shown.
- **Marks:** the CSS Custom Highlight API: every match as the highlight
  `frostmail-find`, the current one as `frostmail-find-current`, in the
  app's document and in each frame's (each document keeps its own
  highlights). In the app, a match is `--flag-3` at 40% over the text and
  the current one `--flag-2` at 60%; the frame's page style has the same
  colors as literals (#ffcc00 at 40%, #ff9500 at 60%). Without the API,
  the bar counts and scrolls, and marks nothing.
- **Moving:** Enter and Next Match go to the next match, Shift+Enter and
  Previous Match to the previous one, both wrapping around. The reader
  scrolls so that the current match shows, centred when it was out of
  view. Each move and each change of the text finds the matches again, so
  a message that expanded while the bar was open joins at the next move.
- **Closing:** Escape in the bar, or Done, closes it, removes the marks,
  and focuses the reader. The bar also closes when the selection changes
  or the module changes.
- **lodge's web build:** no find bar. With a message shown, the window
  leaves Ctrl+F to the browser, whose find reaches the reader's
  content-origin frame (specs/lodge-app.md); the palette lists no Find in
  Message.

### Reply with the selected text

Reply and Reply All quote only the text the user selected in the reader,
as Mail.app does (P-508, [ADR-0053](../adr/0053-reply-quotes-the-selected-text.md);
the quote itself:
[send.md](../design/send.md#replies-and-forwards), Quoting).

- **When:** Reply or Reply All runs from the toolbar (its buttons or its
  menus), the keyboard (Ctrl+R, Ctrl+Shift+R) or the command palette. The
  list's context menu acts on its row and quotes the whole message, as
  Forward always does.
- **The selection:** the app reads it as the command runs: in an HTML
  frame's document (the same-origin `srcdoc` frame of
  [ADR-0005](../adr/0005-html-mail-rendering.md); the app reads it, and the
  frame still runs no script) or in a plain-text body in the app's
  document. It takes the selection's plain text (`Selection.toString()`),
  never its HTML, so there is nothing to sanitize. The palette takes the
  selection as it opens, because its field takes the app document's
  selection.
- **Which selection:** first, the frame that had focus when the command
  started (for a toolbar button, at the pointer press, before the button
  takes focus), when that frame holds a selection. Else, when the app's
  document holds a selection that is not white space only: that selection
  when it is inside 1 message body, and otherwise none, so the whole
  message is quoted. Else the first frame, in the reader's order, that
  holds a selection: a frame keeps its selection when focus leaves it.
- **Its message:** the expanded message whose body holds the whole
  selection. Reply and Reply All then answer that message, an older
  message of the conversation too, instead of the selected one, and send
  its text as `draft.create`'s `quote`.
- **The whole message:** with no selection, a selection of white space
  only, or a selection that is not inside 1 message body (the headers, a
  banner, a selection across 2 messages), Reply and Reply All answer the
  selected message and quote it whole, as before.
- **lodge's web build:** the reader's frame is on lodge's content origin
  and the app cannot read into it (specs/lodge-app.md), so Reply and Reply
  All always quote the whole message there, a plain-text body too.
- **An older maild:** a maild without `quote` refuses the parameter
  (`invalidParams`); the app then creates the reply again without it, so
  the reply opens with the whole message quoted.

### First run (M6)

A store with no account opens on account setup
([ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md)):
local mail is the default, and lodge is a secondary choice.

- When the session is ready and maild lists no account, the main window
  keeps its toolbar and shows, in place of its modules, a `section`
  `aria-label="Welcome"`, centered (`max-w-[420px]`, `--bg-window`):
  - an `h1` "Welcome to Frostmail" (17/22 semibold);
  - a `p` (13/18 `--text-secondary`, `mt-2`): "Add a mail account to
    start. Frostmail keeps its mail on this computer." in the desktop app
    on this computer's maild, and "Add a mail account to start." elsewhere
    (a browser, or the desktop app on lodge);
  - in a row (`mt-5`, gap 8): "Add Account…" (`PRIMARY_BUTTON`), which
    opens the settings window on Accounts (which starts adding an
    account when there is none); and, in the desktop app on this
    computer's maild only, "Connect to lodge…" (`BUTTON`), which opens it
    on the Server pane.
- **The move note** (plan 0013, Phase 7). In the release Flatpak on this
  computer's maild, when a host install's database exists
  (`~/.local/share/frostmail/frostmail.db`, which the window sees through
  `--filesystem=home:ro`), the welcome also shows, below its buttons
  (`mt-6`), a `div` `role="note"` `aria-label="Mail from another install"`
  (`--bg-banner`, 6 radius, 12px padding, 12/16 text). The shell's
  `host_mail_found` command answers whether to show it: true only in the
  Flatpak (`FLATPAK_ID`), with `FROSTMAIL_SOCKET` unset (the development
  bundle sets it, because its maild is the host's), and when that file
  exists. It reads the file's metadata only, never the file. The note
  changes nothing ([ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md),
  Moving from a host maild):
  - a `p`: "This computer also has mail from a Frostmail installed outside
    Flatpak. Nothing moves by itself. To move it here, run these commands
    in a terminal:";
  - an `ol` (`mt-2`, decimal, inside), each item a `code` in the
    monospace font, selectable:
    1. `systemctl --user disable --now maild.service`
    2. `mailctl export ~/frostmail-export`
    3. `flatpak run --command=mailctl org.frostyard.Frostmail import ~/frostmail-export`
  - a `p` (`mt-2`): "Then open Frostmail again. The folder
    ~/frostmail-export holds your passwords in plain text: delete it once
    your accounts sync.";
  - a "Dismiss" button (`BUTTON`, `mt-3`). It hides the note and keeps that
    choice in the window's `localStorage` (`frostmail.moveNote` =
    `"dismissed"`), so the note does not return on this computer.
  Elsewhere (a browser, lodge mode, outside the Flatpak) the app does not
  ask, and shows no note. An error from the command shows no note.
- In the desktop app, when the main window first renders on a connection
  and finds no account, it opens the settings window on Accounts itself.
  It decides on that first render only: once per connection, so again
  after a reconnection that still finds none, and never when the
  connection started with an account.
- When an account appears (`account.changed`), the main window shows its
  modules. Removing the last account shows the welcome again, without
  opening the settings window.

### Narrow windows (M8)

Below 600 CSS pixels wide (frostnotes' measure, a phone in lodge's frame;
`NARROW` in `app/narrow.ts` and the `narrow:` variant in `app.css`), Mail
shows one pane at a time, as Mail.app does on a phone
([plan 0011](../plans/0011-m8-mail-in-lodge.md), Phase 4). The desktop
app's window is never this narrow (its minimum is 800); a browser and
lodge's frame are.

```
┌──────────────────────────┐  ┌──────────────────────────┐  ┌──────────────────────────┐
│ Mailboxes          ⟳  ⚙ │  │ ‹ Mailboxes  Inbox     ✎ │  │ ‹ Inbox                  │  toolbar 52
├──────────────────────────┤  │ 12 messages, 3 unread    │  ├──────────────────────────┤
│ Favorites                │  ├──────────────────────────┤  │ (A) Sender         date  │
│  All Inboxes          79 │  │ ⌕ Search                 │  │     Subject              │  search 44
│  Flagged               9 │  ├──────────────────────────┤  │ body                     │
│ Account                  │  │ All Unread Flagged  Sort │  │                          │
│  Inbox                79 │  │ ● Sender           9:41  │  │                          │
│  …                       │  │   Subject…               │  ├──────────────────────────┤
│                          │  │   …                      │  │ 🗑 ▣ ↩ ↩↩ ↪ ⚑ ✉ ⇥        │  message bar 52
└──────────────────────────┘  └──────────────────────────┘  └──────────────────────────┘
       Mailboxes                       the list                     the message
```

- **Panes:** Mailboxes (the sidebar), the list, or the message, each
  the whole width. The message shows while exactly one message is
  selected; Mailboxes while asked for; the list otherwise. The list stays
  mounted, and inert, under the other two, so it keeps its scroll. The
  splitters, the sidebar toggle and the To-Do Bar go, and the sidebars'
  module bar gives way to the module tabs (below).
- **The toolbar** (52, as in a wide window) holds its pane's controls,
  as 44 × 44 targets with 20px icons:
  - Mailboxes: the title "Mailboxes" (13/600), then Get Mail and
    Settings.
  - The list: Back ("Mailboxes"), the source's title and subtitle, and
    New Message. The search field takes a row of its own under it (44
    high, the full width less 12px a side, 32 high, its text 16px so a
    phone does not zoom in on it); the scope bar shows under that while
    searching.
  - The message: Back, named by the list's title ("‹ Inbox", or
    "‹ Search").

  Back is a chevron (`chevron-left`, 24px) and its label in `--accent`,
  15/20, named "Back to …". It takes the message to its list by clearing
  the selection, and the list to Mailboxes.
- **The message bar,** at the foot of the window while a message is
  shown or the list has a selection: 52 high plus the bottom safe area,
  `--bg-toolbar`, 1px `--separator` top, with the toolbar's message
  actions in its order (Delete, Archive, Reply, Reply All, Forward, Flag,
  Mark, Move), sharing the width, each at least 44 wide and 44 high with a
  20px icon. Flag and Move open their menus above it. When the bar cannot
  hold all 8 actions at 44 wide, the actions move into » from their end,
  as in the toolbar. At 390 px all 8 fit. At 320 px Flag, Mark and Move go
  into ».
- **The module tabs,** at the foot of the window wherever the message bar
  is not (Mailboxes, the list without a selection, and every pane of
  Calendar, People and Tasks but a contact being edited, which ends with
  its own Save or Cancel): a `toolbar` named "Modules", 52 high plus the bottom safe
  area, `--bg-toolbar`, 1px `--separator` top, with Mail, Calendar,
  People and Tasks sharing the width, in module order. Each is a 44-high
  target: its module's icon (22px) over its name (10/12), in `--accent`
  for the module shown and `--text-secondary` for the others, with
  `aria-pressed`. A tap shows that module; a module keeps its own
  selection while another is shown.
- **Choosing a source** in Mailboxes shows its list, even when it is the
  source already shown.
- **History** in lodge ([lodge-app.md](lodge-app.md#the-web-build), and
  lodge-client.md's Routes, which allows it): opening a message from its
  list pushes its route, so the shell's Back returns to the list, and the
  toolbar's Back is `history.back()` then, undoing the entry mail pushed.
  A message opened by its route (a link, a reload) has no list under it:
  Back replaces its route with the list's, and never calls
  `history.back()`, whose entry may not be lodge's.
- **Rows and targets:** the sidebar's rows are 44 high. Compose's
  toolbar buttons are 44 × 44 ([compose-ui.md](compose-ui.md)), and
  Settings stacks its lists over their forms
  ([settings-ui.md](settings-ui.md)).
- **Calendar, People and Tasks** show one pane at a time too, as Mail
  does: the pane below the top stays mounted, and inert, under the one
  over it, the toolbar names what Back returns to, and a module's top
  pane has no Back (the tabs move between modules). Each module keeps its
  own selection and pane while another is shown.
  - **Calendar:** the day, under the week strip; Calendars (the month and
    the calendars, the sidebar's) over it; and the event (the event
    pane) over it once an occurrence is selected.
    - The toolbar: over the day, Back ("Calendars"), the date ("Monday,
      October 19"), Today (15/20 `--accent`, 44 high) and New Event; over
      the event, Back named by the day ("Mon, Oct 19"); over Calendars,
      the title "Calendars".
    - The week strip (a `fieldset` named "Week", 1px `--separator`
      bottom): Previous Week and Next Week, 44 × 44 at its ends, and the
      selected date's seven days between, sharing the width, each 56
      high: the weekday's initial (11/14 `--text-tertiary`) over its
      number (15/20) in a 32px circle, filled as the small month's
      selection is, with a dot when the day has occurrences.
    - The day is the day view's grid for the selected date alone,
      whatever the wide window's view. A date picked in Calendars shows
      its day, not an event picked before.
  - **People:** the list (`role="listbox"` named "Contacts", with the
    search row under the toolbar as Mail's list has); Address Books (the
    sidebar) over it; and the person (the contact pane, or its editor)
    over it. The toolbar: over the list, Back ("Address Books"), the
    book's title and count, and New Contact; over the person, Back named
    by the book ("All Contacts"), and no Back while a contact is being
    edited. A book picked shows its list, not a person picked before.
  - **Tasks:** the tasks (the task list); Lists (the sidebar) over them;
    and the task (the task pane) over them once one is selected. The
    toolbar: over the tasks, Back ("Lists"), the list's title and New
    Task; over the task, Back named by the list ("Today").
  - Their sidebars' rows are 44 high, as Mail's are.

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
- **Search coverage**
  ([ADR-0016](../adr/0016-sync-a-window-of-recent-mail.md), part of
  [#163](https://github.com/frostyard/frostmail/issues/163)): the store
  holds only each account's sync window, so under the scope bar's row a
  `p` (`px-3 pb-1 text-[12px] leading-4 text-secondary`, plain text, no
  `role`) says how far the search looked. The searched accounts are the
  account of the current mailbox when the scope is the current mailbox and
  the source is a mailbox, else every account. A window's first day is
  maild's: the UTC date `syncDays` days before now
  (`internal/mailsync/window.go`), in the long date form
  (`dateStyle: "long"`, in UTC, the app's locale). When every searched
  account with `syncDays > 0` shares one window and no searched account
  keeps every message, the line reads "Searched mail since August 7, 2025
  on this computer." Otherwise it names each windowed account (its display
  name, or its email when the name is empty), in the accounts' order:
  "Searched mail on this computer. Gmail: since August 7, 2025; Work:
  since November 9, 2025." An account that keeps every message is not
  named. No line when no searched account has a window, and none without
  a search (the scope bar shows only during one).
- **Context menu** on list rows (right click or the Menu key), in this
  order, with separators between the groups:
  - Reply, Reply All, Forward, Forward as Attachment and Redirect… (M5,
    P-402, P-403), on the last message of the menu's set. Forward as
    Attachment opens a draft of kind `attached` as Forward opens its
    draft. Redirect… (Ctrl+Shift+E) opens the Redirect sheet (below).
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
    Mark as Unread, and Mute (below): the reminder choices (`remindChoices`: Remind Me in 1
    Hour; Remind Me Tonight, at 9:00 PM, before then; Remind Me
    Tomorrow, at 8:00 AM), a separator, Remind Me Later… (the time sheet
    titled "Remind Me", starting tomorrow at 8:00 AM), and, when a message
    of the menu's set has a pending reminder, Clear Reminder. Each calls
    `message.remind` on the menu's messages, with the time or, for Clear
    Reminder, without.
  - Mute (M5, P-410), between Mark as Read or Mark as Unread and Remind
    Me ▸: `message.mute {ids, muted: true}` on the menu's messages; it
    reads Unmute, and sends `muted: false`, when every message of the
    menu's set is muted. A muted conversation's new mail never notifies.
  - Apply Rules (M5), only when a rule is enabled: `rule.apply` on the
    menu's messages, which runs the enabled rules on them now, wherever
    they are. It has no shortcut: Mail.app's Option-Command-L would be
    Ctrl+Alt+L, which locks the screen on most Linux desktops.

  Every item but Reply, Reply All, Forward and Forward as Attachment is
  disabled when the account is read-only. The menu acts on the selection when the clicked
  row is part of it, otherwise on the clicked row alone, which it selects.
- **Redirect sheet** (M5, P-403): a dialog (the time sheet's frame, 420
  wide) titled "Redirect", the message's subject under the title (12/16
  `--text-secondary`, "(no subject)" when blank), a To recipient field as
  compose's (suggestions from `address.suggest`; it has focus), and
  Cancel and Redirect. Redirect is disabled until there is at least one
  address and every address is valid, and while it is sending; it calls
  `message.redirect {id, to}` and closes, or shows the error's message
  under the field (`role="alert"`, 12/16) and stays. The message then
  waits in the outbox with the undo delay, as a send does. Escape
  cancels.
- **Undo** (M5, P-406, ADR-0028): Ctrl+Z takes back the newest of the
  main window's last 20 moves, deletes, archives, spam moves, and flag,
  flag color and read changes, made from the list, its menus and row
  actions, the toolbar, drag and drop or the keyboard. Opening a message,
  which marks it read, and copies are not undone; the list is gone when
  the window closes.
  - **Remembering a move or delete:** `message.summaries` of the messages
    before and after it. For each message still there afterwards, the
    mailboxes it left and the ones it entered. Taking it back: with one
    entered, `message.move {mailboxId: the first it left, fromMailboxId:
    the one entered}`, then `message.copy` to any other it left; with
    none entered (a Gmail archive), `message.copy` to each it left. A
    message gone afterwards (deleted from the Trash) cannot come back.
  - **Remembering a flag change:** `message.summaries` before it. Taking
    it back: `message.setFlags` with each message's earlier `seen`, or
    earlier `flagColor` (0 when it was not flagged), for what the change
    set.
  - Calls with the same method and params but the messages are made once,
    with the messages together. An action that changed nothing is not
    remembered, and Ctrl+Z with nothing to undo does nothing.
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

### Command palette

Ctrl+Shift+P opens the command palette: a field that finds a command of
the [Keyboard map](#keyboard-map) by name, shows its shortcut, and runs
it as its key does. Mail.app's nearest is the Help menu's search, which
finds a menu command by name (parity P-958).

- **Where:** the main window, anywhere, a text field included (it is a
  Ctrl+Shift key). It does not open while a sheet or a window in lodge's
  frame has the keys, or while a contact is being edited. The compose
  window has none: Ctrl+K stays Link there ([compose-ui.md](compose-ui.md)).
- **The commands** are `PALETTE` in `app/src/lib/keymap.ts`, beside the
  `Command` union and typed `Record<Command, …>`, so a new command with no
  entry fails `tsc`. An entry names the command in each module where it
  acts, and says what it needs. The palette lists the current module's
  commands in `PALETTE`'s order:

  | Command | Name | Modules | Needs |
  | --- | --- | --- | --- |
  | compose | New Message, New Event, New Contact, New Task | each | |
  | reply, replyAll, forward | Reply, Reply All, Forward | Mail | a selection |
  | redirect | Redirect… | Mail | a selection |
  | archive | Archive | Mail | a selection |
  | delete | Delete | Mail, Calendar, Tasks | a selection |
  | junk | Mark as Spam/Not Spam | Mail | a selection |
  | toggleRead | Mark as Read/Unread | Mail | a selection |
  | toggleFlag | Flag/Unflag | Mail | a selection |
  | undo | Undo | Mail, Calendar, People | |
  | selectAll | Select All | Mail | |
  | getMail | Get Mail | Mail | |
  | focusSearch | Search | Mail, People | |
  | find | Find in Message | Mail (not the web build) | — (focuses search without a message) |
  | print | Print… | Mail | one message |
  | rawSource | Raw Source… | Mail | one message |
  | allHeaders | Show/Hide All Headers | Mail | one message |
  | today | Go to Today | Calendar | |
  | dayView, weekView, monthView | Day View, Week View, Month View | Calendar | |
  | previousPeriod, nextPeriod | Previous Period, Next Period | Calendar | |
  | allInboxes | All Inboxes | each | |
  | showCalendar, showPeople, showTasks | Show Calendar, Show People, Show Tasks | each | |
  | toggleSidebar | Show/Hide Sidebar | each | |
  | settings | Settings… | each | |

  The moves have no entry (`null`): previous and next, extend, first and
  last, page, left and right, the panes, Enter's open, Escape, the context
  menu, and the palette itself.
- **The shortcut** of a row comes from the keymap's bindings, written as
  this map writes keys: Ctrl, then Alt, then Shift, then the key, a letter
  in capitals, the arrows as ← and →. A command with two keys shows both
  ("Delete, Backspace").
- **The sheet:** lodge-ui's `Sheet` named "Commands" (480 wide, no heading
  and no close button), with a field named "Command" (placeholder "Find a
  Command"), which has focus, and under it a listbox named "Commands" of at
  most 360 high, which scrolls. Each row is an option: the name on the
  left and the shortcut on the right (12/16 `--text-secondary`). The
  highlighted row is `--selection-inactive`, and the field names it
  (`aria-activedescendant`). With no match, the list shows "No Commands".
- **Finding:** case-insensitive; each word typed must start a word of the
  name, where words are split at anything but a letter or a digit ("arch"
  finds Archive, "new m" New Message, "unr" Mark as Read/Unread). An empty
  field lists every command. A change of the text highlights the first
  row.
- **Keys:** ↑ and ↓ move the highlight and stop at the ends; Enter runs the
  highlighted row and closes; Escape, or a click outside, closes. A click
  on a row runs it. Focus returns where it was before the command runs, so
  a command that moves focus (Search) keeps it there.
- **Disabled rows:** a command that needs what is missing is shown
  disabled (`aria-disabled`, at 40% opacity) with its reason under its
  name (12/16 `--text-secondary`): "Select a message first" (Mail),
  "Select an event first" (Calendar), "Select a task first" (Tasks); and
  "Select one message first" for a command on one message. Enter or a
  click on it does nothing, and the palette stays open.
- **Running** calls the dispatch the key calls (`runCommand` in
  `app/src/app/MainWindow.tsx`), so a command does what its key does in the
  pane that had focus, and, as with its key, may do nothing there
  (Redirect… on a read-only account, Select All outside the list).
- **The web build:** Firefox keeps Ctrl+Shift+P (a private window), and a
  page cannot take it. So the web build's toolbar has a Commands button
  (`keyboard`, named "Commands", its tooltip Ctrl+Shift+P) just before
  Settings, which opens the palette. The desktop app's toolbar has none,
  and neither has a narrow window's.
- **lodge's keys:** lodge's shell answers its switcher shortcut (Ctrl+\`)
  and its companion chord (Ctrl+Shift+\`). lodge-client catches them in the
  app's frame and stops them, so the app never sees them (lodge's
  `docs/specs/shell.md`, The switcher and The companion). Neither is
  Ctrl+Shift+P, and the frame passes Ctrl+Shift+P to the app. A user may
  set either to Ctrl+Shift+P, as lodge does not rule it out: then lodge's
  wins, and the Commands button still opens the palette.

### Keyboard map

In the native mail list, Delete holds each selected row until the list
reflects the operation. If the connection ends before maild answers, an
unchanged read on the next connection does not release that hold: the old
transaction may still commit. Reads retry until they show the message
moved or erased; a view delta dropping the loaded row also releases it.
An answered Delete that leaves the message unchanged can release its
hold, since maild has finished that request.

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
| Ctrl+Alt+F | focus the search field | anywhere |
| Ctrl+F | [find in the message](#find-in-the-message); with no message shown, focus the search field | anywhere |
| Enter / Shift+Enter | next / previous match | the find bar |
| Escape | clear search, close a menu, close the find bar | search field, menus, the find bar |
| Ctrl+Shift+N | get new mail | anywhere |
| Ctrl+1 | All Inboxes | anywhere |
| Ctrl+Alt+S | toggle sidebar | anywhere |
| Tab / Shift+Tab | next / previous pane | anywhere |
| Enter, double-click | open a message in Drafts as a draft | list |
| Ctrl+N | new message | anywhere but a text field |
| Ctrl+R / Ctrl+Shift+R | reply / reply all to the selection | Ctrl+R: not in a text field |
| Ctrl+Shift+F | forward the selection | anywhere |
| Ctrl+, | open the settings window ([settings-ui.md](settings-ui.md)) | anywhere |
| Ctrl+Shift+P | the [command palette](#command-palette) | anywhere |
| Ctrl+K | the command palette (native app; plan 0015, shared rule 1) | anywhere but a text editor |
| Ctrl+J | show or hide Activity (native app) | anywhere |
| Ctrl+Z | undo the newest move, delete or flag change (M5) | anywhere but a text field |
| Ctrl+Shift+E | Redirect… the selection's last message (M5) | anywhere with a selection, writable account |
| Ctrl+P | print the message shown (M5) | anywhere with one message selected |
| Ctrl+Alt+U | Raw Source… of the message selected (M5) | anywhere with one message selected |
| Ctrl+Shift+H | show or hide all headers of the message selected (M5) | anywhere with one message selected |

The compose window's own keys are in [compose-ui.md](compose-ui.md).

## Rules

- No component draws literal colors or fonts; tokens only.
- No view of messages is built in the app except through `view.open`.
- The reader frame never has `allow-scripts`; the app never fetches remote
  content (ADR-0005).
- Every interactive element is reachable by keyboard and has an accessible
  name.
- When the system asks for reduced motion (`prefers-reduced-motion:
  reduce`), no animation or transition runs (`app/src/styles/app.css`).

## References

- Rationale: [ADR-0009](../adr/0009-ui-architecture.md),
  [ADR-0005](../adr/0005-html-mail-rendering.md),
  [ADR-0038](../adr/0038-windows-as-a-second-platform.md)
- Context: [design/app.md](../design/app.md)
