ALTER TABLE keys ADD COLUMN deleted_at TIMESTAMPTZ;
UPDATE keys SET deleted_at = last_updated WHERE body = 'null'::jsonb;
CREATE INDEX idx_keys_topic_id_deleted_at ON keys(topic_id, deleted_at DESC) WHERE deleted_at IS NOT NULL;
