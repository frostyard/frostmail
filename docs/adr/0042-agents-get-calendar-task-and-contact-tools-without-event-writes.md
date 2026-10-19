# 0042 — Agents get calendar, task and contact tools, without event writes

- **Status:** Proposed
- **Date:** 2026-10-19

## Context

`/mcp/mail` serves agents the user's mail and nothing else
([specs/lodge-app.md](../specs/lodge-app.md#mcp),
[plan 0011](../plans/0011-m8-mail-in-lodge.md)'s Phase 5):

- the read tools come with every scope;
- `;write` adds flags, moves and drafted replies;
- `;send` adds sending a draft.

maild also holds the user's calendars, tasks and contacts
([ADR-0017](0017-sync-contacts-calendars-and-tasks.md)), and no agent can
reach them.

The first agent that needs them is **Porter**, lodge's assistant app
([frostyard/porter](https://github.com/frostyard/porter/blob/main/docs/design/overview.md)).
It reaches mail through `/mcp/mail` over lodge's local agent socket
([lodge ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)),
with a token scoped `account=*;write` and never `;send`. Its routines, a
weekday morning brief and a Sunday weekly review, need:

- today's events and next week's, with clashes;
- tasks due today and overdue, and undated ones;
- **waiting on you:** up to five threads where someone asked the user
  something they haven't answered, VIPs first;
- **waiting on others:** mail the user sent that has had no reply in five
  days.

Its capture turns "remind me to call Iris Friday" into a task.

Porter thinks with the user's local model. On 2026-10-19 that model took
about 2 s and 550 to 880 prompt tokens a step. If it found waiting threads
itself, by searching and reading threads, it would need many steps and
many tokens. Two runs would also pick different threads, and the text a
hostile message carries would help decide what counts as waiting.

Calendar writes reach other people:

- an event with attendees sends invitations, through the provider's
  scheduling ([ADR-0035](0035-invite-through-the-providers-scheduling.md))
  or by iTIP mail
  ([ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md));
- answering an invitation mails its organizer
  ([ADR-0019](0019-answer-invitations-through-the-server.md));
- a calendar may be shared with other people, and maild cannot tell
  whether one is.

The PIM data is not all the user's own words:

- an invitation from anyone puts its title, place and description on the
  user's calendar, and Gmail adds invitations by itself;
- a task made from mail in Gmail links to that mail;
- contacts come from address books that others may share.

## Decision

- **`/mcp/mail` gains read tools for calendars, tasks and contacts,** with
  every scope:
  - `list_collections`: the calendars, task lists and address books;
  - `list_events`, the occurrences in a range of days with their clashes,
    and `read_event`;
  - `list_tasks`, due today or overdue, overdue only, undated, or by list,
    and `read_task`;
  - `search_contacts` and `read_contact`.
- **Two thread tools, whose answers maild computes:**
  - `threads_awaiting_reply` (waiting on you);
  - `threads_awaiting_others` (waiting on others).

  They call two new maild methods, `thread.awaitingReply` and
  `thread.awaitingOthers`. maild chooses the threads by fixed rules over
  what it stores:
  - who sent each thread's newest message, and to whom;
  - list and no-reply senders;
  - `\Answered`, mute and block
    ([ADR-0028](0028-mute-and-block-in-maild-undo-in-the-app.md));
  - the inbox, and the messages' dates.

  Threads with a VIP come first. The model reads the answer; it does not
  decide which threads are waiting.
- **`;write` gains tasks and Remind Me,** through three tools:
  - `create_task`;
  - `complete_task`, which also reopens a task;
  - `remind_me` on a message
    ([ADR-0025](0025-maild-keeps-send-later-remind-me-and-undo-send.md)).

  Like flags and moves, these change only the user's own data and its
  servers, and each can be undone: by reopening the task, by clearing the
  reminder, or by deleting the task in Tasks.
- **No event writes in v1,** whether or not the event has attendees.
  - Agents cannot create, change, delete or answer an event, or snooze or
    dismiss its reminder, under any scope, `;send` included.
  - With attendees, an event write sends mail, so it would be `;send`'s at
    the least.
  - Without attendees, it is left out too:
    - maild cannot tell a shared calendar from a private one, so others
      may see an event without attendees;
    - a hostile message could lead an agent to plant an event that looks
      like the user's own ("call the bank on…"), and the user trusts their
      calendar more than a draft;
    - Porter, the first agent, puts event changes up for the user's
      approval, so v1 does not need them.
  - A later ADR may add event writes under a scope word of their own.
- **Account scope covers PIM sources.**
  - Each calendar, task list and address book belongs to an account. Its
    events, tasks and contacts are reachable when the scope reaches that
    account.
  - Another account's collection, event, task, contact or message is
    `notFound`, and lists skip it, as for mail.
  - A person joins contacts across accounts
    ([design/pim.md](../design/pim.md#storage)). An agent sees only the
    person's contacts that its scope reaches, with the name and
    organization taken from those. A person with none of them is
    `notFound`.
- **The scope grammar is unchanged.** `;write` means more: a token issued
  with `;write` before this gains the task and Remind Me tools when its
  maild is updated.
- **Every answer carries the data notice,** which now names calendar,
  tasks and contacts as well as mail.
- **Every list and text has a limit,** each tested at its value and one
  past it ([specs/lodge-app.md](../specs/lodge-app.md#calendar-tasks-and-contacts)).

## Consequences

- The morning brief needs one call for each of its sections: events,
  waiting on you, and tasks due. The model reads tens of lines, not
  whole threads.
- maild gains new methods:
  - `tasks.get`;
  - `thread.awaitingReply` and `thread.awaitingOthers`;
  - caller `agent` on `account.collections`, `calendar.range`,
    `calendar.event`, `tasks.list`, `people.list` and `people.get`;
  - with `;write`, on `tasks.create`, `tasks.update` and `message.remind`.

  The engine enforces account scope on each of them, as it does for mail.
- The rules for waiting threads are maild's, and so are their misses:
  - a question asked to the user in Cc;
  - a reply that is outside the sync window
    ([ADR-0016](0016-sync-a-window-of-recent-mail.md)) or not threaded;
  - a thread filed away before it was answered.

  The model can still search for more.
- Agents cannot schedule anything. "Put lunch with Iris on my calendar" is
  the user's to do, or a task for later.
- Completing or creating a task writes to its server, Google Tasks or
  CalDAV, as the user's own change would.
- `list_tasks` lists tasks only, not flagged mail. The app's Tasks module
  shows both, and `search_messages` with `is:flagged` finds flagged mail.

## Alternatives considered

- **Letting the model find waiting threads** with `search_messages` and
  `read_thread`: that takes many steps and many tokens, and the answer
  changes from run to run. A message's own text ("you already answered
  this") could also steer the answer.
- **A smart mailbox for waiting threads**
  ([ADR-0023](0023-one-condition-language-for-smart-mailboxes-and-rules.md)):
  its conditions match one message at a time. They cannot say that a
  message is its thread's newest, or that its thread has no reply.
- **Allowing events without attendees under `;write`:** for the three
  reasons in the Decision.
- **A scope word for tasks, such as `;tasks`:** that is more grammar and
  more tokens for changes of the same kind as flags and moves.
- **One MCP tool per RPC method:** that shows agents maild's shapes, which
  change, and every parameter. Fewer tools, named for what the user asks,
  can limit their answers and stay stable.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md#calendar-tasks-and-contacts), [plan 0012](../plans/0012-calendar-tasks-and-contacts-for-agents.md)
- Builds on: [ADR-0041](0041-maild-runs-as-a-lodge-app.md), [ADR-0017](0017-sync-contacts-calendars-and-tasks.md), [ADR-0025](0025-maild-keeps-send-later-remind-me-and-undo-send.md), [ADR-0028](0028-mute-and-block-in-maild-undo-in-the-app.md)
- Related: [ADR-0019](0019-answer-invitations-through-the-server.md), [ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md), [ADR-0035](0035-invite-through-the-providers-scheduling.md), [ADR-0016](0016-sync-a-window-of-recent-mail.md), [ADR-0023](0023-one-condition-language-for-smart-mailboxes-and-rules.md)
- lodge: [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md), [ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)
- Porter: [design/overview.md](https://github.com/frostyard/porter/blob/main/docs/design/overview.md)
