-- 0001_init.sql: the initial frostmail schema (docs/design/storage.md).
-- Pre-release, this file is edited in place; from the first daily-driver
-- release on, schema changes are new append-only migrations
-- (docs/adr/0003-sqlite-store.md). Times are UTC text in the fixed format
-- 2006-01-02T15:04:05.000Z so they sort lexically.

CREATE TABLE accounts (
  id                INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: clients and secrets hold IDs
  kind              TEXT NOT NULL CHECK (kind IN ('imap', 'gmail', 'microsoft', 'icloud')),
  email             TEXT NOT NULL UNIQUE COLLATE NOCASE,
  display_name      TEXT NOT NULL DEFAULT '',
  auth              TEXT NOT NULL CHECK (auth IN ('password', 'oauth2')),
  imap_host         TEXT NOT NULL,
  imap_port         INTEGER NOT NULL CHECK (imap_port BETWEEN 1 AND 65535),
  imap_tls          TEXT NOT NULL CHECK (imap_tls IN ('tls', 'starttls', 'insecure')),
  imap_username     TEXT NOT NULL,
  smtp_host         TEXT NOT NULL,
  smtp_port         INTEGER NOT NULL CHECK (smtp_port BETWEEN 1 AND 65535),
  smtp_tls          TEXT NOT NULL CHECK (smtp_tls IN ('tls', 'starttls', 'insecure')),
  smtp_username     TEXT NOT NULL,
  quirks_json       TEXT NOT NULL DEFAULT '{}',
  server_saves_sent INTEGER CHECK (server_saves_sent IN (0, 1)), -- NULL until probed
  needs_reauth      INTEGER NOT NULL DEFAULT 0 CHECK (needs_reauth IN (0, 1)),
  created_at        TEXT NOT NULL
) STRICT;

CREATE TABLE identities (
  id             INTEGER PRIMARY KEY,
  account_id     INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  name           TEXT NOT NULL DEFAULT '',
  email          TEXT NOT NULL,
  reply_to       TEXT NOT NULL DEFAULT '',
  signature_html TEXT NOT NULL DEFAULT '',
  is_default     INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1))
) STRICT;

CREATE TABLE mailboxes (
  id              INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: clients and secrets hold IDs
  account_id      INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  path            TEXT NOT NULL,
  delimiter       TEXT NOT NULL DEFAULT '',
  name            TEXT NOT NULL,
  role            TEXT NOT NULL DEFAULT 'none'
                  CHECK (role IN ('none', 'inbox', 'drafts', 'sent', 'archive', 'junk', 'trash', 'all', 'flagged')),
  attrs_json      TEXT NOT NULL DEFAULT '[]',
  is_gmail_label  INTEGER NOT NULL DEFAULT 0 CHECK (is_gmail_label IN (0, 1)),
  subscribed      INTEGER NOT NULL DEFAULT 1 CHECK (subscribed IN (0, 1)),
  selectable      INTEGER NOT NULL DEFAULT 1 CHECK (selectable IN (0, 1)),
  uidvalidity     INTEGER, -- NULL until the first SELECT
  uidnext         INTEGER,
  highestmodseq   INTEGER,
  server_count    INTEGER, -- MESSAGES at the end of the last reconcile pass (fast path)
  synced_low_uid  INTEGER, -- backfill frontier: lowest UID with headers stored
  needs_reconcile INTEGER NOT NULL DEFAULT 0 CHECK (needs_reconcile IN (0, 1)),
  last_sync_at    TEXT,
  UNIQUE (account_id, path)
) STRICT;

CREATE TABLE threads (
  id                INTEGER PRIMARY KEY,
  account_id        INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  gm_thrid          INTEGER, -- Gmail X-GM-THRID; NULL elsewhere
  subject_norm      TEXT NOT NULL DEFAULT '',
  last_date         TEXT NOT NULL,
  msg_count         INTEGER NOT NULL DEFAULT 0,
  unread_count      INTEGER NOT NULL DEFAULT 0,
  flagged_count     INTEGER NOT NULL DEFAULT 0,
  participants_json TEXT NOT NULL DEFAULT '[]',
  UNIQUE (account_id, gm_thrid)
) STRICT;
CREATE INDEX threads_last_date ON threads (account_id, last_date DESC);

