-- Contacts, calendars and tasks (docs/design/pim.md, ADR-0017, ADR-0018).
-- Sources are stored as the server sent them (objects.raw); every other
-- table below objects is an index rebuilt from them.

-- The OAuth scopes the account's grant covers, space-separated, from the
-- token response; services whose scope is missing wait for a new sign-in.
ALTER TABLE accounts ADD COLUMN granted_scopes TEXT NOT NULL DEFAULT '';

-- The services an account syncs besides mail. A row exists once the user
-- has turned a service on; enabled = 0 stops syncing and keeps the data.
CREATE TABLE account_services (
  account_id   INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  service      TEXT NOT NULL CHECK (service IN ('contacts', 'calendar', 'tasks')),
  enabled      INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  url          TEXT NOT NULL, -- where discovery starts (a profile's, the user's or SRV's URL), or the Tasks API's base URL
  home         TEXT NOT NULL DEFAULT '', -- the DAV home set discovery found from url; '' until it has
  last_sync_at TEXT,          -- the end of the last complete pass
  last_error   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (account_id, service)
) STRICT, WITHOUT ROWID;

-- An address book, a calendar (CalDAV: events, tasks or both, by
-- components) or a Google Tasks list.
CREATE TABLE collections (
  id          INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: clients hold IDs
  account_id  INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK (kind IN ('addressbook', 'calendar', 'tasklist')),
  href        TEXT NOT NULL, -- the collection's URL path, or the Google list's ID
  name        TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '', -- #rrggbb, or '' for the module's default
  components  TEXT NOT NULL DEFAULT '', -- calendars: 'VEVENT', 'VTODO' or 'VEVENT,VTODO'
  read_only   INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1)),
  enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)), -- synced and shown
  is_default  INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)), -- where new objects go
  position    INTEGER NOT NULL DEFAULT 0,
  sync_token  TEXT NOT NULL DEFAULT '', -- the sync-collection token, or Google's updatedMin mark
  ctag        TEXT NOT NULL DEFAULT '',
  synced_at   TEXT,
  UNIQUE (account_id, kind, href)
) STRICT;

-- One contact, calendar object or Google task, as sent.
CREATE TABLE objects (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  href          TEXT NOT NULL, -- the object's URL path, or the Google task's ID
  etag          TEXT NOT NULL DEFAULT '', -- '' until the server has stored a local creation
  kind          TEXT NOT NULL CHECK (kind IN ('vcard', 'vevent', 'vtodo', 'gtask', 'other')),
  uid           TEXT NOT NULL DEFAULT '',
  raw           BLOB NOT NULL, -- the source as the server sent it, or as patched here
  parse_error   TEXT NOT NULL DEFAULT '', -- why the index below has nothing for it
  updated_at    TEXT NOT NULL,
  UNIQUE (collection_id, href)
) STRICT;
CREATE INDEX objects_uid ON objects (uid);

