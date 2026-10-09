# Spec: people, calendar and tasks in the main window (M4.5)

The look and behavior of the main window's modules (ADR-0020): the module
bar, the People module, the contact card that mail shows for an address,
and, as their phases start, the Calendar and Tasks modules, the To-Do bar,
the invitation card and the reminder window. It binds the presentational
components in `app/src/features/{people,calendar,tasks}/` and
`features/sidebar/ModuleBar.tsx`, the containers in `app/src/app/` that
wire them to maild, and the app tests. Tokens, type, measurements,
selection colors and the conventions for buttons are those of the reader
UI ([ui.md](ui.md)); Mail's own panes are unchanged.

## Modules

- The window has four modules, in this order: **Mail**, **Calendar**,
  **People**, **Tasks**. One is shown at a time; the window opens on Mail.
  The current module is window state (`useUI().module`), not persisted.
- Each module keeps its own sidebar, list and detail panes in the main
  window's three columns, with the same widths and splitters as Mail, and
  its own selection; switching back to a module finds it as it was left.
- **Keys:** Ctrl+1 shows Mail and selects All Inboxes (as before); Ctrl+2
  Calendar, Ctrl+3 People, Ctrl+4 Tasks. They work anywhere, text fields
  included, like Ctrl+1 today. A module that is not built yet ignores its
  key.

### Module bar

```
├──────────────┤
│ ✉  ▦  👥  ☑  │  44
└──────────────┘
```

- At the bottom of every module's sidebar: 44 high, a 1px `--separator`
  top border, `--bg-sidebar`. A `toolbar` named "Modules" with one
  `button` per built module, left to right in module order, starting at
  12px with 4px gaps.
- A button is 36 × 28, rounded 6, an 18px Lucide icon: Mail `Mail`,
  Calendar `Calendar`, People `Users`, Tasks `ListChecks`. Its accessible
  name is the module's name; its tooltip is the name and its key ("People
  (Ctrl+3)"). The current module's button has `aria-pressed="true"` and
  its icon in `--accent`; the others are `--text-secondary` with the
  toolbar buttons' hover.
- When the sidebar is hidden (Ctrl+Alt+S) the bar hides with it; the keys
  still switch.

## People module

```
┌──────────────┬───────────────────────┬──────────────────────────────────┐
│ ▢            │ All Contacts     ✉    │                  ⌕ Search Contacts – □ ✕ │
├──────────────┼───────────────────────┼──────────────────────────────────┤
│ People       │ A                     │ (AL) Ada Lovelace                │
│  All Contacts│ (AL) Ada Lovelace     │      Countess · Engines          │
│ Gmail        │      Engines          │      [✉ Message]                 │
│  Contacts    │ (AT) Alan Turing      │ CONTACTS — GMAIL                 │
│ iCloud       │      alan@…           │      home  ada@example.com       │
│  Card 🔒     │ G                     │    mobile  +44 20 7946 0000      │
│              │ (GH) Grace Hopper     │ RECENT MAIL                      │
│ ✉ ▦ 👥 ☑     │                       │  Engines                  Oct 1  │
└──────────────┴───────────────────────┴──────────────────────────────────┘
```

### People sidebar (`PeopleSidebar`)

- Background, padding, sections, rows, icons and selection exactly as
  Mail's sidebar (ui.md, Sidebar), as a `tree` named "Address Books".
- **Sections:** "People" holds one row, **All Contacts** (icon `Users`,
  key `all`). Then one section per account that has address books, titled
  with the account's email address, as Mail's sidebar titles accounts,
  holding a row per address book (icon `BookUser`,
  key `book:<id>`), in collection order. A read-only book shows a 12px
  `Lock` icon after its name (`aria-label="Read-only"`). Sections collapse
  as Mail's do.
- Selecting a row selects the list's source: every person, or the people
  with a contact in that book (`people.list` with `collectionId`).
- The module bar follows the tree.

### People list (`PeopleList`)

- A `listbox` named "Contacts", virtualized, in `--bg-window`.
- **Index headers:** before the first person of each letter, a 24px
  header (`role="presentation"`), sticky at the top while its letter
  scrolls, `--bg-sidebar`, 12px left padding, 11/14 600
  `--text-secondary`, showing the person's `index` (A–Z, or # for
  anything else).
- **Rows** (`role="option"`, `aria-selected`) are 44 high: 12px left
  padding, a 28px avatar circle, 10px gap, then two lines: the name 13/18
  600 truncated (the email when the name is empty); below it the
  organization, else the email, 12/16 `--text-secondary` truncated, left
  out when it would repeat the first line. A 1px `--separator` at the bottom inset 50
  from the left.
