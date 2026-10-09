# 0017 — Sync contacts, calendars and tasks with CardDAV, CalDAV and Google Tasks

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

Frostmail adds contacts, calendars and a task list, integrated with mail the
way Outlook's client was (one window, invitations in the reader, contact
cards, reminders, flagged mail as tasks). The user's accounts are Gmail
(OAuth with their own client) and iCloud (app-specific password); generic
servers (Fastmail, Nextcloud, Radicale) should work too. Facts that shape
the choice, checked 2026-10-08:

- iCloud serves contacts over CardDAV (`contacts.icloud.com`) and
  calendars over CalDAV (`caldav.icloud.com`) with the same app-specific
  password as mail. Reminders upgraded since iOS 13 are no longer reachable
  over CalDAV.
- Google serves calendars over CalDAV and contacts over CardDAV, both OAuth
  only. Its CalDAV does no tasks (VTODO), no free/busy and no MKCALENDAR, and
  expects clients to use sync-collection after the first sync. Its CardDAV
  serves the user's saved contacts ("My Contacts"); Google's other
  contacts, saved automatically from mail, are only in the People API.
  Google Tasks are only in the Tasks REST API (scope `auth/tasks`).
- CalDAV and CardDAV have no push a desktop client can use: Google's push
  needs a public webhook, iCloud's goes through Apple's push service.
- emersion/go-webdav (v0.7.0, the author of go-imap) has CardDAV and CalDAV
  clients and servers. Its CardDAV client has sync-collection, its CalDAV
  client does not, and neither sends conditional writes (If-Match).

## Decision

- **Protocols.** Contacts sync over CardDAV and calendars over CalDAV for
  every account kind; tasks sync with the Google Tasks API for Google
  accounts and over CalDAV (VTODO) where a server offers it. Flagged mail
  appears as tasks without any sync of its own.
- **Services on accounts.** An account gains contacts, calendar and tasks
  services next to mail, each switched on per account (off for existing
  accounts), found by provider profile (Gmail, iCloud) or by discovery
  (`/.well-known/caldav` and `carddav`, SRV `_caldavs._tcp` and
  `_carddavs._tcp`). They sign in as mail does: iCloud's app-specific
  password; Google's OAuth with the calendar, CardDAV and Tasks scopes added
  to the user's client, granted by signing in again.
- **Library.** `internal/davx` is the only package that imports go-webdav,
  as `imapx` is for go-imap. go-webdav is forked in `third_party/go-webdav`
  ([ADR-0004](0004-go-imap-fork.md)'s rules: a small, recorded,
  upstreamable patch set) to add CalDAV sync-collection and conditional
  `PUT` and `DELETE` of raw bytes. `internal/gtasks` is a small client for
  the Tasks REST API.
- **Freshness.** maild polls: every 5 minutes, at once when the app's
  window gains focus or the user asks, and right after its own writes.
  sync-collection tokens keep a poll that finds nothing to one request per
  collection. Writes go through `pending_ops` like mail's offline actions.

## Consequences

- One sync path covers Gmail, iCloud and standard servers; Google-only
  calendar features (Meet links as structured data, working location,
  focus time) are out of reach until a Google Calendar API adapter is worth
  writing.
- Changes made elsewhere show up within minutes, not seconds, unlike mail's
  IDLE. Frostmail's own changes show at once: they are local first.
- Google's "other contacts" are missing from People until a People API
  adapter is added; seen addresses already cover autocomplete for them.
- iCloud has no tasks for Frostmail; the user's tasks live in Google Tasks.
- A second fork to keep (go-webdav), tested the same way as the first.
- The user's Google client needs three more APIs enabled and one more
  consent screen, as for an unverified app today.

## Alternatives considered

- **Google's own APIs (Calendar, People, Tasks) for Google accounts:**
  richer, but two engines for the same features and a larger OAuth
  surface; kept for later, as adapters behind the same store.
- **Evolution Data Server:** would put events in GNOME Shell's clock menu,
  but ties Frostmail to GNOME's stack and its sync engine, which the
  project avoids.
- **Local-only tasks:** simplest, but not on the user's phone; the user
  chose Google Tasks.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [design/accounts.md](../design/accounts.md)
- Builds on: [ADR-0004](0004-go-imap-fork.md),
  [ADR-0011](0011-sign-in-and-credentials.md)
- Plan: [plans/0007-m4.5-people-calendar-tasks.md](../plans/0007-m4.5-people-calendar-tasks.md)
