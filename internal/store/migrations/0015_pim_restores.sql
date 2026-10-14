-- Restores (ADR-0032): what each contact or event write replaced, kept a
-- day so people.restore and calendar.restore can write it back.
CREATE TABLE pim_restores (
  id         TEXT PRIMARY KEY, -- opaque, random: the restoreId the API returns
  account_id INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
  created_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;
CREATE INDEX pim_restores_created ON pim_restores (created_at);

-- One row per object a write touched, in the order it touched them.
CREATE TABLE pim_restore_objects (
  restore_id    TEXT NOT NULL REFERENCES pim_restores (id) ON DELETE CASCADE,
  position      INTEGER NOT NULL,
  collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  href          TEXT NOT NULL,
  uid           TEXT NOT NULL DEFAULT '',
  raw           BLOB, -- the source before the write; NULL when the write created the object
  PRIMARY KEY (restore_id, position)
) STRICT, WITHOUT ROWID;
