ALTER TABLE keys ADD COLUMN body JSONB;
CREATE INDEX idx_keys_body_gin ON keys USING gin (body jsonb_path_ops);
