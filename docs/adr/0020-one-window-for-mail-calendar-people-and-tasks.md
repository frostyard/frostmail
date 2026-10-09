# 0020 — One window for mail, calendar, people and tasks

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

Apple ships Mail, Calendar, Contacts and Reminders as separate apps that
share little on screen. Outlook's desktop client put them in one window,
and the integration is what users remember: today's agenda beside the
inbox, invitations answered in the reader, a sender's card one click away,
flagged mail in the task list, reminders that appear whatever is open.
The user wants that integrated feel. Frostmail's look stays Mail.app's
([ADR-0009](0009-ui-architecture.md)).

## Decision

- **One main window with four modules:** Mail, Calendar, People and Tasks,
  switched from the bottom of the sidebar and with Ctrl+1 to Ctrl+4
  (Outlook's own keys). Each module keeps its own sidebar, list and detail
  panes in the main window's layout; the toolbar's buttons follow the
  module.
- **A To-Do bar in Mail:** an optional right-hand pane with a small month,
  the next events, and tasks due soon, including flagged mail.
- **People everywhere:** a sender's name in the reader, the list and
  compose opens their card (details, recent mail, upcoming events with
  them, New Message, Add to Contacts).
- **Reminders in a small window of their own,** with Snooze and Dismiss,
  raised over whatever module is open, and as desktop notifications from
  maild when the app is closed.
- Compose, settings and, later, an event editor stay separate windows.

## Consequences

- One app, one process, one connection to maild; modules share the stores
  and the people cache.
- The main window's layout has to fit four modules; the sidebar and toolbar
  specs grow a module dimension.
- A user who wants two calendars side by side waits for "open in new
  window", which is left for later.

## Alternatives considered

- **Separate windows per module (Apple's model):** a smaller change to the
  layout, but the integration the user asked for lives between modules.

## References

- Shapes: [design/pim.md](../design/pim.md), [design/app.md](../design/app.md),
  [specs/ui.md](../specs/ui.md)
- Builds on: [ADR-0009](0009-ui-architecture.md)
