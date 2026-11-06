-- spaces.id and threads.id are never reused (AUTOINCREMENT). A reused id let a member token, an inbox
-- scope or a stream of a deleted space match the next space, and an old threadId (inbox entry, report,
-- web link) open another thread. Table rebuild: the runner applies migrations with foreign keys off.
CREATE TABLE spaces_new (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE,
  created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
INSERT INTO spaces_new (id, name, created_by, created_at) SELECT id, name, created_by, created_at FROM spaces;
DROP TABLE spaces;
ALTER TABLE spaces_new RENAME TO spaces;
CREATE TABLE threads_new (id INTEGER PRIMARY KEY AUTOINCREMENT,
  blackboard_id INTEGER NOT NULL REFERENCES blackboards(id) ON DELETE CASCADE,
  title TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
INSERT INTO threads_new (id, blackboard_id, title, created_by, created_at)
  SELECT id, blackboard_id, title, created_by, created_at FROM threads;
DROP TABLE threads;
ALTER TABLE threads_new RENAME TO threads;
