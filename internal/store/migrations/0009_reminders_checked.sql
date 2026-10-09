-- When the reminder scheduler last checked for due alarms. After a
-- restart it catches up only on alarms that came due while maild was
-- stopped, not on every alarm of the past day (an alarm due before its
-- calendar was synced never reminds).
CREATE TABLE reminders_checked (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  checked_at TEXT NOT NULL
) STRICT;
