# 0023 — One condition language for smart mailboxes and rules

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

Mail.app's smart mailboxes and rules are built in the same kind of editor:
a list of conditions, each a field, an operator and a value ("From
contains ann", "Date Received is in the last 7 days", "Sender is a VIP"),
joined by "any" or "all", with no nesting. M5 needs both (parity rows
P-105 and P-701), plus filters and a notification scope built from the
same conditions (P-205, P-803).

Frostmail already turns the search language
([specs/search.md](../specs/search.md)) into SQL: `internal/search` parses
text into a `Query`, the engine copies it into a `store.ViewFilter`, and
`store.ViewIDs` joins the filter's conditions with AND. The FTS5 table
`messages_fts` serves the text conditions. A view keeps its filter and
reruns `ViewIDs` after every commit that touches its account
([ADR-0009](0009-ui-architecture.md)). The search language cannot say
"any of", and text cannot hold an editor's structure.

## Decision

- **Conditions are data.** A `Conditions` value is `match` (`all` or
  `any`) and a list of `Condition`s, each a `field`, an `op` and a
  `value`. There is no nesting, as in Mail.app. The API carries them as
  typed records (`schema/rpc`), and maild stores them as JSON with a
  version, in the smart mailbox or rule that owns them.
- **The store compiles them** to one SQL predicate over `messages`, the
  same way `ViewIDs` builds its conditions today, using `message_mailbox`,
  `messages_fts`, and the tables for VIPs and contacts. Relative dates
  ("today", "in the last 7 days") are computed from the clock each time
  the predicate is built, so a view's recompute moves them along. A
  `ViewFilter` gains the compiled conditions beside its other fields.
- **One compiler, every use.** A smart mailbox is a view of its
  conditions. A rule runs its conditions' SQL over the messages it is
  given. The filter bar's new choices and the notification scope are
  conditions too. `internal/search` converts a parsed search into
  conditions for Save as Smart Mailbox. That conversion is exact, since
  every search operator has a condition.
- **The set is fixed and checked.** maild accepts only the fields and
  operators [design/organize.md](../design/organize.md) lists, with
  values that parse. It refuses anything else with `invalidParams`, so
  stored conditions always compile. A condition naming a mailbox or
  account that later goes away matches nothing, and the editor marks it.
- **Trash and Sent stay out** of a smart mailbox unless it says to include
  them, as in Mail.app's two checkboxes.

## Consequences

- A smart mailbox's list and count are live views for free, and a rule
  and a smart mailbox with the same conditions always agree, because the
  same SQL decides both.
- The condition editor, shared by the smart mailbox sheet and the rule
  editor, is one component over one schema.
- Adding a field means touching the IDL, the compiler, its tests and the
  editor. Each field is a small, separate change.
- A relative-date smart mailbox is only as fresh as its view's last
  recompute. A view with relative dates is also recomputed when the local
  date changes.
- Some text operators cannot use the FTS index ("ends with" on a name), so
  they scan the rows the other conditions leave. Phase 3 measures them on
  the 200,000-message fixture against the 150 ms search budget.

## Alternatives considered

- **Store a search string:** the editor would have to round-trip text, and
  the language has no OR. The search language remains how people type
  searches.
- **Nested AND/OR trees:** more expressive, but Mail.app's flat
  any-or-all covers its users, and the editor stays one list.
- **Evaluate rules in Go over message structs:** a second implementation
  of every condition that would drift from the SQL a smart mailbox uses.

## References

- Shapes: [design/organize.md](../design/organize.md),
  [specs/organize-ui.md](../specs/organize-ui.md) (the editor),
  [specs/parity.md](../specs/parity.md) (P-105, P-205, P-701, P-803)
- Builds on: [ADR-0003](0003-sqlite-store.md) (SQL for smart mailboxes
  and rules), [ADR-0009](0009-ui-architecture.md) (views),
  [specs/search.md](../specs/search.md)
