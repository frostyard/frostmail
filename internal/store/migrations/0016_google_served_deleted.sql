-- Google's sync-collection calls some objects deleted that it still serves
-- (providers.DAV.ServesDeleted), so Gmail calendars synced before a pass
-- asked for those lack them. Drop their sync state: the next pass lists
-- each again, fetching only what is missing or has another ETag.
UPDATE collections SET sync_token = '', ctag = ''
WHERE kind = 'calendar' AND account_id IN (SELECT id FROM accounts WHERE kind = 'gmail');
