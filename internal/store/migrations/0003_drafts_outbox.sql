-- Drafts, the outbox's send states and address suggestions
-- (docs/design/send.md, ADR-0010).

-- A draft is structured content the compose window edits; its built message
-- is copied to the Drafts mailbox (server_uid) after a quiet period.
CREATE TABLE drafts (
  id           INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: windows hold IDs
  account_id   INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  kind         TEXT NOT NULL DEFAULT 'new' CHECK (kind IN ('new', 'reply', 'replyall', 'forward')),
  source_id    INTEGER REFERENCES messages (id) ON DELETE SET NULL,
  content_json TEXT NOT NULL, -- identity, to, cc, bcc, subject, html (store.DraftContent)
  msgid_hdr    TEXT NOT NULL, -- chosen at creation; the sent message keeps it
  in_reply_to  TEXT NOT NULL DEFAULT '',
  refs_json    TEXT NOT NULL DEFAULT '[]',
  server_uid   INTEGER,       -- the copy in the Drafts mailbox; NULL before the first save
  saved_at     TEXT,          -- when that copy was written; NULL before the first save
  updated_at   TEXT NOT NULL,
  created_at   TEXT NOT NULL
) STRICT;
CREATE INDEX drafts_account ON drafts (account_id, updated_at);

CREATE TABLE draft_attachments (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  draft_id     INTEGER NOT NULL REFERENCES drafts (id) ON DELETE CASCADE,
  blob_id      TEXT NOT NULL,
  filename     TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size         INTEGER NOT NULL
) STRICT;
CREATE INDEX draft_attachments_draft ON draft_attachments (draft_id);

-- 0001's outbox predates the send design: no 'accepted' state, no draft
-- link, no display fields. Nothing used it, so it is rebuilt.
DROP TABLE outbox;
CREATE TABLE outbox (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id  INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  draft_id    INTEGER REFERENCES drafts (id) ON DELETE SET NULL,
  state       TEXT NOT NULL CHECK (state IN ('queued', 'sending', 'accepted', 'sent', 'failed')),
  send_at     TEXT NOT NULL,
  blob_id     TEXT NOT NULL,     -- the built message
  msgid_hdr   TEXT NOT NULL,
  from_addr   TEXT NOT NULL,     -- the envelope sender
  rcpt_json   TEXT NOT NULL,     -- the envelope recipients: To, Cc and Bcc
  subject     TEXT NOT NULL DEFAULT '',
  to_json     TEXT NOT NULL DEFAULT '[]',
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
) STRICT;
CREATE INDEX outbox_due ON outbox (account_id, state, send_at);

-- Addresses seen in stored and sent mail, for recipient suggestions.
CREATE TABLE addresses (
  address   TEXT PRIMARY KEY, -- lowercased
  name      TEXT NOT NULL DEFAULT '',
  count     INTEGER NOT NULL DEFAULT 0,
  last_seen TEXT NOT NULL
) STRICT, WITHOUT ROWID;

INSERT INTO addresses (address, name, count, last_seen)
SELECT lower(addr), max(name), count(*), max(seen) FROM (
  SELECT from_addr AS addr, from_name AS name, internal_date AS seen FROM messages WHERE from_addr != ''
  UNION ALL
  SELECT json_extract(j.value, '$.addr'), coalesce(json_extract(j.value, '$.name'), ''), m.internal_date
    FROM messages m, json_each(m.to_json) j
  UNION ALL
  SELECT json_extract(j.value, '$.addr'), coalesce(json_extract(j.value, '$.name'), ''), m.internal_date
    FROM messages m, json_each(m.cc_json) j
) WHERE addr IS NOT NULL AND addr != ''
GROUP BY lower(addr);

-- Every account sends as itself by default.
INSERT INTO identities (account_id, name, email, is_default)
SELECT a.id, a.display_name, a.email, 1 FROM accounts a
WHERE NOT EXISTS (SELECT 1 FROM identities i WHERE i.account_id = a.id);
