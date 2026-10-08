-- The sync window (ADR-0016): maild keeps the messages that arrived in the
-- last sync_days days; 0 keeps every message.
ALTER TABLE accounts ADD COLUMN sync_days INTEGER NOT NULL DEFAULT 0 CHECK (sync_days >= 0);
