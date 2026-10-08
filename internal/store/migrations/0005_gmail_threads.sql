-- Gmail's X-GM-THRID per message, so threads follow Gmail's conversations
-- (ADR-0012). NULL for other accounts.
ALTER TABLE messages ADD COLUMN gm_thrid INTEGER;
