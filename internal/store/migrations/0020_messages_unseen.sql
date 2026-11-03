-- Serves the unread count of every inbox, which maild reports to lodge as
-- the rail's badge (lodge ADR-0020; docs/specs/lodge-app.md, /health):
-- unread and undeleted messages, in any mailbox the app gives the inbox
-- role.
CREATE INDEX messages_unseen ON messages (id) WHERE seen = 0 AND deleted = 0;
