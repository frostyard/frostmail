-- A message's display copy (ADR-0037): the parts a reader shows, fetched
-- when it is opened or prefetched, in the blob store under this hash.
-- blob_sha and body_state = 'full' keep meaning the whole message.
ALTER TABLE messages ADD COLUMN display_sha TEXT;
