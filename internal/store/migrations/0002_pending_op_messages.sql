-- The messages each queued action covers. While an action is queued, a
-- reconcile pass leaves those messages' flags alone, so the server's older
-- state cannot undo the user's change before the replay
-- (docs/design/sync.md, offline actions).
CREATE TABLE pending_op_messages (
  op_id      INTEGER NOT NULL REFERENCES pending_ops (id) ON DELETE CASCADE,
  message_id INTEGER NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
  PRIMARY KEY (op_id, message_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX pending_op_messages_message ON pending_op_messages (message_id);
