CREATE TABLE IF NOT EXISTS keys (
    id            SERIAL PRIMARY KEY,
    topic_id      INT NOT NULL REFERENCES topics(id),
    key           TEXT NOT NULL,
    message_count INT NOT NULL DEFAULT 1,
    partition     INT NOT NULL DEFAULT 0,
    offset_id     BIGINT NOT NULL DEFAULT 0,
    last_updated  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(topic_id, key)
);

CREATE INDEX idx_keys_topic_id_last_updated ON keys(topic_id, last_updated DESC);
CREATE INDEX idx_keys_topic_id_offset ON keys(topic_id, offset_id DESC);

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_keys_key_trgm ON keys USING gin (key gin_trgm_ops);
