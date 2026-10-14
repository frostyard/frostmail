# 0029 — Show no mail categories

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

Mail.app sorts the inbox into Primary, Transactions, Updates and
Promotions by learning on the device (P-901). Gmail sorts mail into its own
tabs, Primary, Social, Promotions, Updates and Forums, plus Purchases and
Reservations beside them. iCloud's server sorts nothing. Plan 0008's
Phase 7 measured both ways of getting categories on the user's accounts on
2026-10-14 with `tools/catprobe`, read-only, over the user's Gmail
account's 365-day window (39,010 messages of the 689,212 in All Mail) and
the iCloud account (5,295 messages).

- **Gmail answers only by search.** `UID SEARCH X-GM-RAW "category:…"` on
  All Mail works for every category. No category appears in LIST or in
  X-GM-LABELS: 100 messages sampled across the tabs carried only
  `\Important`, `\Inbox` and two user labels. Neither CONDSTORE nor IDLE can
  report a message moved to another tab, so only another search finds it.
- **Primary is not a category.** `category:primary` matched 2,019 messages,
  all in the inbox. Outside the inbox, Primary is having none of the other
  categories.
- **The searches are cheap when bounded by date.** Over the window, each
  search took a median of 160–500 ms, 1.5 s for the five tabs. Bounded to
  mail since yesterday, each took 150–270 ms, 0.9 s for the five. A UID
  floor over the newest 200 messages was slower, 3.9 s for the five: Gmail
  does not narrow a search by UID cheaply.
- **Most of the inbox is not Primary.** Of the 14,173 inbox messages,
  Gmail puts 7,611 (54%) in Updates, 3,473 (25%) in Promotions, 2,019
  (14%) in Primary, 868 (6%) in Social and 202 (1%) in Forums. Over the
  window, Promotions holds 17,033 messages and Updates 15,327; Purchases
  holds 2,009, all of them also in a tab.
- **Sorting on the machine is a guess.** A classifier of the stored
  headers (sender, subject, List-Id, List-Unsubscribe, contacts and
  conversations the user wrote in) agreed with Gmail on 58.1% of 36,124
  received messages, mapped to Mail.app's four. It found Primary (98.6% of
  Gmail's) but called Primary much that Gmail calls Updates (precision
  20.4%), and it found only 28% of Updates and 20.5% of Transactions. A
  memory of each sender's Gmail category, learned from the oldest 80% of
  that mail, agreed on 84.1% of the newest 20%. iCloud has no answer to
  learn from, and only 21% of its mail comes from an address the Gmail
  account has seen, 5% more from a domain it has.

## Decision

Frostmail shows no mail categories. The inbox is one list, as are All
Inboxes and every mailbox; maild stores no category and searches for none.
P-901 is Out. The user chose this on 2026-10-14, after the measurements
above.

## Consequences

- Nothing in sync, the store, the API or the UI changes. A Gmail pass
  makes no category searches, and `mailctl verify` has no state to compare
  that the server cannot report.
- Every account's inbox looks the same: no sorted Gmail beside an unsorted
  iCloud in All Inboxes.
- On the user's Gmail, 86% of the inbox is mail Gmail keeps out of Primary,
  and Frostmail lists it with the rest. Gmail's web and phone apps still
  show their tabs, so the same inbox looks different there.
- Narrowing stays with what exists: rules that file mail, smart mailboxes
  over the sender, contacts, VIPs and List-Id, the filter bar, mute, block
  and unsubscribe.
- `imapx.SearchGmailRaw` stays: `tools/catprobe` uses it to repeat these
  measurements, and server search beyond the sync window (P-604) will need
  it. A later ADR that brings categories back supersedes this one.

## Alternatives considered

- **Gmail's categories on Gmail accounts, the rest Primary:** cheap (four
  date-bounded searches a pass, Primary as the absence of the others, and
  a periodic search of the window to catch mail moved between tabs). But
  iCloud mail would never be sorted, and All Inboxes would mix sorted and
  unsorted mail.
- **Gmail's categories, and local guesses elsewhere:** the header
  classifier starts at 58% agreement. A sender memory reaches 84% on Gmail
  only because Gmail taught it, and iCloud shares too few senders to learn
  from Gmail, so it would start from the classifier and the user's
  corrections.
- **Sorting locally everywhere:** throws away Gmail's accurate answer for
  a weaker one, and on-device learning is Out already (P-415).
- **Gmail's five tabs:** Gmail's own view rather than Mail.app's, and for
  Gmail accounts only.

## References

- Shapes: [specs/parity.md](../specs/parity.md) (P-901),
  [plans/0008](../plans/0008-m5-mail-app-parity.md) (Phase 7)
- Builds on: [ADR-0012](0012-gmail-labels-as-memberships.md),
  [ADR-0016](0016-sync-a-window-of-recent-mail.md)