-- A person: the contacts that share an email address, across address
-- books and accounts. id is the smallest objects.id among them.
CREATE TABLE people (
  id           INTEGER PRIMARY KEY,
  display_name TEXT NOT NULL,
  sort_key     TEXT NOT NULL, -- lowercased; family name first when there is one
  organization TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX people_sort ON people (sort_key);

-- A vCard's index (internal/vcardx). Groups (KIND:group) are not contacts.
CREATE TABLE contacts (
  object_id    INTEGER PRIMARY KEY REFERENCES objects (id) ON DELETE CASCADE,
  person_id    INTEGER REFERENCES people (id) ON DELETE SET NULL,
  display_name TEXT NOT NULL DEFAULT '',
  sort_key     TEXT NOT NULL DEFAULT '',
  given_name   TEXT NOT NULL DEFAULT '',
  family_name  TEXT NOT NULL DEFAULT '',
  organization TEXT NOT NULL DEFAULT '',
  photo        BLOB,                     -- an inline PHOTO's bytes
  photo_type   TEXT NOT NULL DEFAULT ''  -- its media type, such as image/jpeg
) STRICT;
CREATE INDEX contacts_person ON contacts (person_id);

CREATE TABLE contact_emails (
  contact_id INTEGER NOT NULL REFERENCES contacts (object_id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  email      TEXT NOT NULL, -- lowercased
  label      TEXT NOT NULL DEFAULT '', -- home, work, other, or the card's own label
  PRIMARY KEY (contact_id, position)
) STRICT, WITHOUT ROWID;
CREATE INDEX contact_emails_email ON contact_emails (email);

-- A VEVENT's index (internal/calendar): a series' master or single event
-- (recurrence_id ''), or an override of one occurrence.
CREATE TABLE events (
  id             INTEGER PRIMARY KEY,
  object_id      INTEGER NOT NULL REFERENCES objects (id) ON DELETE CASCADE,
  uid            TEXT NOT NULL,
  recurrence_id  TEXT NOT NULL DEFAULT '', -- the replaced occurrence's start, as start_at
  summary        TEXT NOT NULL DEFAULT '',
  location       TEXT NOT NULL DEFAULT '',
  description    TEXT NOT NULL DEFAULT '',
  all_day        INTEGER NOT NULL DEFAULT 0 CHECK (all_day IN (0, 1)),
  start_at       TEXT NOT NULL, -- timed: a UTC instant (TimeFormat); all-day: YYYY-MM-DD
  end_at         TEXT NOT NULL, -- likewise; exclusive
  tzid           TEXT NOT NULL DEFAULT '', -- DTSTART's IANA zone; 'UTC'; '' when floating or all-day
  recurrence     TEXT NOT NULL DEFAULT '', -- the RRULE, RDATE and EXDATE lines as sent
  status         TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed', 'tentative', 'cancelled')),
  transparent    INTEGER NOT NULL DEFAULT 0 CHECK (transparent IN (0, 1)), -- does not count as busy
  organizer      TEXT NOT NULL DEFAULT '', -- lowercased email
  organizer_name TEXT NOT NULL DEFAULT '',
  -- The user's answer when one of the account's addresses is an attendee.
  partstat       TEXT NOT NULL DEFAULT ''
                 CHECK (partstat IN ('', 'needsaction', 'accepted', 'declined', 'tentative', 'delegated')),
  sequence       INTEGER NOT NULL DEFAULT 0,
  UNIQUE (object_id, recurrence_id)
) STRICT;
CREATE INDEX events_uid ON events (uid);

CREATE TABLE event_attendees (
  event_id INTEGER NOT NULL REFERENCES events (id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  email    TEXT NOT NULL, -- lowercased
  name     TEXT NOT NULL DEFAULT '',
  role     TEXT NOT NULL DEFAULT 'required' CHECK (role IN ('chair', 'required', 'optional', 'none')),
  partstat TEXT NOT NULL DEFAULT 'needsaction'
           CHECK (partstat IN ('needsaction', 'accepted', 'declined', 'tentative', 'delegated')),
  PRIMARY KEY (event_id, position)
) STRICT, WITHOUT ROWID;
CREATE INDEX event_attendees_email ON event_attendees (email);

-- Occurrences inside instance_window (ADR-0018): one per single event, one
-- per occurrence of a series, under the override's event when one
-- replaces it.
CREATE TABLE instances (
  event_id      INTEGER NOT NULL REFERENCES events (id) ON DELETE CASCADE,
  recurrence_id TEXT NOT NULL DEFAULT '', -- the occurrence's original start; '' for a single event
  all_day       INTEGER NOT NULL CHECK (all_day IN (0, 1)),
  start_at      TEXT NOT NULL, -- as events.start_at
  end_at        TEXT NOT NULL,
  PRIMARY KEY (event_id, recurrence_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX instances_start ON instances (start_at);

-- The dates instances covers: from_date inclusive, to_date exclusive.
CREATE TABLE instance_window (
  id        INTEGER PRIMARY KEY CHECK (id = 1),
  from_date TEXT NOT NULL,
  to_date   TEXT NOT NULL
) STRICT;

-- An event's VALARMs that Frostmail shows: DISPLAY and AUDIO.
CREATE TABLE alarms (
  event_id       INTEGER NOT NULL REFERENCES events (id) ON DELETE CASCADE,
  position       INTEGER NOT NULL,
  action         TEXT NOT NULL CHECK (action IN ('display', 'audio')),
  offset_seconds INTEGER, -- relative to the occurrence's start (end when related_end); negative is before
  related_end    INTEGER NOT NULL DEFAULT 0 CHECK (related_end IN (0, 1)),
  trigger_at     TEXT,    -- an absolute trigger (TimeFormat)
  PRIMARY KEY (event_id, position),
  CHECK ((offset_seconds IS NULL) != (trigger_at IS NULL))
) STRICT, WITHOUT ROWID;

-- Reminders that fired, and what the user did with them. Keyed by the
-- event's UID rather than its row, which a resync replaces, so a dismissed
-- reminder stays dismissed; an event moved to a new time reminds again.
CREATE TABLE reminders (
  collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  uid           TEXT NOT NULL,
  recurrence_id TEXT NOT NULL DEFAULT '',
  trigger_at    TEXT NOT NULL, -- when the alarm was due
  fired_at      TEXT NOT NULL,
  snoozed_until TEXT,
  dismissed_at  TEXT,
  PRIMARY KEY (collection_id, uid, recurrence_id, trigger_at)
) STRICT, WITHOUT ROWID;

-- A VTODO's or Google task's index.
CREATE TABLE tasks (
  object_id    INTEGER PRIMARY KEY REFERENCES objects (id) ON DELETE CASCADE,
  uid          TEXT NOT NULL DEFAULT '', -- the VTODO's UID, or the Google task's ID
  parent_uid   TEXT NOT NULL DEFAULT '', -- RELATED-TO (RELTYPE=PARENT), or Google's parent
  title        TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',
  due          TEXT NOT NULL DEFAULT '', -- YYYY-MM-DD, a UTC instant (TimeFormat), or ''
  completed    INTEGER NOT NULL DEFAULT 0 CHECK (completed IN (0, 1)),
  completed_at TEXT,
  position     TEXT NOT NULL DEFAULT '', -- the server's sort key, compared as text
  gm_thrid     INTEGER  -- the Gmail thread a task made from a message links to
) STRICT;
CREATE INDEX tasks_due ON tasks (completed, due);

-- Changes made here, waiting to be written to the server (pimsync). The
-- object holds the patched source; payload_json holds the patch, to apply
-- again to the server's newer version after a 412.
CREATE TABLE pim_ops (
  id            INTEGER PRIMARY KEY,
  account_id    INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  object_id     INTEGER REFERENCES objects (id) ON DELETE SET NULL, -- NULL once a delete removed it
  kind          TEXT NOT NULL CHECK (kind IN ('put', 'delete', 'tasks.insert', 'tasks.patch', 'tasks.delete')),
  href          TEXT NOT NULL, -- where to write, kept after the object is gone
  if_match      TEXT NOT NULL DEFAULT '', -- the ETag the change was made on; '' creates (If-None-Match: *)
  payload_json  TEXT NOT NULL DEFAULT '{}',
  state         TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'failed')),
  attempts      INTEGER NOT NULL DEFAULT 0,
  next_try_at   TEXT NOT NULL,
  last_error    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL
) STRICT;
CREATE INDEX pim_ops_due ON pim_ops (account_id, state, next_try_at);
CREATE INDEX pim_ops_object ON pim_ops (object_id);
