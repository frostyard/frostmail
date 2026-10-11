# Spec: the search language

What the search field (and `mailctl search`, `view.open {query: {text}}`)
accepts, and what it means. `internal/search` parses the text into a
`search.Query`; the store turns that into SQL over `messages` and the FTS5
table `messages_fts` (columns `subject`, `from_text`, `to_text`,
`body_text`, `attachment_names`). The language is a small, forgiving
subset of Gmail's: anything it does not understand is searched as text.

## Interface

```go
// Parse never fails: unknown or malformed operators become text terms.
func Parse(text string, now time.Time, loc *time.Location) Query

type Query struct {
    Terms         []Term          // full-text conditions, in input order
    Unread        *bool           // is:unread / is:read
    Flagged       *bool           // is:flagged / -is:flagged
    HasAttachment *bool           // has:attachment / -has:attachment
    After         time.Time       // inclusive lower bound (zero: none)
    Before        time.Time       // exclusive upper bound (zero: none)
    Roles         []api.MailboxRole // in:inbox, in:sent, … (any of them)
    // newer_than: and older_than:'s values ("2d"), when After or Before
    // came from one; a later after:, before: or on: clears them.
    AfterRel, BeforeRel string
}

type Term struct {
    Column string // "" (any column), "subject", "from_text", "to_text", "attachment_names"
    Text   string // the words, as typed, quotes removed
    Phrase bool   // typed in double quotes
    Not    bool   // prefixed with "-"
}

// Match is the FTS5 expression for the positive terms ("" when none).
func (q Query) Match() string
// Exclude is the FTS5 expression matching any negated term ("" when none).
func (q Query) Exclude() string
// Empty reports whether the query has no condition at all.
func (q Query) Empty() bool

// ToConditions turns a query into conditions listing the same messages,
// for Save as Smart Mailbox (docs/design/organize.md, From a search).
func ToConditions(q Query, loc *time.Location) api.Conditions
// Words splits a condition's value into terms on a column: a value wholly
// in double quotes is one phrase, else each word with a letter or digit.
func Words(value, column string) []Term
// ParseRelative reads newer_than:'s Nd, Nw, Nm or Ny as a day in loc.
func ParseRelative(v string, now time.Time, loc *time.Location) (time.Time, bool)
```

## Rules

- **Tokens.** The text is split on whitespace, except inside double quotes;
  an unclosed quote runs to the end. A quote opens a phrase only at the start
  of a token or right after `name:`; elsewhere it is part of the word. A
  token may start with `-` (negation)
  and may be `name:value`, where the value may itself be quoted
  (`subject:"lunch plans"`).
- **Text terms.** A bare word or quoted phrase is a `Term` with no column.
  Words with no letter or digit are dropped (`-`, `--`, `"…"` of
  punctuation).
- **Operators** (names case-insensitive):

  | Operator | Meaning |
  | --- | --- |
  | `from:x` | `Term{Column: "from_text"}` |
  | `to:x`, `cc:x` | `Term{Column: "to_text"}` |
  | `subject:x` | `Term{Column: "subject"}` |
  | `filename:x` | `Term{Column: "attachment_names"}` |
  | `is:unread`, `is:read` | `Unread` true / false (`-` inverts) |
  | `is:flagged`, `is:starred` | `Flagged` true (`-` makes it false) |
  | `has:attachment` | `HasAttachment` true (`-` makes it false) |
  | `after:D`, `before:D`, `on:D` | `After` = D 00:00, `Before` = D 00:00, or both for D and the next day, in `loc` |
  | `newer_than:Nd`, `older_than:Nd` | `After` / `Before` = start of the day N days before `now` (also `Nw` weeks, `Nm` 30-day months, `Ny` 365-day years) |
  | `in:inbox`, `in:drafts`, `in:sent`, `in:junk` (`in:spam`), `in:trash`, `in:archive` | add the role to `Roles` |

  `D` is `YYYY-MM-DD` or `YYYY/MM/DD`. An operator with an empty, unknown or
  malformed value (`in:nowhere`, `after:yesterday`, `size:3`), or an unknown
  name, is a text term with the whole token as its text (`size:3`). A later
  `is:`, `has:`, `after:` or `before:` replaces an earlier one; `in:`
  accumulates without repeats. Negation applies to text terms, `is:` and
  `has:`; a negated `in:`, `after:`, `before:`, `on:`, `newer_than:` or
  `older_than:` is a negated text term whose text is the token without its
  `-` (`-in:trash` searches the words "in" and "trash" out).
- **Match.** Each positive term becomes a quoted FTS5 phrase with every
  `"` in its text doubled, prefixed with `column : ` when it has a column
  (`subject : "lunch"*`); bare words get a trailing `*` (prefix match),
  phrases do not. Terms are joined by spaces (AND), in input order.
- **Exclude.** The negated terms, formatted the same way, joined with
  ` OR `, wrapped in parentheses when there is more than one.

## Examples

| Input | Query |
| --- | --- |
| `lunch` | Terms `[{"" lunch}]`; Match `"lunch"*` |
| `from:ann subject:"q3 plan"` | Match `from_text : "ann"* subject : "q3 plan"` |
| `report -draft` | Match `"report"*`; Exclude `"draft"*` |
| `-from:bob -spam` | Match ``; Exclude `(from_text : "bob"* OR "spam"*)` |
| `is:unread has:attachment` | Unread true, HasAttachment true; no terms |
| `-is:unread` | Unread false |
| `after:2026-09-01 before:2026/10/01` | After 2026-09-01 00:00, Before 2026-10-01 00:00 (loc) |
| `on:2026-10-07` | After 2026-10-07 00:00, Before 2026-10-08 00:00 |
| `newer_than:2d` (now 2026-10-08 15:00) | After 2026-10-06 00:00 |
| `in:sent in:inbox in:sent` | Roles `[sent inbox]` |
| `size:3 in:nowhere after:yesterday` | three text terms, as typed |
| `say "hi` | Terms `[{"" say} {"" hi phrase}]` |
| `it's 5"x` | Match `"it's"* "5""x"*` (a quote inside a word is text) |

## References

- Context: [design/storage.md](../design/storage.md) (the FTS table),
  [specs/ui.md](ui.md) (the search field)
