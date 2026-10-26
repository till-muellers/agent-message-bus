-- edge: this entry turned its recipient's mailbox scope from empty to non-empty (or is a critical re-notify).
-- noticed: a channel told the agent (hint, stream) or bus_inbox_read/bus_wait returned the entry.
ALTER TABLE inbox ADD COLUMN edge INTEGER NOT NULL DEFAULT 0 CHECK (edge IN (0, 1));
ALTER TABLE inbox ADD COLUMN noticed INTEGER NOT NULL DEFAULT 0 CHECK (noticed IN (0, 1));
CREATE INDEX inbox_unnoticed ON inbox(agent_id, space_id) WHERE edge = 1 AND noticed = 0;
