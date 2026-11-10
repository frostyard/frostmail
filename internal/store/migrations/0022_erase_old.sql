-- Erasing old Trash and Junk mail (ADR-0051, plan 0014 Phase 4):
-- message_mailbox gains added_at, the time maild first saw the message in
-- that mailbox, the age the erase pass uses. IMAP keeps no such time
-- (INTERNALDATE is the message's own date), so the rows that exist now get
-- the migration's time: nothing is erased until a full period after the
-- upgrade. A NULL added_at is never erased.
ALTER TABLE message_mailbox ADD COLUMN added_at TEXT;

UPDATE message_mailbox SET added_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now');

-- The two per-account settings, as Mail.app has them: 0 is Never, the
-- default; 1, 7 and 30 days are the choices.
ALTER TABLE accounts ADD COLUMN erase_trash_days INTEGER NOT NULL DEFAULT 0 CHECK (erase_trash_days IN (0, 1, 7, 30));
ALTER TABLE accounts ADD COLUMN erase_junk_days INTEGER NOT NULL DEFAULT 0 CHECK (erase_junk_days IN (0, 1, 7, 30));
