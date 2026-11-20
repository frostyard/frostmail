-- The SIZE the account's SMTP server announced at the last submission, in
-- bytes; 0 when not known yet or when the server announces none. maild
-- resets it when the SMTP settings change (docs/design/send.md, Attachments).
ALTER TABLE accounts ADD COLUMN smtp_max_size INTEGER NOT NULL DEFAULT 0 CHECK (smtp_max_size >= 0);
