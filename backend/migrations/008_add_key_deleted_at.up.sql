ALTER TABLE keys ADD COLUMN deleted_at TIMESTAMPTZ;
-- CASE forces the size check first so large bodies are never detoasted; a JSON null body is a few bytes.
UPDATE keys SET deleted_at = last_updated WHERE CASE WHEN pg_column_size(body) < 16 THEN body = 'null'::jsonb ELSE false END;
CREATE INDEX idx_keys_topic_id_deleted_at ON keys(topic_id, deleted_at DESC) WHERE deleted_at IS NOT NULL;
