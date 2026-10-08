-- Read-only accounts, notifications and OAuth clients
-- (docs/design/accounts.md, ADR-0011).

-- read_only: maild makes no server changes for the account.
ALTER TABLE accounts ADD COLUMN read_only INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1));
-- notify: new inbox mail shows a desktop notification.
ALTER TABLE accounts ADD COLUMN notify INTEGER NOT NULL DEFAULT 1 CHECK (notify IN (0, 1));

-- The user's own OAuth clients until Frostmail ships verified ones (M6).
-- The client secret is in the secret store (oauth/<provider>/client-secret).
CREATE TABLE oauth_clients (
  provider  TEXT PRIMARY KEY CHECK (provider IN ('google', 'microsoft')),
  client_id TEXT NOT NULL
) STRICT;
