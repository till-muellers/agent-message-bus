-- The inviter's agent name (leader or admin); the task is delivered as a direct message from it.
ALTER TABLE invites ADD COLUMN invited_by TEXT NOT NULL DEFAULT '';
