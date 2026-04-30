ALTER TABLE messages DROP CONSTRAINT messages_topic_id_partition_offset_id_key;
ALTER TABLE messages DROP CONSTRAINT messages_pkey;
ALTER TABLE messages DROP COLUMN id;
ALTER TABLE messages ADD PRIMARY KEY (topic_id, partition, offset_id);
