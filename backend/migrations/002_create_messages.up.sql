CREATE TABLE IF NOT EXISTS messages (
    id         BIGSERIAL PRIMARY KEY,
    topic_id   INT NOT NULL REFERENCES topics(id),
    key        TEXT NOT NULL,
    body       JSONB NOT NULL,
    format     TEXT NOT NULL DEFAULT 'json',
    partition  INT NOT NULL,
    offset_id  BIGINT NOT NULL,
    timestamp  TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(topic_id, partition, offset_id)
);

CREATE INDEX idx_messages_topic_id_key ON messages(topic_id, key);
CREATE INDEX idx_messages_topic_id_key_ts ON messages(topic_id, key, timestamp DESC);
CREATE INDEX idx_messages_topic_id_partition_offset ON messages(topic_id, partition, offset_id DESC);