- **Avatar:** initials (as the reader's, from the name, else the email)
  13/600 white on `--avatar-N` by `avatarTone(email)`; the list shows no
  photos.
- **Selection, focus and keys** as the message list: click selects; ↑/↓,
  Home/End move it; the selection is `--accent` with `--accent-contrast`
  text while the list has focus, `--selection-inactive` otherwise. There is
  no multiple selection.
- **Empty:** "No Contacts" centered, 15/400 `--text-secondary`; while
  searching, "No Results".

### Person pane (`PersonPane`)

- Background `--bg-window`, 24px padding, scrolling. **Empty:** "No
  Contact Selected" centered, 15/400 `--text-secondary`.
- **Header:** a 64px avatar (the person's photo, `object-fit: cover`, when
  there is one; else initials as in the list at 20/600); 16px gap; the name
  20/26 600 (an `h2`); below it the first contact's title and the person's
  organization joined by " · " (13/18 `--text-secondary`, omitted when
  both are empty). Under them a **Message** button (`Mail` icon and label,
  28 high, `--accent` fill, `--accent-contrast` text, rounded 6) that
  starts a new message to the person's first email; disabled when the
  person has none.
- **Contacts:** one block per contact, in the person's order, 20px apart:
  a `section` labeled by its heading, an `h3` with the address book's name
  and the account's email address (as Mail's sidebar names accounts)
  joined by " — ", plus " · Read-only" for a
  read-only contact, shown uppercase (CSS `uppercase`), 11/14 600
  `--text-secondary`. Then a two-column grid (`dl`): labels right-aligned
  in a 96px column, 12/18 `--text-secondary`; values 13/18
  `--text-primary`, 12px column gap, 6px row gap. Rows, each omitted when
  empty, in this order:
  - each email: label as given, else "email"; the value is a link-styled
    `button` (`--accent`) that starts a new message to it;
  - each phone: label as given, else "phone"; plain selectable text;
  - each address: label as given, else "address"; street, then
    "locality region postcode" (single spaces, empty parts skipped), then
    country, each non-empty line its own `div`;
  - each URL: label as given, else "homepage"; an `http` or `https` URL
    is a link that opens in the system browser (the click is the app's,
    never the webview's); any other URL is plain text;
  - birthday: label "birthday"; `YYYY-MM-DD` as the locale's long date
    ("December 10, 1815"), `--MM-DD` as month and day ("December 9");
  - nickname: label "nickname";
  - note: label "note"; `white-space: pre-wrap`.
- **Recent Mail:** when the contact card of the person's first email has
  recent messages, a `section` labeled "Recent Mail" with that heading (as
  the blocks' headings) and a row per message, 32 high: the subject 13/18 truncated ("(No Subject)" in
  `--text-tertiary` when empty), and at the right the list's date format
  (ui.md, Message list: Dates) 12/16 `--text-secondary`. A row is a
  `button`; clicking it shows the message in Mail (Mail module, the
  message's mailbox, the message selected).
- **Upcoming:** with the Calendar module, the card's upcoming events the
  same way ("UPCOMING"); hidden while empty.

### People toolbar

The main toolbar keeps its three segments and its right end (search field,
Settings, window controls); `Toolbar`'s `mode` is `"people"`:

- over the sidebar: the sidebar toggle, without Get Mail;
- over the list: the title ("All Contacts", or the book's name), the
  subtitle ("1 contact", "N contacts", or "N results" while searching), and
  **New Message** as in Mail, addressed to the selected person's first
  email, or blank without a selection;
- over the person pane: none of Mail's message actions.
- The search field's placeholder is "Search Contacts"; it filters the list
  (`people.list` with `query`) 250 ms after typing stops; Escape clears it.
  Mail's scope bar does not appear.

### Behavior

- **Data:** the list comes from `people.list` (with the book and the
  query); the pane from `people.get` for the selected person, `people.card`
  of their first email for Recent Mail, and `people.photo` when
  `hasPhoto`. A `people.changed` event refetches the list and the selected
  person within 100 ms; `account.changed` refetches the address books.
- **Selection** follows the person's ID across refetches; when the person
  is gone, nothing is selected.
- **Focus:** Tab and Shift+Tab move between sidebar, list and pane as in
  Mail. Ctrl+F (and Ctrl+Alt+F) focus the search field. Ctrl+N starts a
  new message (to the selected person's first email when the list has
  focus).

## Contact card

A popover that mail shows for an address, the way Outlook's contact card
does.

- **Where:** the reader's sender name and each name in its To and Cc lines
  is a button (named by the address); clicking it opens the card anchored
  below it. Escape, a click outside, or opening another closes it.
- **Box:** 320 wide, `--bg-window`, 1px `--separator` border, rounded 10,
  a soft shadow, 16px padding, `role="dialog"` named by the card's name or
  address.
- **Header:** a 48px avatar (the person's photo, else initials on the
  address's tone), the name 15/20 600 (the card's name, else the address),
  the address 12/16 `--text-secondary`, and the person's organization when
  there is one.
- **Actions** (a row of buttons, 28 high): **Message** (new message to the
  address, closing the card); **Add to Contacts** when `canAdd` (calls `people.add` and shows
  "Added" in place of the button when it succeeds, or the error under the
  row); **Open in People** when there is a person (switches to People with
  them selected).
- **Recent Mail** and **Upcoming** as in the person pane, at most 5 rows
  each; a recent message opens in Mail and closes the card.
- The card asks `people.card` each time it opens; while the answer is
  pending it shows the name and address it was opened with.

## Calendar module

Day, week and month views over `calendar.range`, Apple Calendar's look
with Outlook's keys. Phase 3 shows events; creating and editing them is a
later event editor window (ADR-0020), and answering invitations is Phase
4's.

```
┌──────────────┬──────────────────────────────────────────┬─────────────────┐
│ ▢            │ Today ‹ ›  October 2026   [Day|Week|Month]│     ⚙  – □ ✕    │
├──────────────┼──────────────────────────────────────────┼─────────────────┤
│ ‹ October 2026 ›│      Mon 5   Tue 6   Wed 7  (8)Thu  …  │ Standup         │
│ M T W T F S S │ all-day        [Holiday──────────]        │ Room 4          │
│ 28 29 30  1 … │  9 AM  ┃Standup ┃Standup                │ Thursday, Oct 8 │
│ …             │ 10 AM          ┃Review                  │ 9:00 – 9:15 AM  │
│ USER@DAV.TEST │ ──●──── now                               │ Repeats  Every  │
│ ■ Work        │ 11 AM                                     │   day, 7 times  │
│ □ Home 🔒     │                                           │ ● Work          │
│ ✉ ▦ 👥 ☑      │                                           │ Maria (organizer)│
└──────────────┴──────────────────────────────────────────┴─────────────────┘
```

- **Panes:** the sidebar (Mail's width and splitter), the view (the rest),
  and the event pane, 320 wide at the right with a 1px `--separator` left
  border and no splitter.
- **Dates** are `YYYY-MM-DD` strings in the app's time zone
  (`Intl.DateTimeFormat().resolvedOptions().timeZone`), which every
  `calendar.range` call names. Weeks start on the locale's first day
  (`Intl.Locale(navigator.language)`'s week info; Sunday when the browser
  has none); components take it as `weekStart` (0 is Sunday). Times and
  dates are formatted with `Intl.DateTimeFormat` in the locale; tests pass
  `en-US`.
- **Calendar colors** are the collection's `color`, else `var(--accent)`:
  data applied inline, like photos. An event's fill is its color mixed 18%
  into `--bg-window` (`color-mix(in srgb, <color> 18%, var(--bg-window))`)
  with a 3px left bar in the color; selected, the fill is the color itself
  and its text `--accent-contrast`.
- **What shows:** occurrences the user declined (`answer` `declined`) are
  left out. Cancelled ones (`status` `cancelled`) show at 60% opacity with
  their title struck through. Unanswered invitations (`needsaction`) and
  tentative answers have a 1px dashed border in the color and a
  `--bg-window` fill.

### Calendar sidebar (`CalendarSidebar`)

- **Small month (`MiniMonth`)** at the top, 12px padding: a header with
  the month and year 13/16 600 and ‹ › buttons (24 × 24, 14px
  `ChevronLeft`/`ChevronRight`, named "Previous Month"/"Next Month"), then
  a `grid` named by the month and year: a row of narrow weekday names
  (11/14 `--text-tertiary`) and six weeks of day buttons, 24 × 24 rounded
  full, 12/16, named by the full date ("Thursday, October 8, 2026").
  Days of other months are `--text-tertiary`; today has
  `aria-current="date"` and `--accent` 600 text; the selected date has
  `aria-pressed="true"` and a `--selection-sidebar` circle, or an
  `--accent` circle with `--accent-contrast` text when it is today. A day
  with occurrences shows a 4px `--text-tertiary` dot under its number.
  Clicking a day selects it; ‹ › page the small month without changing the
  selection, and it returns to the selected date's month when the
  selection changes.
- **Calendars:** one section per account with calendars, titled with its
  email as Mail's sidebar titles accounts, holding a row per calendar in
  collection order: 28 high, 12px left padding, a 14px rounded-3 square
  (filled with the color when shown, a 1.5px border in the color when
  hidden), 8px gap, the name 13/16 truncated, and a 12px `Lock`
  (`aria-label="Read-only"`) for a read-only calendar. A row is a
  `checkbox` (`aria-checked` = `enabled`) named by the calendar; clicking
  it or Space toggles it (`account.setCollection` with `enabled`: hidden
  calendars are neither synced nor shown).
- The module bar follows.

### Day and week views (`TimeGrid`)

- **Header** (48 high, 1px `--separator` bottom): a 56px gutter, then a
  column per day: the short weekday 11/14 `--text-secondary` and the day
  number 15/20 (today's in a 24px `--accent` circle with
  `--accent-contrast` text). A day's header is a button named by the full
  date; clicking it shows that day in the day view.
- **All-day strip** under it: a row of lanes, 22 high each, 2px apart,
  holding all-day occurrences as bars across the days they cover within the
  shown days (`allDayLanes`); the gutter says "all-day" 11/14
  `--text-tertiary`. The week view shows at most 3 lanes; a day with more
  shows "N more" in the last lane, a button that opens that day in the day
  view. The day view shows every lane. The strip is 8 high when empty.
- **Grid:** 24 hours, 48px each, scrolling vertically under the fixed
  header and strip. The gutter labels each hour but midnight ("9 AM",
  11/14 `--text-tertiary`, right-aligned 8px from the grid, centered on
  the line); 1px `--separator` lines at each hour and between days.
  Opening a view scrolls 7:00 to 12px below the top, so its label shows
  whole.
- **Timed occurrences** are blocks in their day's column (split at midnight
  when they cross it), placed by `timedLayout`: top at the start's minutes
  × 0.8px, height the duration likewise but at least 18px, left and width
  by column within the column's width less 4px on the right, rounded 4.
  Inside, 4px padding: the title 12/15 600 truncated; when the block is at
  least 36 high, the time range and location 11/14 `--text-secondary`.
  A block is a `button` named "<title>, <time range>[, <location>]".
- **Now:** today's column has a 2px `--flag-1` line at the current time
  with an 8px dot at its left end, and the gutter shows the time 11/14 600
  `--flag-1`. It moves every minute.

### Month view (`MonthGrid`)

- A row of short weekday names (24 high, 11/14 `--text-secondary`,
  centered), then six weeks of seven cells filling the view; 1px
  `--separator` lines between cells.
- **Cells:** the day number 12/16 at the top right, 4px in (the 1st of a
  month adds the short month: "Oct 1"); other months' days in
  `--text-tertiary`; today's number in a 20px `--accent` circle with
  `--accent-contrast` text. Then one 18-high line per occurrence of the day
  (`monthItems`): all-day ones first as filled pills (the fill rule above,
  11/14 600 title), then timed ones as a 6px dot in the color, the title
  11/14 truncated, and the start time 11/14 `--text-secondary` at the
  right, left out in cells narrower than 9rem (a container query). A cell shows
  at most `lines` lines (the container fits them to the cell's height, at
  least 2): when the day has more, the last line is "N more"
  (`--text-secondary`), a button that opens the day in the day view.
- Clicking a cell's empty space selects its date; double-clicking shows it
  in the day view.

### Event pane (`EventPane`)

- 20px padding, scrolling. Without a selection: "No Event Selected"
  centered, 13/16 `--text-tertiary`.
- **Title** 17/22 600 ("No Title" when empty), the location under it
  13/16 `--text-secondary`, and "Cancelled" 12/16 600 `--flag-1` when it
  is.
- **When:** the date line 13/18 ("Thursday, October 8, 2026"; an all-day
  event over several days "October 12 – 14, 2026") and the time range
  ("9:00 – 9:15 AM"; "All day"). When the event's `timeZone` differs from
  the app's, a line 12/16 `--text-tertiary` with that zone and the times
  there ("Europe/Berlin: 9:00 – 9:15 AM").
- **Rows** (label 12/16 `--text-secondary` 80 wide, value 13/18):
  **Repeats** (`describeRecurrence`), **Calendar** (an 8px dot in its
  color, its name, " · " and the account's email `--text-tertiary`),
  **Alerts** (`alarmText` per alarm, one per line), **Your answer**
  ("Accepted", "Declined", "Maybe", "Not answered"; only when invited).
  Rows without a value are left out.
- **People:** "Organizer", then "Invitees" (11/14 600 `--text-secondary`
  headings), a 36-high row per person: a 28px avatar (initials on the
  address's tone), the name or else the address 13/16, " (you)" when it is
  the user, " (optional)" `--text-tertiary` for optional ones, and the
  answer at the right: a 14px `Check` in `--flag-4` (accepted), `X` in
  `--flag-1` (declined), `CircleHelp` in `--flag-2` (tentative), nothing
  for no answer; the icon's `aria-label` is the answer. A person's name is
  a button that opens the contact card.
- **Notes:** the description 13/18, line breaks kept, links not made.

### Calendar toolbar

`Toolbar`'s `mode` is `"calendar"`; its middle segment fills the view's
width and its right segment the event pane's.

- Over the sidebar: the sidebar toggle.
- Over the view: **Today** (a text button 28 high, 10px padding), ‹ ›
  (28 × 28, named "Previous Day/Week/Month" and "Next …"), the title
  15/20 600 ("Thursday, October 8, 2026"; a week "October 2026", or
  "Sep – Oct 2026" across months, or across years "Dec 2026 – Jan 2027";
  a month "October 2026"), and at the right a `radiogroup` named "View"
  of Day, Week and Month (a 28-high segmented control: a `--badge-bg`
  track, the chosen segment `--bg-window` with a soft shadow).
- Over the event pane: Settings and the window controls; no search field
  in Phase 3.

### Calendar behavior

- **State** (`useUI`): `calendarView` (`day`, `week` or `month`; first
  `week`), `calendarDate` (the selected date; first today), the selected
  occurrence (`eventId` and `recurrenceId`, or none), and the focused pane.
  Window state, not persisted.
- **Keys:** Ctrl+Alt+1, Ctrl+Alt+2 and Ctrl+Alt+3 show Day, Week and Month;
  Ctrl+T selects today; Ctrl+← and Ctrl+→ page the view by its period;
  with the view focused, ← → move the selected date by a day and ↑ ↓ by a
  week in the month view (in the day and week views they scroll the grid
  by an hour); Enter shows the selected date's day; Escape clears the
  selected occurrence. Tab and Shift+Tab move between sidebar, view and
  pane.
- **Data:** the view's days come from `calendar.range` (the day; the
  week; the month view's 42 days), the small month's dots from another over
  its 42 days; `account.collections` lists the calendars and `account.list`
  names their sections. A `calendar.changed` event refetches the ranges and
  the selected event within 100 ms; `account.changed` refetches the
  calendars. The pane shows `calendar.event` for the selection.
- **Selection** follows the occurrence's `eventId` and `recurrenceId`
  across refetches; when it is gone the pane is empty. Clicking an
  occurrence selects it and its date; paging keeps the selection when it
  is still shown.
- **From elsewhere:** an Upcoming row on a contact card or the person
  pane switches to Calendar, selects the occurrence's date and the
  occurrence, and keeps the current view.

## Tasks module

Task lists from Google Tasks and CalDAV, with subtasks, due dates and
completion, and flagged mail as a list of its own: Apple Reminders' rows
in Mail.app's panes, with Outlook's keys.

```
┌──────────────┬──────────────────────────────────────────┬─────────────────┐
│ ▢            │ +  Work            [Show Completed]      │     ⚙  – □ ✕    │
├──────────────┼──────────────────────────────────────────┼─────────────────┤
│ Tasks        │ +  New Task                               │ [Report       ] │
│  ☀ Today   2 │ ○  Report                                 │ Due   10/10/2026 ✕│
│  ☰ All     5 │    Tomorrow · Q3 numbers                  │ List  Work ·    │
│  ⚑ Flagged 3 │      ○  Charts                            │       user@…    │
│ USER@GMAIL   │ ○  Groceries                         ✉    │ Notes           │
│  ☰ Work    3 │    Yesterday                              │ [Q3 numbers   ] │
│  ☰ Home 🔒 1 │                                           │                 │
│ ✉ ▦ 👥 ☑     │                                           │ Delete Task     │
└──────────────┴──────────────────────────────────────────┴─────────────────┘
```

- **Panes:** the sidebar (Mail's width and splitter), the task list (the
  rest), and the task pane, 320 wide at the right with a 1px
  `--separator` left border and no splitter, as Calendar's.
- **Sources:** Today (open tasks due today or earlier, in every list),
  All Tasks (every open task, list by list), Flagged Mail (flagged
  messages), and each task list. A source is `"today"`, `"all"`,
  `"flagged"` or a list's ID (`TasksSource`, `lib/taskText.ts`).
- **Dates:** `due` is a `YYYY-MM-DD` string compared with today in the
  app's time zone, as Calendar's dates are.

### Tasks sidebar (`TasksSidebar`)

- As People's sidebar: a `tree` named "Task Lists" with Mail's rows,
  selection and section titles; sections collapse.
- **Sections:** "Tasks" holds **Today** (icon `Sun`, key `today`), **All
  Tasks** (`ListChecks`, key `all`) and **Flagged Mail** (`Flag`, key
  `flagged`). Then one section per account with task lists, titled with
  its email, holding a row per list (`List`, key `list:<id>`) in
  collection order; a read-only list shows the 12px `Lock`
  (`aria-label="Read-only"`).
- **Counts** at the right of a row, 12/16 `--text-secondary`
  (`--accent-contrast` on the focused selection): Today's and a list's
  open tasks, All Tasks' every open task, Flagged Mail's flagged
  messages. None when 0.
- ↑ ↓ Home End move the selection within the tree's shown rows. The
  module bar follows.

### Task list (`TaskList`)

- A `listbox` named by the source ("Today", "All Tasks", "Flagged Mail"
  or the list's name), `--bg-window`, scrolling; not virtualized.
- **New Task field** at the top, for every source but Flagged Mail and a
  read-only list: 40 high, a 1px `--separator` bottom, a 16px `Plus`
  in `--text-tertiary` where the check circles are, then a borderless
  input 13/18 named "New Task" with that placeholder. Enter with text
  creates the task (the title trimmed): in the selected list; for Today
  and All Tasks in the default list, due today for Today. The field
  empties and keeps focus. Escape empties it and focuses the list.
- **List headers** in Today and All Tasks: before each list's first
  task, 28 high, 12px left padding, the list's name 11/14 600
  `--text-secondary` (`role="presentation"`).
- **Task rows** (`role="option"`, `aria-selected`), at least 36 high,
  8px vertical padding, 12px left padding plus 28px for a subtask whose
  parent is shown above it, and a 1px `--separator` bottom inset 40:
  - the **check circle**, 18px: a `checkbox` named "Completed"
    (`aria-checked`), a 1.5px `--text-tertiary` ring; checked, an
    `--accent` disc with a 12px `Check` in `--accent-contrast`. Clicking
    it toggles completion without selecting the row. Disabled in a
    read-only list.
  - 10px gap, the title 13/18 (`--text-tertiary` when completed), and
    under it, when there is either, a line 12/16 `--text-secondary`
    truncated: the due text (`dueText`; in `--flag-1` when overdue) and
    " · " the first line of the notes.
  - a 14px `Mail` icon at the right (`aria-label="From mail"`) for a
    task made from a message.
- **Completed tasks** show only with Show Completed (a list or All
  Tasks), and a task ticked here stays, ticked, until the source changes.
- **Flagged Mail rows:** the check circle (ticking clears the flag; the
  row leaves), the subject 13/18 ("No Subject"), under it the sender's
  name or address, " · " and the date (`formatListDate`) 12/16
  `--text-secondary`, and a 14px filled `Flag` in the flag's color at
  the right. The view's first 200 rows.
- **Selection** as People's list: click selects; the selection is
  `--accent` with `--accent-contrast` text (and ring) while the list has
  focus, `--selection-inactive` otherwise. No multiple selection.
- **Empty:** "No Tasks" ("No Flagged Mail") centered, 15/400
  `--text-secondary`, under the New Task field.

### Task pane (`TaskPane`)

- `--bg-window`, 20px padding, scrolling. Without a selection: "No Task
  Selected" centered, 13/16 `--text-tertiary`.
- **Title:** a text field 17/22 600 named "Title", borderless but for a
  1px `--separator` rounded 6 on hover and focus. Enter or leaving it
  commits a changed, non-empty title; an empty one returns to the old
  title; Escape returns to it and leaves the field.
- **Rows** (label 12/16 `--text-secondary` 80 wide, value 13/18, 32
  high): **Due**, a date input named "Due" that commits on change, and
  when there is a date a 14px `X` button named "Clear Due Date";
  **List**, the list's name, " · " and the account's email
  `--text-tertiary` on one line, truncated, the whole in its tooltip; **Completed**, when it is, the date and time
  (medium date, short time, in the app's zone); **From mail**, for a
  task made from a message, an **Open Message** text button in
  `--accent` that shows the message in Mail.
- **Notes:** a heading "Notes" 12/16 `--text-secondary`, then a textarea
  named "Notes", 13/18, at least 120 high, 1px `--separator` rounded 6,
  8px padding; leaving it commits a change.
- **Delete Task:** a text button 13/16 `--flag-1` at the bottom; it
  deletes the task and its subtasks at once, as Reminders does.
- **Read-only:** the fields are read-only, the date input disabled, no
  Clear or Delete, and a line "Read-only" 12/16 `--text-tertiary` with a
  12px `Lock` under the title.
- **A flagged message** shows its subject as an `h2` 17/22 600, the
  sender and date 13/18 `--text-secondary`, and two text buttons:
  **Open in Mail** and **Clear Flag**.

### Tasks toolbar

`Toolbar`'s `mode` is `"tasks"`, laid out as Calendar's: the middle
segment fills the list's width and the right segment the pane's.

- Over the sidebar: the sidebar toggle.
- Over the list: **New Task** (`Plus`, "New Task (Ctrl+N)"), which
  focuses the New Task field (disabled in Flagged Mail and a read-only
  list), the source's name 15/20 600, and at the
  right **Show Completed**, a 28-high text button with `aria-pressed`,
  shown for a list and All Tasks.
- Over the pane: Settings and the window controls.

### Tasks behavior

- **State** (`useUI`): `tasksSource` (first `"today"`), `tasksSelected`
  (a task's ID, or a message's in Flagged Mail; first none),
  `tasksFocus` (first `list`) and `tasksShowCompleted` (first false).
  Window state, not persisted.
- **Keys** with the list focused: ↑ ↓ Home End move the selection;
  Space toggles the selected task's completion (in Flagged Mail, clears
  the flag); Delete and Backspace delete the selected task (never a
  message); Enter focuses the pane's Title (in Flagged Mail, opens the
  message in Mail); Escape clears the selection. Ctrl+N focuses the New
  Task field from any pane. Tab and Shift+Tab move between the panes;
  Ctrl+4 shows Tasks from anywhere.
- **Data:** one `tasks.list` with `completed` true holds every task; the
  sources, counts and To-Do bar come from it (`sourceTasks`). A
  `tasks.changed` event refetches it within 100 ms; `account.changed`
  refetches the lists (`account.collections` of kind `tasklist` that are
  enabled, sections named by `account.list`). Flagged Mail is
  `view.open` with `flagged` (no threads), its first 200 rows, open
  while the Tasks module or the To-Do bar shows.
- **Writes:** ticking calls `tasks.update` with `completed` and shows the
  change at once; a failure refetches. The pane's edits call
  `tasks.update` with the changed field; New Task `tasks.create`; Delete
  `tasks.delete`, selecting the next row. Clearing a flag is
  `message.setFlags` with `flagColor` 0.
- **Selection** follows the ID across refetches; when it is gone the
  pane is empty. Changing the source clears it.
- **From elsewhere:** the To-Do bar's tasks open in Tasks with their list
  and the task selected; Open Message shows the message in Mail as a
  notification's does (`revealMessage`).

## To-Do bar

Mail's optional right-hand pane (ADR-0020): the small month, what is
next in the calendar, and what is due.

```
┌──────────┬──────────────┬─────────────────────┬────────────────────┐
│ sidebar  │ list         │ reader              │ ‹ October 2026 ›   │
│          │              │                     │ M T W T F S S      │
│          │              │                     │ …                  │
│          │              │                     │ Upcoming           │
│          │              │                     │▌Standup   In 10 min│
│          │              │                     │ Tasks              │
│          │              │                     │ + New Task         │
│          │              │                     │ ○ Report     Today │
│          │              │                     │ ○ ⚑ Contract       │
└──────────┴──────────────┴─────────────────────┴────────────────────┘
```

- **Pane:** 280 wide at the right of the reader, which narrows; 1px
  `--separator` left border, `--bg-sidebar`, scrolling as a whole, a
  `complementary` region named "To-Do Bar". Shown and hidden with the
  toolbar's **To-Do Bar** button (`PanelRight`, `aria-pressed`, over
  the reader before the search field); the choice persists (`todoBar`
  in `useUI`'s persisted part). Mail only.
- **Small month:** `MiniMonth` with its busy days; clicking a day shows
  Calendar on that date, the view kept.
- **Upcoming:** a heading 11/14 600 `--text-secondary` (12px padding,
  16px above), then `UpcomingList` with the occurrences not yet ended and
  not cancelled from now to the end of the seventh day, at most 5, else "No Upcoming
  Events" 12/16 `--text-tertiary`. Clicking one opens it in Calendar.
- **Tasks:** a heading, a New Task field (32 high, into the default
  list), then up to 10 open tasks due within seven days or earlier, by
  due date and then list order, and up to 5 flagged messages, newest
  first. Rows are 32 high: a 16px check circle (completes the task, or
  clears the flag), the title or subject 13/16 truncated, and at the
  right the due text 12/16 (`--flag-1` when overdue) or a 12px `Flag`
  in the flag's color. "Nothing Due" 12/16 `--text-tertiary` when there
  are neither. Clicking a task's title opens it in Tasks; a message's
  subject shows it in Mail.
- **Data:** the tasks of the Tasks module; `calendar.range` over the
  small month's 42 days (dots) and from today to seven days on
  (upcoming), refetched on `calendar.changed` and every minute; the
  flagged view.

## Reminder window

A small window of its own (ADR-0020) that maild's reminders raise over
whatever the app shows.

```
┌──────────────────────────────────────┐
│ Reminders                         ✕  │ 36
├──────────────────────────────────────┤
│▌Standup                 [Snooze ▾] ✓│
│▌In 10 minutes · Room 4               │ 56
│▌Design review           [Snooze ▾] ✓│
│▌Now · Studio B                       │
├──────────────────────────────────────┤
│                         Dismiss All  │ 44
└──────────────────────────────────────┘
```

- **Window:** the Tauri window labeled `reminders` (`open_reminders`; a
  new tab at `#/reminders` in a browser), 380 × 300, always on top, no
  system title bar. `--bg-window`.
- **Title strip** (36 high, 1px `--separator` bottom, draggable): "Reminders"
  13/16 600 at 12px, and Close (`X`, 28 × 28) at the right. Escape closes
  the window; the reminders stay.
- **Rows** (`list` named "Reminders", oldest due first), 56 high, 12px
  padding, a 4px bar in the calendar's color at the left (rounded 2):
  the title 13/18 600 truncated ("No Title"), and under it 12/16
  `--text-secondary` the time (`reminderWhen`) and " · " the location.
  At the right: **Snooze**, a 28-high button with a `ChevronDown` that
  opens a menu of 5 minutes, 10 minutes, 15 minutes, 1 hour and Tomorrow
  (9:00 the next day), and **Dismiss** (28 × 28, `Check`). Clicking the
  title opens the occurrence in the Calendar module of the main window
  (and keeps the reminder). A row is a `listitem`.
- **Footer** (44 high, 1px `--separator` top) with **Dismiss All**, shown
  when there are two or more.
- **When** (`reminderWhen`): timed — "In N minutes" within the hour
  before the start (N rounded up), "Now" for five minutes from it, "N
  minutes ago" for the rest of the hour after it, otherwise "Today, 9:30
  AM", "Tomorrow, 9:30 AM", "Yesterday, 9:30 AM" or "Fri, Oct 9, 9:30
  AM"; all-day — "Today", "Tomorrow", "Yesterday" or "Fri, Oct 9". It is
  renewed every 30 seconds.
- **Behavior:** the window lists `calendar.reminders`, and lists them
  again on `calendar.reminders` and `calendar.changed` events. Snooze calls
  `calendar.snooze` with the chosen time, Dismiss and Dismiss All
  `calendar.dismiss`; the row leaves at once. When the list is empty the
  window closes itself.
- **Raising it:** the main window opens the reminder window when it
  connects and `calendar.reminders` has any, and on each
  `calendar.reminders` event with a count above zero.

## Invitation card

The reader shows an invitation as a card above the message's body
(ADR-0019), answered there.

```
┌──────────────────────────────────────────────────────────────┐
│ ┌───┐  Lunch with Ann                              Invitation│
│ │OCT│  Friday, October 9, 2026 · 12:00 – 1:00 PM             │
│ │ 9 │  Cafe Nord · Ann Smith (organizer)                     │
│ └───┘  ⚠ Conflicts with Design review                        │
│        Before: Standup · After: Weekly sync                  │
│        [Accept] [Maybe] [Decline]          Show in Calendar  │
└──────────────────────────────────────────────────────────────┘
```

- **When:** a message whose parts include one that is `text/calendar`,
  `application/ics` or named `*.ics` (`invitationPart`): the reader asks
  `calendar.invitation` for it, and shows the card when the answer comes;
  an error shows no card. The invitation's part leaves the attachment
  strip while the card shows.
- **Card** (`InvitationCard`): a `region` named "Invitation", 12px from
  the header's sides and 8px below it, rounded 8, a 1px `--separator`
  border, `--bg-sidebar`, 12px padding, a 12px gap between the date tile
  and the text.
  - **Date tile:** 40 × 44, rounded 6, `--bg-window`: the month (short,
    uppercase) 10/12 600 `--flag-1`, then the day 18/22 600, in the app's
    zone (an all-day event's own date).
  - **Title** 15/20 600 (the summary, or "No Title"), and at the right a
    badge 11/14 600: "Invitation" (request), "Cancelled" (cancel, in
    `--flag-1`), "Reply" (reply), "Event" (publish and others).
  - **When** 13/18: `dateText` and " · " and `timeRange` in the app's zone,
    or "All day".
  - **Where and who** 12/16 `--text-secondary`: the location, " · ", the
    organizer's name (else address) and " (organizer)". For a reply,
    instead: the sender's name (else address) and their answer, "accepted",
    "declined" or "said maybe".
  - **Conflicts** 12/16 `--flag-2` with a 12px `TriangleAlert`: "Conflicts
    with " and the conflicts' summaries joined by ", "; left out when
    there are none. **Adjacent** 12/16 `--text-tertiary`: "Before: <summary>"
    and "After: <summary>" joined by " · "; left out when there are none.
  - **Answers** when `canRespond`: a `group` named "Answer" of three text
    buttons, 28 high, 10px padding: Accept, Maybe, Decline; the current
    answer's has `aria-pressed="true"` and `--accent` with
    `--accent-contrast`, the others `--bg-window` with a 1px `--separator`
    border. Otherwise one line 12/16 `--text-secondary`: "This invitation
    is out of date." when outdated, else the user's answer ("You
    accepted", "You declined", "You said maybe"), else nothing.
  - **Show in Calendar** at the right of the last line, a text button in
    `--accent`: Calendar on the event's date, its occurrence selected when
    the calendar has it (`eventId`).
- **Answering** calls `calendar.respond` with the message's ID and the
  answer; the buttons are disabled until it answers, then the card asks
  `calendar.invitation` again. A `calendar.changed` event refreshes it
  too. When maild mails the reply, the outbox's undo toast offers to stop
  it.

## Rules

- Components in `features/` are presentational: data and callbacks by
  props, no imports from `app/src/data/` or `app/src/rpc/` except types.
- Tokens only; no literal colors. Photos are data: URLs from
  `people.photo`; nothing is fetched from the network.
- Every control is reachable by keyboard and has an accessible name.

## References

- Rationale: [ADR-0020](../adr/0020-one-window-for-mail-calendar-people-and-tasks.md),
  [ADR-0017](../adr/0017-sync-contacts-calendars-and-tasks.md)
- Context: [design/pim.md](../design/pim.md), [ui.md](ui.md)
- Plan: [plans/0007-m4.5-people-calendar-tasks.md](../plans/0007-m4.5-people-calendar-tasks.md)
