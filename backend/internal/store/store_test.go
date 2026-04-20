package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration tests")
	}

	ctx := context.Background()
	s, err := New(ctx, url)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS topics (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topics_cluster ON topics(cluster)`,
		`CREATE TABLE IF NOT EXISTS messages (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_topic_id_key ON messages(topic_id, key)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_topic_id_key_ts ON messages(topic_id, key, timestamp DESC)`,
		`CREATE TABLE IF NOT EXISTS keys (
			id            SERIAL PRIMARY KEY,
			topic_id      INT NOT NULL REFERENCES topics(id),
			key           TEXT NOT NULL,
			message_count INT NOT NULL DEFAULT 1,
			partition     INT NOT NULL DEFAULT 0,
			offset_id     BIGINT NOT NULL DEFAULT 0,
			last_updated  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(topic_id, key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_keys_topic_id_last_updated ON keys(topic_id, last_updated DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_keys_topic_id_offset ON keys(topic_id, offset_id DESC)`,
	}

	for _, m := range migrations {
		if _, err := s.writePool.Exec(ctx, m); err != nil {
			t.Fatalf("failed to run migration: %v", err)
		}
	}

	cleanDB(t, s)

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

func cleanDB(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.writePool.Exec(ctx, "TRUNCATE keys, messages, topics CASCADE"); err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}

func TestUpsertTopic(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()

	t.Run("insert new topic", func(t *testing.T) {
		cleanDB(t, s)
		id, recreated, err := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
		if err != nil {
			t.Fatalf("UpsertTopic: %v", err)
		}
		if id == 0 {
			t.Error("expected non-zero id")
		}
		if recreated {
			t.Error("expected recreated=false for new topic")
		}
	})

	t.Run("upsert same data", func(t *testing.T) {
		cleanDB(t, s)
		id1, _, err := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
		if err != nil {
			t.Fatalf("first upsert: %v", err)
		}

		id2, recreated, err := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
		if err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if id1 != id2 {
			t.Errorf("expected same id, got %d and %d", id1, id2)
		}
		if recreated {
			t.Error("expected recreated=false for same kafka_topic_id")
		}
	})

	t.Run("upsert with different kafka_topic_id", func(t *testing.T) {
		cleanDB(t, s)
		_, _, err := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
		if err != nil {
			t.Fatalf("first upsert: %v", err)
		}

		_, recreated, err := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-002")
		if err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if !recreated {
			t.Error("expected recreated=true for different kafka_topic_id")
		}
	})
}

func TestListTopicsByCluster(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	s.UpsertTopic(ctx, "cluster-a", "users", 1, false, "tid-002")
	s.UpsertTopic(ctx, "cluster-b", "events", 5, false, "tid-003")

	topics, err := s.ListTopicsByCluster(ctx, "cluster-a")
	if err != nil {
		t.Fatalf("ListTopicsByCluster: %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}

	if topics[0].Name != "orders" || topics[1].Name != "users" {
		t.Errorf("expected sorted by name [orders, users], got [%s, %s]", topics[0].Name, topics[1].Name)
	}
	if topics[0].KafkaTopicID != "tid-001" {
		t.Errorf("expected kafka_topic_id=tid-001, got %s", topics[0].KafkaTopicID)
	}
}

func TestListTopics(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	s.UpsertTopic(ctx, "cluster-a", "alpha", 1, false, "")
	time.Sleep(10 * time.Millisecond)
	s.UpsertTopic(ctx, "cluster-a", "beta", 1, false, "")
	time.Sleep(10 * time.Millisecond)
	s.UpsertTopic(ctx, "cluster-b", "gamma", 1, false, "")

	t.Run("list all desc", func(t *testing.T) {
		topics, err := s.ListTopics(ctx, "", "desc")
		if err != nil {
			t.Fatalf("ListTopics: %v", err)
		}
		if len(topics) != 3 {
			t.Fatalf("expected 3 topics, got %d", len(topics))
		}
		if topics[0].Name != "gamma" {
			t.Errorf("expected first topic gamma (most recent), got %s", topics[0].Name)
		}
	})

	t.Run("list by cluster", func(t *testing.T) {
		topics, err := s.ListTopics(ctx, "cluster-a", "desc")
		if err != nil {
			t.Fatalf("ListTopics: %v", err)
		}
		if len(topics) != 2 {
			t.Fatalf("expected 2 topics, got %d", len(topics))
		}
	})

	t.Run("list asc", func(t *testing.T) {
		topics, err := s.ListTopics(ctx, "", "asc")
		if err != nil {
			t.Fatalf("ListTopics: %v", err)
		}
		if len(topics) != 3 {
			t.Fatalf("expected 3 topics, got %d", len(topics))
		}
		if topics[0].Name != "alpha" {
			t.Errorf("expected first topic alpha (oldest), got %s", topics[0].Name)
		}
	})
}

func TestGetTopicDetail(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")

	t.Run("existing topic", func(t *testing.T) {
		detail, err := s.GetTopicDetail(ctx, "cluster-a", "orders")
		if err != nil {
			t.Fatalf("GetTopicDetail: %v", err)
		}
		if detail.Name != "orders" {
			t.Errorf("expected name=orders, got %s", detail.Name)
		}
		if detail.Cluster != "cluster-a" {
			t.Errorf("expected cluster=cluster-a, got %s", detail.Cluster)
		}
		if detail.Partitions != 3 {
			t.Errorf("expected partitions=3, got %d", detail.Partitions)
		}
		if !detail.Compacted {
			t.Error("expected compacted=true")
		}
	})

	t.Run("missing topic", func(t *testing.T) {
		_, err := s.GetTopicDetail(ctx, "cluster-a", "nonexistent")
		if err == nil {
			t.Fatal("expected error for missing topic")
		}
	})
}

func TestSaveMessageBatch(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	now := time.Now()

	t.Run("save messages", func(t *testing.T) {
		msgs := []PendingMessage{
			{TopicID: id, Key: "key-1", Body: []byte(`{"a":1}`), Format: "json", Partition: 0, Offset: 0, Timestamp: now},
			{TopicID: id, Key: "key-2", Body: []byte(`{"b":2}`), Format: "json", Partition: 0, Offset: 1, Timestamp: now},
			{TopicID: id, Key: "key-3", Body: []byte(`{"c":3}`), Format: "json", Partition: 1, Offset: 0, Timestamp: now},
		}
		count, err := s.SaveMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("SaveMessageBatch: %v", err)
		}
		if count != 3 {
			t.Errorf("expected 3 rows affected, got %d", count)
		}
	})

	t.Run("duplicate messages ignored", func(t *testing.T) {
		msgs := []PendingMessage{
			{TopicID: id, Key: "key-1", Body: []byte(`{"a":1}`), Format: "json", Partition: 0, Offset: 0, Timestamp: now},
		}
		count, err := s.SaveMessageBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("SaveMessageBatch: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 rows affected for duplicate, got %d", count)
		}
	})

	t.Run("empty batch", func(t *testing.T) {
		count, err := s.SaveMessageBatch(ctx, nil)
		if err != nil {
			t.Fatalf("SaveMessageBatch: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 for empty batch, got %d", count)
		}
	})
}

func TestListKeys(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	now := time.Now()

	s.UpsertKey(ctx, id, "user-100", 0, 10, now)
	s.UpsertKey(ctx, id, "user-200", 0, 20, now.Add(time.Second))
	s.UpsertKey(ctx, id, "order-300", 1, 30, now.Add(2*time.Second))
	s.IncrementTopicStats(ctx, id, 3, 3, now.Add(2*time.Second), "json")

	t.Run("list all keys", func(t *testing.T) {
		keys, total, err := s.ListKeys(ctx, "cluster-a", "orders", "", "", "desc", nil, 0, 1, 50)
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if total != 3 {
			t.Errorf("expected total=3, got %d", total)
		}
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d", len(keys))
		}
	})

	t.Run("search filter", func(t *testing.T) {
		keys, total, err := s.ListKeys(ctx, "cluster-a", "orders", "user", "", "desc", nil, 0, 1, 50)
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if total != 2 {
			t.Errorf("expected total=2 for search=user, got %d", total)
		}
		if len(keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("partition filter", func(t *testing.T) {
		keys, total, err := s.ListKeys(ctx, "cluster-a", "orders", "", "", "desc", []int{1}, 0, 1, 50)
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if total != 1 {
			t.Errorf("expected total=1 for partition=1, got %d", total)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key, got %d", len(keys))
		}
	})

	t.Run("offset filter", func(t *testing.T) {
		keys, total, err := s.ListKeys(ctx, "cluster-a", "orders", "", "", "desc", nil, 20, 1, 50)
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if total != 1 {
			t.Errorf("expected total=1 for offset=20, got %d", total)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key, got %d", len(keys))
		}
	})

	t.Run("sort by offset", func(t *testing.T) {
		keys, _, err := s.ListKeys(ctx, "cluster-a", "orders", "", "offset", "asc", nil, 0, 1, 50)
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if len(keys) < 2 {
			t.Fatal("expected at least 2 keys")
		}
		if keys[0].Offset > keys[1].Offset {
			t.Errorf("expected ascending offset order, got %d > %d", keys[0].Offset, keys[1].Offset)
		}
	})

	t.Run("topic not found", func(t *testing.T) {
		_, _, err := s.ListKeys(ctx, "cluster-a", "nonexistent", "", "", "desc", nil, 0, 1, 50)
		if err != ErrTopicNotFound {
			t.Errorf("expected ErrTopicNotFound, got %v", err)
		}
	})
}

func TestGetMaxOffsets(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	now := time.Now()

	msgs := []PendingMessage{
		{TopicID: id, Key: "k1", Body: []byte(`{}`), Format: "json", Partition: 0, Offset: 5, Timestamp: now},
		{TopicID: id, Key: "k2", Body: []byte(`{}`), Format: "json", Partition: 0, Offset: 10, Timestamp: now},
		{TopicID: id, Key: "k3", Body: []byte(`{}`), Format: "json", Partition: 1, Offset: 3, Timestamp: now},
	}
	s.SaveMessageBatch(ctx, msgs)

	offsets, err := s.GetMaxOffsets(ctx, id)
	if err != nil {
		t.Fatalf("GetMaxOffsets: %v", err)
	}
	if len(offsets) != 2 {
		t.Fatalf("expected 2 partitions, got %d", len(offsets))
	}

	m := make(map[int32]int64)
	for _, po := range offsets {
		m[po.Partition] = po.Offset
	}

	if m[0] != 10 {
		t.Errorf("expected max offset for partition 0 = 10, got %d", m[0])
	}
	if m[1] != 3 {
		t.Errorf("expected max offset for partition 1 = 3, got %d", m[1])
	}
}

func TestPurgeTopicData(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	now := time.Now()

	msgs := []PendingMessage{
		{TopicID: id, Key: "k1", Body: []byte(`{}`), Format: "json", Partition: 0, Offset: 0, Timestamp: now},
	}
	s.SaveMessageBatch(ctx, msgs)
	s.UpsertKey(ctx, id, "k1", 0, 0, now)
	s.IncrementTopicStats(ctx, id, 1, 1, now, "json")

	err := s.PurgeTopicData(ctx, id)
	if err != nil {
		t.Fatalf("PurgeTopicData: %v", err)
	}

	offsets, _ := s.GetMaxOffsets(ctx, id)
	if len(offsets) != 0 {
		t.Error("expected no messages after purge")
	}

	keys, total, _ := s.ListKeys(ctx, "cluster-a", "orders", "", "", "desc", nil, 0, 1, 50)
	if total != 0 || len(keys) != 0 {
		t.Error("expected no keys after purge")
	}

	detail, err := s.GetTopicDetail(ctx, "cluster-a", "orders")
	if err != nil {
		t.Fatalf("topic should still exist after purge: %v", err)
	}
	if detail.ConsumedMessages != 0 {
		t.Errorf("expected message_count=0, got %d", detail.ConsumedMessages)
	}
	if detail.UniqueKeys != 0 {
		t.Errorf("expected key_count=0, got %d", detail.UniqueKeys)
	}
}

func TestDeleteTopic(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")
	now := time.Now()

	s.SaveMessageBatch(ctx, []PendingMessage{
		{TopicID: id, Key: "k1", Body: []byte(`{}`), Format: "json", Partition: 0, Offset: 0, Timestamp: now},
	})
	s.UpsertKey(ctx, id, "k1", 0, 0, now)

	err := s.DeleteTopic(ctx, id)
	if err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}

	_, err = s.GetTopicDetail(ctx, "cluster-a", "orders")
	if err == nil {
		t.Error("expected error after DeleteTopic")
	}
}

func TestRemoveStaleTopics(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	s.UpsertTopic(ctx, "cluster-a", "active-topic", 1, false, "")
	s.UpsertTopic(ctx, "cluster-a", "stale-topic", 1, false, "")

	removed, err := s.RemoveStaleTopics(ctx, "cluster-a", []string{"active-topic"})
	if err != nil {
		t.Fatalf("RemoveStaleTopics: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(removed))
	}
	if removed[0] != "stale-topic" {
		t.Errorf("expected stale-topic removed, got %s", removed[0])
	}

	topics, _ := s.ListTopicsByCluster(ctx, "cluster-a")
	if len(topics) != 1 {
		t.Fatalf("expected 1 remaining topic, got %d", len(topics))
	}
	if topics[0].Name != "active-topic" {
		t.Errorf("expected active-topic remaining, got %s", topics[0].Name)
	}
}

func TestGetClusterStats(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	id1, _, _ := s.UpsertTopic(ctx, "cluster-a", "t1", 1, false, "")
	id2, _, _ := s.UpsertTopic(ctx, "cluster-a", "t2", 1, false, "")
	s.UpsertTopic(ctx, "cluster-b", "t3", 1, false, "")

	now := time.Now()
	s.IncrementTopicStats(ctx, id1, 10, 5, now, "json")
	s.IncrementTopicStats(ctx, id2, 20, 8, now, "json")

	stats, err := s.GetClusterStats(ctx)
	if err != nil {
		t.Fatalf("GetClusterStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(stats))
	}

	m := make(map[string]ClusterStats)
	for _, cs := range stats {
		m[cs.Name] = cs
	}

	if m["cluster-a"].TopicCount != 2 {
		t.Errorf("cluster-a: expected topic_count=2, got %d", m["cluster-a"].TopicCount)
	}
	if m["cluster-a"].MessageCount != 30 {
		t.Errorf("cluster-a: expected message_count=30, got %d", m["cluster-a"].MessageCount)
	}
	if m["cluster-a"].KeyCount != 13 {
		t.Errorf("cluster-a: expected key_count=13, got %d", m["cluster-a"].KeyCount)
	}
	if m["cluster-b"].TopicCount != 1 {
		t.Errorf("cluster-b: expected topic_count=1, got %d", m["cluster-b"].TopicCount)
	}
}

func TestDeleteClusterData(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()
	cleanDB(t, s)

	s.UpsertTopic(ctx, "cluster-a", "t1", 1, false, "")
	s.UpsertTopic(ctx, "cluster-a", "t2", 1, false, "")
	s.UpsertTopic(ctx, "cluster-b", "t3", 1, false, "")

	err := s.DeleteClusterData(ctx, "cluster-a")
	if err != nil {
		t.Fatalf("DeleteClusterData: %v", err)
	}

	topicsA, _ := s.ListTopicsByCluster(ctx, "cluster-a")
	if len(topicsA) != 0 {
		t.Errorf("expected 0 topics for cluster-a, got %d", len(topicsA))
	}

	topicsB, _ := s.ListTopicsByCluster(ctx, "cluster-b")
	if len(topicsB) != 1 {
		t.Errorf("expected 1 topic for cluster-b, got %d", len(topicsB))
	}
}

func TestIncrementTopicStats(t *testing.T) {
	s := setupTestDB(t)
	ctx := context.Background()

	t.Run("basic increment", func(t *testing.T) {
		cleanDB(t, s)
		id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 1, false, "")
		now := time.Now()

		err := s.IncrementTopicStats(ctx, id, 10, 5, now, "json")
		if err != nil {
			t.Fatalf("IncrementTopicStats: %v", err)
		}

		detail, _ := s.GetTopicDetail(ctx, "cluster-a", "orders")
		if detail.ConsumedMessages != 10 {
			t.Errorf("expected message_count=10, got %d", detail.ConsumedMessages)
		}
		if detail.UniqueKeys != 5 {
			t.Errorf("expected key_count=5, got %d", detail.UniqueKeys)
		}
		if detail.MessageFormat != "json" {
			t.Errorf("expected format=json, got %s", detail.MessageFormat)
		}
	})

	t.Run("accumulative increment", func(t *testing.T) {
		cleanDB(t, s)
		id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 1, false, "")
		now := time.Now()

		s.IncrementTopicStats(ctx, id, 10, 5, now, "json")
		s.IncrementTopicStats(ctx, id, 5, 3, now, "json")

		detail, _ := s.GetTopicDetail(ctx, "cluster-a", "orders")
		if detail.ConsumedMessages != 15 {
			t.Errorf("expected message_count=15, got %d", detail.ConsumedMessages)
		}
		if detail.UniqueKeys != 8 {
			t.Errorf("expected key_count=8, got %d", detail.UniqueKeys)
		}
	})

	t.Run("tombstone format ignored", func(t *testing.T) {
		cleanDB(t, s)
		id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 1, false, "")
		now := time.Now()

		s.IncrementTopicStats(ctx, id, 10, 5, now, "json")
		s.IncrementTopicStats(ctx, id, 1, 0, now, "tombstone")

		detail, _ := s.GetTopicDetail(ctx, "cluster-a", "orders")
		if detail.MessageFormat != "json" {
			t.Errorf("expected format=json (tombstone ignored), got %s", detail.MessageFormat)
		}
	})

	t.Run("mixed formats become other", func(t *testing.T) {
		cleanDB(t, s)
		id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 1, false, "")
		now := time.Now()

		s.IncrementTopicStats(ctx, id, 10, 5, now, "json")
		s.IncrementTopicStats(ctx, id, 5, 3, now, "avro")

		detail, _ := s.GetTopicDetail(ctx, "cluster-a", "orders")
		if detail.MessageFormat != "other" {
			t.Errorf("expected format=other for mixed formats, got %s", detail.MessageFormat)
		}
	})
}