CREATE TABLE messages (
  id               INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: clients and secrets hold IDs
  account_id       INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  thread_id        INTEGER REFERENCES threads (id) ON DELETE SET NULL,
  gm_msgid         INTEGER, -- Gmail X-GM-MSGID; NULL elsewhere
  msgid_hdr        TEXT NOT NULL DEFAULT '', -- Message-ID without angle brackets
  in_reply_to      TEXT NOT NULL DEFAULT '',
  refs_json        TEXT NOT NULL DEFAULT '[]',
  subject          TEXT NOT NULL DEFAULT '',
  subject_norm     TEXT NOT NULL DEFAULT '',
  from_name        TEXT NOT NULL DEFAULT '',
  from_addr        TEXT NOT NULL DEFAULT '',
  to_json          TEXT NOT NULL DEFAULT '[]',
  cc_json          TEXT NOT NULL DEFAULT '[]',
  bcc_json         TEXT NOT NULL DEFAULT '[]',
  reply_to_json    TEXT NOT NULL DEFAULT '[]',
  date_hdr         TEXT, -- NULL when the Date header is missing or unparseable
  internal_date    TEXT NOT NULL,
  size             INTEGER NOT NULL DEFAULT 0,
  preview          TEXT NOT NULL DEFAULT '',
  has_attachments  INTEGER NOT NULL DEFAULT 0 CHECK (has_attachments IN (0, 1)),
  list_id          TEXT NOT NULL DEFAULT '',
  list_unsubscribe TEXT NOT NULL DEFAULT '',
  auth_results     TEXT NOT NULL DEFAULT '',
  body_state       TEXT NOT NULL DEFAULT 'headers' CHECK (body_state IN ('headers', 'full')),
  blob_sha         TEXT, -- raw message in the blob store once body_state = 'full'
  seen             INTEGER NOT NULL DEFAULT 0 CHECK (seen IN (0, 1)),
  flagged          INTEGER NOT NULL DEFAULT 0 CHECK (flagged IN (0, 1)),
  answered         INTEGER NOT NULL DEFAULT 0 CHECK (answered IN (0, 1)),
  forwarded        INTEGER NOT NULL DEFAULT 0 CHECK (forwarded IN (0, 1)),
  draft            INTEGER NOT NULL DEFAULT 0 CHECK (draft IN (0, 1)),
  deleted          INTEGER NOT NULL DEFAULT 0 CHECK (deleted IN (0, 1)), -- \Deleted, not yet expunged
  flag_color       INTEGER NOT NULL DEFAULT 0 CHECK (flag_color BETWEEN 0 AND 7), -- 0 unflagged; 1-7 = $MailFlagBit0-2 + 1
  keywords_json    TEXT NOT NULL DEFAULT '[]',
  trackers_blocked INTEGER NOT NULL DEFAULT 0,
  UNIQUE (account_id, gm_msgid)
) STRICT;
CREATE INDEX messages_thread ON messages (thread_id);
CREATE INDEX messages_msgid ON messages (account_id, msgid_hdr);
CREATE INDEX messages_date ON messages (account_id, internal_date DESC);

-- Membership of a message in a mailbox. Generic IMAP has one row per
-- message; Gmail has one per label, with the UID on the All Mail row only.
CREATE TABLE message_mailbox (
  message_id INTEGER NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
  mailbox_id INTEGER NOT NULL REFERENCES mailboxes (id) ON DELETE CASCADE,
  uid        INTEGER, -- NULL for label memberships and optimistic local moves
  modseq     INTEGER,
  pending    INTEGER NOT NULL DEFAULT 0 CHECK (pending IN (0, 1)), -- created by a queued op
  PRIMARY KEY (message_id, mailbox_id)
) STRICT, WITHOUT ROWID;
CREATE UNIQUE INDEX message_mailbox_uid ON message_mailbox (mailbox_id, uid) WHERE uid IS NOT NULL;

-- Every Message-ID seen, including referenced-but-missing ones, mapped to
-- its thread: the JWZ container table (docs/design/sync.md).
CREATE TABLE thread_refs (
  account_id INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  msgid_hdr  TEXT NOT NULL,
  thread_id  INTEGER NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
  PRIMARY KEY (account_id, msgid_hdr)
) STRICT, WITHOUT ROWID;
CREATE INDEX thread_refs_thread ON thread_refs (thread_id);

CREATE TABLE parts (
  message_id   INTEGER NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
  path         TEXT NOT NULL, -- IMAP part specifier, e.g. 1.2
  content_type TEXT NOT NULL,
  charset      TEXT NOT NULL DEFAULT '',
  encoding     TEXT NOT NULL DEFAULT '',
  disposition  TEXT NOT NULL DEFAULT '',
  filename     TEXT NOT NULL DEFAULT '',
  content_id   TEXT NOT NULL DEFAULT '',
  size         INTEGER NOT NULL DEFAULT 0,
  cached       INTEGER NOT NULL DEFAULT 0 CHECK (cached IN (0, 1)),
  PRIMARY KEY (message_id, path)
) STRICT, WITHOUT ROWID;

-- Full-text index; rowid = messages.id. Contentless: text lives in the blob
-- store, and rows are deleted by rowid.
CREATE VIRTUAL TABLE messages_fts USING fts5 (
  subject, from_text, to_text, body_text, attachment_names,
  content = '', contentless_delete = 1,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
);

CREATE TABLE outbox (
  id          INTEGER PRIMARY KEY,
  account_id  INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  identity_id INTEGER REFERENCES identities (id) ON DELETE SET NULL,
  state       TEXT NOT NULL
              CHECK (state IN ('draft', 'queued', 'scheduled', 'sending', 'sent', 'failed', 'cancelled')),
  send_at     TEXT,
  blob_sha    TEXT,
  msgid_hdr   TEXT NOT NULL,
  rcpt_json   TEXT NOT NULL DEFAULT '[]',
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT NOT NULL DEFAULT '',
  draft_uid   INTEGER,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
) STRICT;
CREATE INDEX outbox_due ON outbox (state, send_at);

-- Offline actions awaiting replay against the server (docs/design/sync.md).
CREATE TABLE pending_ops (
  id           INTEGER PRIMARY KEY,
  account_id   INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  kind         TEXT NOT NULL CHECK (kind IN ('flags', 'move', 'copy', 'delete', 'expunge', 'append', 'labels')),
  payload_json TEXT NOT NULL,
  state        TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'failed')),
  attempts     INTEGER NOT NULL DEFAULT 0,
  next_try_at  TEXT NOT NULL,
  last_error   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
) STRICT;
CREATE INDEX pending_ops_due ON pending_ops (account_id, state, next_try_at);

-- The durable event log behind events.subscribe(sinceSeq). AUTOINCREMENT
-- keeps sequence numbers unique after pruning.
CREATE TABLE changes (
  seq   INTEGER PRIMARY KEY AUTOINCREMENT,
  event TEXT NOT NULL,
  data  TEXT NOT NULL,
  at    TEXT NOT NULL
) STRICT;
