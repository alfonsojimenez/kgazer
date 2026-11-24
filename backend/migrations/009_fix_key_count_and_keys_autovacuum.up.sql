UPDATE topics t SET key_count = COALESCE((SELECT COUNT(*) FROM keys k WHERE k.topic_id = t.id), 0);
ALTER TABLE keys SET (autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02);
