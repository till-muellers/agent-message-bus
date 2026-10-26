-- Initial schema after the alpha reset. Not reversible: delete the database file to reset.
CREATE TABLE agents (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK (kind IN ('admin','leader','member')), created_at TEXT NOT NULL);
CREATE TABLE spaces (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
CREATE TABLE tokens (hash TEXT PRIMARY KEY,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  space_id INTEGER REFERENCES spaces(id) ON DELETE CASCADE, -- NULL = leader or admin token
  created_at TEXT NOT NULL);
CREATE TABLE participants (space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('admin','leader','member')), joined_at TEXT NOT NULL,
  PRIMARY KEY (space_id, agent_id));
CREATE TABLE topics (id INTEGER PRIMARY KEY, space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id),
  created_at TEXT NOT NULL, closed_at TEXT);
CREATE UNIQUE INDEX topics_open_name ON topics(space_id, name) WHERE closed_at IS NULL;
CREATE TABLE topic_subscriptions (topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE, PRIMARY KEY (topic_id, agent_id));
CREATE TABLE blackboards (id INTEGER PRIMARY KEY, space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL, owner_id INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL,
  UNIQUE (space_id, name));
CREATE TABLE threads (id INTEGER PRIMARY KEY, blackboard_id INTEGER NOT NULL REFERENCES blackboards(id) ON DELETE CASCADE,
  title TEXT NOT NULL, created_by INTEGER NOT NULL REFERENCES agents(id), created_at TEXT NOT NULL);
CREATE TABLE posts (id INTEGER PRIMARY KEY, thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  author_id INTEGER NOT NULL REFERENCES agents(id), body TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE thread_subscriptions (thread_id INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE, PRIMARY KEY (thread_id, agent_id));
CREATE TABLE inbox (id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  type TEXT NOT NULL, from_name TEXT NOT NULL, critical INTEGER NOT NULL DEFAULT 0,
  topic TEXT, blackboard TEXT, thread_id INTEGER, thread_title TEXT, body TEXT, created_at TEXT NOT NULL);
CREATE INDEX inbox_agent ON inbox(agent_id, id);
CREATE TABLE invites (
  hash TEXT PRIMARY KEY,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  task TEXT, -- the member's task
  expires_at INTEGER NOT NULL -- unix seconds, so SQL can compare
);
CREATE INDEX invites_space_name ON invites (space_id, name);
