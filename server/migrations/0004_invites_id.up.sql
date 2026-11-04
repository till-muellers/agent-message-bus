-- Opaque handle of one pending invite code (16 lowercase hex chars), listed and cancelled by the web GUI.
ALTER TABLE invites ADD COLUMN id TEXT NOT NULL DEFAULT '';
UPDATE invites SET id = lower(hex(randomblob(8)));
CREATE UNIQUE INDEX invites_id ON invites(id);
