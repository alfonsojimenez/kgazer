ALTER TABLE messages DROP CONSTRAINT messages_pkey;
ALTER TABLE messages ADD COLUMN id BIGSERIAL;
ALTER TABLE messages ADD PRIMARY KEY (id);
ALTER TABLE messages ADD UNIQUE (topic_id, partition, offset_id);
