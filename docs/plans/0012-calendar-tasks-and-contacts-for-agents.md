# Plan 0012: M8.5 Calendar, tasks and contacts for agents

M8.5 gives agents the user's calendars, tasks and contacts through
`/mcp/mail`, beside their mail, and two thread queries that maild computes:
waiting on the user, and waiting on others
([ADR-0042](../adr/0042-agents-get-calendar-task-and-contact-tools.md)).

- **What it is for:** frostmail's part of what
  [Porter](https://github.com/frostyard/porter/blob/main/docs/design/overview.md),
  lodge's assistant app, needs for its morning brief, its weekly review and
  capture.
- **What it builds on:** [plan 0011](0011-m8-mail-in-lodge.md)'s Phase 5,
  which made `/mcp/mail`, its scope and its tools.
- **The contract:** [specs/lodge-app.md](../specs/lodge-app.md#calendar-tasks-and-contacts).

Status (2026-10-21):
- **Phase 1 is done.** The user accepted ADR-0042 on 2026-10-21.
- **Phases 2 to 4 are built** (2026-10-19, frostyard/frostmail#82–#84),
  each with what it built and its evidence.
- **Phase 5 is done** (2026-10-21): checked live over lodge's local agent
  socket
  ([lodge ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)).

## Decisions for M8.5

The user asked for this work on 2026-10-19, with these decisions from
Porter's design:

- **Porter's mail token is `account=*;write`, never `;send`.** Porter
  reaches `/mcp/mail` through lodge's agent socket.
- **Reading** covers events in a range and one event, tasks due, overdue
  or by list and one task, a contact search and one contact, and the two
  thread queries. It comes with every scope.
- **`;write`** adds creating a task, completing one, and Remind Me on a
  message.
- **No event writes in v1,** with attendees or without (ADR-0042).
- **maild computes the waiting threads,** not the model, with VIPs first.
- **Account scope applies:** a calendar, task list or address book belongs
  to an account, so another account's is `notFound`.
- **Generated data only.** Every test and live check uses generated mail,
  events, tasks and contacts, as M8 does until its Phase 7.

## Phase 1 — The decision and the contract (planner, small)

- [ADR-0042](../adr/0042-agents-get-calendar-task-and-contact-tools.md)
- [specs/lodge-app.md](../specs/lodge-app.md#calendar-tasks-and-contacts):
  - the methods agents gain, and their scope;
  - the rules for `thread.awaitingReply` and `thread.awaitingOthers`;
  - the tools, with their inputs, answers and limits.
- **Done when:** the user accepts ADR-0042, and the contract is merged.

## Phase 2 — Agents reach PIM sources in the engine (planner, medium)

- **The IDL:**
  - caller `agent` on the methods in the spec's table;
  - `scope: write` on `tasks.create`, `tasks.update` and `message.remind`;
  - the new method `tasks.get`;
  - `make gen`.
- **The engine's account scope,** beside the mail domains'
  (`internal/engine/scope.go`):
  - `account.collections`, `calendar.range` and `calendar.event`;
  - `tasks.list`, `tasks.get`, `tasks.create` and `tasks.update`;
  - `people.list` and `people.get`;
  - `message.remind`.
- **Under a scope, defaults stay inside it:**
  - `tasks.create` without `listId` picks the default list of the first
    account the scope reaches;
  - `people.get` answers only the contacts the scope reaches, with the
    person's name and organization taken from those.
- **Done when,** on an agent's connection, in `internal/lodge`'s tests,
  with a second account that has its own calendar, task list and address
  book:
  - under `account=<id>`, each method answers only that account's data,
    and the other account's collection, event, task, person and message
    are `notFound`;
  - under `account=<id>`, `tasks.create`, `tasks.update` and
    `message.remind` are `methodNotFound`;
  - under `account=<id>;write;send`, every event write and every other
    method the spec keeps from agents is `methodNotFound`;
  - a person with contacts in both accounts shows only the contact in the
    scope.
- **Built:** the reads in #82, the writes in #84.
  - Tests: `internal/lodge/mcp_pim_test.go` and
    `mcp_pim_write_test.go`, on an agent's connection.
  - People: `store.ScopedPeople` and `store.ScopedPerson` make a person
    of the scope's contacts alone, for `people.list`'s match too.
  - Under `;write`, a parent task of another account is `notFound` to
    `tasks.create` as well.

## Phase 3 — Waiting threads (planner, medium)

- **`thread.awaitingReply` and `thread.awaitingOthers`** in the store and
  the engine, with the `Awaiting` types. Each is one query over every
  thread's newest message, the accounts' identities, VIPs, mutes and
  blocks.
- **A `mailctl` command** that prints both, for checking the rules against
  a store.
- **Done when:**
  - for each rule in the spec, a thread that breaks only that rule is
    left out. Among them:
    - a newest message from an identity, or only to the user in Cc;
    - `List-Id` or `List-Unsubscribe`, and each no-reply local part;
    - `\Answered`, a muted thread, a blocked sender;
    - a newest message outside every inbox, a draft, or one only in
      Trash or Junk;
  - a thread at exactly `waitingDays` is kept, and one at exactly
    `withinDays` is left out;
  - VIPs come first, then the newest. `total` counts every match before
    `limit`. Gmail's `INBOX` label counts as an inbox;
  - only the scope's accounts' threads answer an agent;
  - each param is tested at the ends of its range and one past each end;
  - on 50,000 generated messages (`tools/mailgen`), each method answers in
    under 150 ms.
- **Built** in #83, with `mailctl waiting [--others]`. The rules' tests are
  `internal/engine/awaiting_test.go`, a thread for each rule and boundary,
  with a Gmail account for the `INBOX` label.
  - Timed on 2026-10-19, on `tools/uifixture -n 50000 -seed 7` with maild
    on the Framework (Strix Halo), 20 runs over the socket after one
    warm-up, `limit` 25 and `waitingDays` 0. mailgen's mail is to
    `test@mailtest.test`, so the fixture account gained that identity.
    - `withinDays` 30: `awaitingReply` p95 41.5 ms (1,647 threads match),
      `awaitingOthers` p95 28.8 ms.
    - `withinDays` 90: `awaitingReply` p95 66.4 ms (7,439 match),
      `awaitingOthers` p95 65.4 ms.
    - No thread matched `awaitingOthers`, since mailgen writes no sent
      mail; it still scans the same window.

## Phase 4 — The tools (planner, medium)

- **The read tools** in `internal/mcpserver`:
  - `list_collections`;
  - `list_events` with its clashes, and `read_event`;
  - `list_tasks` and `read_task`;
  - `search_contacts` and `read_contact`;
  - `threads_awaiting_reply` and `threads_awaiting_others`.
- **The write tools,** with `;write`: `create_task`, `complete_task` and
  `remind_me`.
- **What every session is told:** the server's instructions per scope, and
  the widened notice.
- **Done when,** through the kit's `kittest` and the MCP SDK's client, as
  `/mcp/mail`'s tools are tested today (`internal/lodge/mcp_test.go`,
  `mcp_limits_test.go`):
  - **Under `account=<id>`,** the read tools are offered and the write
    tools are not:
    - `list_events` answers a day's occurrences with a clash, and none of
      the other account's;
    - `read_event`, `read_task` and `read_contact` of the other account's
      are errors.
  - **Under `account=<id>;write`:**
    - `create_task` with "Call Iris", due on a Friday, lands in the
      account's default list, and `tasks.list` shows it;
    - `complete_task` completes it, and reopens it;
    - `remind_me` sets a reminder, and clears it;
    - the other account's task and message are errors.
  - **Every limit is tested at its value and one past it,** a `limit` of 0
    being its default:
    - `list_events`: `to` 31 and 32 days after `from`; `limit` 1 and 200,
      -1 and 201; clashes 50 and 51;
    - `list_tasks`: `limit` 1 and 100, -1 and 101;
    - `search_contacts`: `query` of 1 and 256 bytes, empty and 257;
      `limit` 1 and 50, -1 and 51;
    - both thread tools: `limit` 1 and 25, -1 and 26; `waiting_days` 0 and
      30, -1 and 31; `within_days` 1 and 90, 0 and 91;
    - `create_task`: `title` of 1 and 1,024 bytes, empty and 1,025;
      `notes` of 32 KiB and a byte more;
    - `remind_me`: `at` 365 days ahead, a second past that, and a second
      ago;
    - texts of 32 KiB kept whole and texts a byte longer cut; attendees
      100 and 101; recipients 10 and 11.
  - **Every answer** carries the notice, and each scope's tool list
    matches the spec.
- **Built:** the read tools in #82 and #83, and the write tools in #84.
  - Tests: `internal/lodge/mcp_pim_test.go`, `mcp_pim_limits_test.go`,
    `mcp_waiting_test.go` and `mcp_pim_write_test.go`.
  - A `limit` of 0 is its default, so a limit's "one past" below is -1.
  - The tools read "today" and reminder times from `lodge.Options.Now`, so
    the tests run on a fixed clock.
  - Added after, for Porter: `draft_message`, the draft of a new message
    under `;write`, since `draft_reply` drafts only replies.

## Phase 5 — Live, for Porter (planner, small)

- On lodge's development build, with an MCP client on lodge's agent socket
  holding an `account=*;write` token. The data is generated mail, events,
  tasks, contacts and VIPs.
- **Done when** that client:
  - lists today's events with a clash, the tasks due today and overdue,
    the threads waiting on the user with a VIP first, and the threads
    waiting on others;
  - creates "Call Iris" due Friday, which then shows in Tasks in the web
    build;
  - is not offered `send_draft` or any event write, and gets `notFound`
    for an account outside its scope when its token names one account;
  - is cut off at once when its token is revoked.

  The evidence is recorded here.
- **Result (2026-10-21): done,** in two runs.
- **2026-10-19,** from Porter's own Phase 1 check on
  lodge's host (lodge `bab1ed7`, mail `424e2a7`, showcase mail;
  [Porter's roadmap](https://github.com/frostyard/porter/blob/main/docs/plans/0001-roadmap.md)),
  as user `frostporter` on `/run/lodge/agent.sock` with an
  `account=*;write` token:
  - today's three events listed with their clash; nine tasks; the threads
    waiting on the user, with a Message-ID and a `/mail/message/…` link;
  - no send tool offered;
  - the same token on lodge's tailnet route answered 401, and a user
    outside `lodge-agents` could not open the socket.
- **2026-10-20 (23:47 EDT),** on lodge's development build (lodge
  `25d0d3b`, mail `f8a74d1`, showcase mail), with MCP over
  `agent.sock`, an `account=*;write` token and an `account=2` token:
  - `tools/list`: 19 tools under `account=*;write`, with no `send_draft`
    and no event write; the 12 read tools under `account=2`.
  - `list_events` from 2026-10-20: four events, and one clash ("Design
    review" against "Interview: Backend").
  - `list_tasks` due today: three tasks, one of them overdue; `overdue`:
    one task.
  - `threads_awaiting_reply`: ten threads, the VIP's first. The showcase
    mail has no VIP, so the run added one to its generated data.
  - `threads_awaiting_others`: one thread ("Re: Dinner Saturday?") with
    `waiting_days` 0, and none at the default 5 days, because the showcase
    mail is one day old.
  - `create_task` "Call Iris" due Friday 2026-10-23: task 36, shown by
    `list_tasks`, by `mailctl tasks ls` and in the web build's Tasks.
  - Under `account=2`, `read_task`, `read_event` and `list_tasks` for
    account 1's items answered `notFound`. Found: `read_thread` answered
    "message 6 does not exist" without the `notFound:` prefix the others
    have.
  - `lodgectl token revoke` closed the open stream at once ("closed 1"),
    and the next call and a new `initialize` answered 401.

## Later / ideas

- **Event writes,** under a scope word of their own and an ADR of their
  own. Perhaps only on calendars the user marks private, and with
  invitations only under `;send`.
- **Answering invitations** (`calendar.respond`) under `;send`.
- **Storing `Auto-Submitted` and `Precedence`,** so the waiting rules also
  catch automated mail that is not from a list.
- **A Waiting section in the app,** from the same two methods: a smart
  mailbox, or a part of the To-Do bar.
- **Contact writes,** such as adding a contact from mail.
- **Flagged mail in `list_tasks`,** as the Tasks module shows it.

## Open questions

- **Cc.** Should a question to the user in Cc count as waiting on them?
  v1 says no. Revisit it after a week of Porter's briefs on the user's own
  mail.
- **The user's time zone.** An agent passes `time_zone`, and the default is
  maild's local zone, which on lodge's host is the server's. Should maild
  keep the user's zone as a preference
  ([ADR-0026](../adr/0026-preferences-live-in-maild.md))? Decide by
  Phase 4.

## References

- Implements: [specs/lodge-app.md](../specs/lodge-app.md#calendar-tasks-and-contacts), [ADR-0042](../adr/0042-agents-get-calendar-task-and-contact-tools.md)
- Context: [design/pim.md](../design/pim.md), [ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md), [ADR-0025](../adr/0025-maild-keeps-send-later-remind-me-and-undo-send.md), [ADR-0028](../adr/0028-mute-and-block-in-maild-undo-in-the-app.md)
- Model: [plan 0011](0011-m8-mail-in-lodge.md)'s Phase 5
- lodge: [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md), [ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)
- Porter: [design/overview.md](https://github.com/frostyard/porter/blob/main/docs/design/overview.md)
