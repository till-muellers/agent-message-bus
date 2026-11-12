-- The post a thread_post entry announces: its id, as bus_add_post returns it and bus_read_thread lists it.
ALTER TABLE inbox ADD COLUMN post_id INTEGER;
