-- M5: preferences, smart mailboxes, rules, Remind Me and Send Later
-- (docs/design/organize.md; ADRs 0023-0027). Additive: nothing here acts
-- on mail until the user makes a rule, a reminder or a scheduled send.

-- Scalar preferences (ADR-0026), one row per setting; a missing row is the
-- default.
CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE vips (
  address  TEXT PRIMARY KEY CHECK (address = lower(address) AND address <> ''),
  name     TEXT NOT NULL DEFAULT '',
  added_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

-- Conditions are JSON with a version (ADR-0023), checked before they are
-- stored, so every stored row compiles.
CREATE TABLE smart_mailboxes (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT NOT NULL,
  position        INTEGER NOT NULL,
  conditions_json TEXT NOT NULL,
  include_trash   INTEGER NOT NULL DEFAULT 0 CHECK (include_trash IN (0, 1)),
  include_sent    INTEGER NOT NULL DEFAULT 0 CHECK (include_sent IN (0, 1))
) STRICT;

CREATE TABLE rules (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT NOT NULL,
  position        INTEGER NOT NULL,
  enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  conditions_json TEXT NOT NULL,
  actions_json    TEXT NOT NULL
) STRICT;

-- New inbox mail waits for the rules (ADR-0024): set in the transaction
-- that inserts it, cleared in the one that applies the rules.
ALTER TABLE messages ADD COLUMN rules_waiting INTEGER NOT NULL DEFAULT 0 CHECK (rules_waiting IN (0, 1));
CREATE INDEX messages_rules_waiting ON messages (account_id) WHERE rules_waiting = 1;

-- Remind Me (ADR-0025): a pending reminder per message, and the date views
-- order by, which is the arrival date until a reminder fires.
CREATE TABLE message_reminders (
  message_id INTEGER PRIMARY KEY REFERENCES messages (id) ON DELETE CASCADE,
  remind_at  TEXT NOT NULL
) STRICT;
CREATE INDEX message_reminders_due ON message_reminders (remind_at);

ALTER TABLE messages ADD COLUMN list_date TEXT NOT NULL DEFAULT '';
UPDATE messages SET list_date = internal_date;
CREATE INDEX messages_list_date ON messages (account_id, list_date DESC);
CREATE TRIGGER messages_list_date AFTER INSERT ON messages WHEN NEW.list_date = ''
BEGIN
  UPDATE messages SET list_date = NEW.internal_date WHERE id = NEW.id;
END;

-- Send Later (ADR-0025): queued for a chosen time, not in its undo window.
ALTER TABLE outbox ADD COLUMN scheduled INTEGER NOT NULL DEFAULT 0 CHECK (scheduled IN (0, 1));
