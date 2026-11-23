DROP INDEX IF EXISTS idx_keys_topic_id_deleted_at;
ALTER TABLE keys DROP COLUMN deleted_at;
