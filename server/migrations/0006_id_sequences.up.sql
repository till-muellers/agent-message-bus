-- threads.id is AUTOINCREMENT since 0005_autoincrement_ids, but its sqlite_sequence started at the highest
-- thread id left then. A thread removed before that is still named by thread_removed inbox entries
-- (inbox.thread_id is no foreign key), so its id could be handed out again: raise the sequence past it.
-- spaces need nothing: every column holding a space id is a foreign key (checked before each migration
-- commits), so a deleted space leaves no id behind.
INSERT INTO sqlite_sequence (name, seq) SELECT 'threads', 0
  WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'threads');
UPDATE sqlite_sequence SET seq = MAX(seq, (SELECT COALESCE(MAX(thread_id), 0) FROM inbox))
  WHERE name = 'threads';
