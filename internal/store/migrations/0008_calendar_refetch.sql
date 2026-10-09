-- Calendars synced before events were indexed (T-0069) hold objects with
-- no events: drop them and the collections' sync state, so the next pass
-- fetches and indexes every event. No calendar writes were possible yet,
-- so nothing local is lost.
DELETE FROM objects WHERE collection_id IN (SELECT id FROM collections WHERE kind = 'calendar');
UPDATE collections SET sync_token = '', ctag = '' WHERE kind = 'calendar';
