CREATE TABLE IF NOT EXISTS topics (
    id              SERIAL PRIMARY KEY,
    cluster         TEXT NOT NULL,
    name            TEXT NOT NULL,
    partitions      INT NOT NULL DEFAULT 0,
    compacted       BOOLEAN NOT NULL DEFAULT false,
    message_count   INT NOT NULL DEFAULT 0,
    key_count       INT NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    message_format  TEXT NOT NULL DEFAULT 'unknown',
    kafka_topic_id  TEXT NOT NULL DEFAULT '',
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(cluster, name)
);

CREATE INDEX idx_topics_cluster ON topics(cluster);
