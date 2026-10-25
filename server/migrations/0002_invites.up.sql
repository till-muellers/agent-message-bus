CREATE TABLE invites (
  hash TEXT PRIMARY KEY,
  space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  task TEXT, -- first prompt for the member, returned once on accept
  expires_at INTEGER NOT NULL -- unix seconds, so SQL can compare
);

CREATE INDEX invites_space_name ON invites (space_id, name);
