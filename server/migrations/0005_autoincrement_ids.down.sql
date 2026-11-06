-- Back to INTEGER PRIMARY KEY without AUTOINCREMENT for spaces and threads (ids of deleted rows may be
-- reused again). Run with foreign keys off, like the runner applies the up migration.
CREATE TABLE spaces_old (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
INSERT INTO spaces_old (id, name, created_by, created_at) SELECT id, name, created_by, created_at FROM spaces;
DROP TABLE spaces;
ALTER TABLE spaces_old RENAME TO spaces;
CREATE TABLE threads_old (id INTEGER PRIMARY KEY,
  blackboard_id INTEGER NOT NULL REFERENCES blackboards(id) ON DELETE CASCADE,
  title TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
INSERT INTO threads_old (id, blackboard_id, title, created_by, created_at)
  SELECT id, blackboard_id, title, created_by, created_at FROM threads;
DROP TABLE threads;
ALTER TABLE threads_old RENAME TO threads;
DELETE FROM sqlite_sequence WHERE name IN ('spaces', 'threads');
