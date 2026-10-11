-- Mute and block (ADR-0028): a muted conversation's new mail never
-- notifies; blocked senders' mail never notifies and, with the
-- blockAction setting trash, moves to the Trash as it arrives.
ALTER TABLE threads ADD COLUMN muted INTEGER NOT NULL DEFAULT 0 CHECK (muted IN (0, 1));

CREATE TABLE blocked (
  address TEXT PRIMARY KEY CHECK (address = lower(address) AND address <> ''),
  name    TEXT NOT NULL DEFAULT '',
  at      TEXT NOT NULL
) STRICT, WITHOUT ROWID;
