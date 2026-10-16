-- A Microsoft Graph object's JSON as Graph last sent it, beside the
-- iCalendar or vCard source maild derives from it (ADR-0040). NULL for
-- objects a DAV server or Google Tasks keeps.
ALTER TABLE objects ADD COLUMN remote BLOB;
