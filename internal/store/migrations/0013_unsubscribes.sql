-- Unsubscribe (ADR-0027): the lists the user left, by List-Id, or by
-- message for mail from no list, so later mail says so.
CREATE TABLE unsubscribes (
  key TEXT PRIMARY KEY, -- 'list:' and the List-Id, or 'message:' and the message ID
  at  TEXT NOT NULL
) STRICT, WITHOUT ROWID;
