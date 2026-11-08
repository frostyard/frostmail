-- M5's local text preferences gain named snippets (plan 0014 Phase 3).
CREATE TABLE snippets (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  html       TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX snippets_name_nocase ON snippets (name COLLATE NOCASE);
