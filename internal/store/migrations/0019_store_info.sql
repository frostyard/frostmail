-- Facts about this database as a whole. The one row so far is store_id, a
-- random id maild draws on first need: the Flatpak's keyring namespace uses
-- it, so that two databases never share secrets (ADR-0046).
CREATE TABLE store_info (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT, WITHOUT ROWID;
