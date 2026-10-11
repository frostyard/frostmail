-- Use This Mailbox For (M5, docs/design/organize.md, Mailboxes): an
-- account's own choice of its drafts, sent, junk, trash or archive mailbox,
-- by path, over the role the server names. ReplaceMailboxes applies it to
-- every listing; a rename carries it along.
CREATE TABLE mailbox_roles (
  account_id INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  role       TEXT NOT NULL CHECK (role IN ('drafts', 'sent', 'junk', 'trash', 'archive')),
  path       TEXT NOT NULL,
  PRIMARY KEY (account_id, role)
) STRICT, WITHOUT ROWID;

-- Mailbox operations are offline actions too (mbcreate, mbrename,
-- mbdelete). SQLite cannot change a CHECK, so pending_ops is rebuilt with
-- the new kinds, and pending_op_messages with it, keeping every row: the
-- old table is renamed (its references follow), the new ones filled, the
-- old ones dropped.
ALTER TABLE pending_ops RENAME TO pending_ops_0011;
CREATE TABLE pending_ops (
  id           INTEGER PRIMARY KEY,
  account_id   INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  kind         TEXT NOT NULL CHECK (kind IN ('flags', 'move', 'copy', 'delete', 'expunge', 'append', 'labels',
                                             'mbcreate', 'mbrename', 'mbdelete')),
  payload_json TEXT NOT NULL,
  state        TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'failed')),
  attempts     INTEGER NOT NULL DEFAULT 0,
  next_try_at  TEXT NOT NULL,
  last_error   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
) STRICT;
INSERT INTO pending_ops SELECT * FROM pending_ops_0011;
CREATE TABLE pending_op_messages_0012 (
  op_id      INTEGER NOT NULL REFERENCES pending_ops (id) ON DELETE CASCADE,
  message_id INTEGER NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
  PRIMARY KEY (op_id, message_id)
) STRICT, WITHOUT ROWID;
INSERT INTO pending_op_messages_0012 SELECT * FROM pending_op_messages;
DROP TABLE pending_op_messages;
DROP TABLE pending_ops_0011;
ALTER TABLE pending_op_messages_0012 RENAME TO pending_op_messages;
CREATE INDEX pending_ops_due ON pending_ops (account_id, state, next_try_at);
CREATE INDEX pending_op_messages_message ON pending_op_messages (message_id);
