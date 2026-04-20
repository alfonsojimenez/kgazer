package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alfonsojimenez/kgazer/backend/internal/config"
	"github.com/alfonsojimenez/kgazer/backend/internal/progress"
	"github.com/alfonsojimenez/kgazer/backend/internal/status"
	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

func newTestRouter(s *store.Store) http.Handler {
	st := status.NewTracker()
	st.Set("cluster-a", status.Connected)
	st.Set("cluster-b", status.Connecting)
	pt := progress.NewTracker()
	return NewRouter(s, st, pt, nil, "test", time.Now(), &config.Config{})
}

func newTrackerOnlyRouter() (http.Handler, *status.Tracker, *progress.Tracker) {
	st := status.NewTracker()
	st.Set("cluster-a", status.Connected)
	st.Set("cluster-b", status.Connecting)
	pt := progress.NewTracker()
	return NewRouter(nil, st, pt, nil, "test", time.Now(), &config.Config{}), st, pt
}

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration tests")
	}

	ctx := context.Background()
	s, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	pool := s.WritePool()
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS topics (
			id SERIAL PRIMARY KEY, cluster TEXT NOT NULL, name TEXT NOT NULL,
			partitions INT NOT NULL DEFAULT 0, compacted BOOLEAN NOT NULL DEFAULT false,
			message_count INT NOT NULL DEFAULT 0, key_count INT NOT NULL DEFAULT 0,
			last_message_at TIMESTAMPTZ, message_format TEXT NOT NULL DEFAULT 'unknown',
			kafka_topic_id TEXT NOT NULL DEFAULT '', synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(cluster, name))`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGSERIAL PRIMARY KEY, topic_id INT NOT NULL REFERENCES topics(id),
			key TEXT NOT NULL, body JSONB NOT NULL, format TEXT NOT NULL DEFAULT 'json',
			partition INT NOT NULL, offset_id BIGINT NOT NULL, timestamp TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(), UNIQUE(topic_id, partition, offset_id))`,
		`CREATE TABLE IF NOT EXISTS keys (
			id SERIAL PRIMARY KEY, topic_id INT NOT NULL REFERENCES topics(id),
			key TEXT NOT NULL, message_count INT NOT NULL DEFAULT 1, partition INT NOT NULL DEFAULT 0,
			offset_id BIGINT NOT NULL DEFAULT 0, last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(topic_id, key))`,
	}
	for _, m := range migrations {
		if _, err := pool.Exec(ctx, m); err != nil {
			t.Fatalf("migration failed: %v", err)
		}
	}
	pool.Exec(ctx, "TRUNCATE keys, messages, topics CASCADE")

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

func TestHealthEndpoint(t *testing.T) {
	router, _, _ := newTrackerOnlyRouter()
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
}

func TestListClustersEndpoint(t *testing.T) {
	router, _, _ := newTrackerOnlyRouter()
	req := httptest.NewRequest("GET", "/api/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var clusters []status.ClusterStatus
	if err := json.NewDecoder(w.Body).Decode(&clusters); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	m := make(map[string]status.ClusterStatus)
	for _, c := range clusters {
		m[c.Name] = c
	}
	if m["cluster-a"].Status != status.Connected {
		t.Errorf("expected cluster-a status=connected, got %s", m["cluster-a"].Status)
	}
	if m["cluster-b"].Status != status.Connecting {
		t.Errorf("expected cluster-b status=connecting, got %s", m["cluster-b"].Status)
	}
}

func TestPauseClusterEndpoint(t *testing.T) {
	router, st, _ := newTrackerOnlyRouter()

	req := httptest.NewRequest("POST", "/api/clusters/cluster-a/pause", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["cluster"] != "cluster-a" {
		t.Errorf("expected cluster=cluster-a, got %v", body["cluster"])
	}
	if body["paused"] != true {
		t.Errorf("expected paused=true, got %v", body["paused"])
	}

	if !st.IsPaused("cluster-a") {
		t.Error("expected cluster-a to be paused in tracker")
	}
}

func TestResumeClusterEndpoint(t *testing.T) {
	router, st, _ := newTrackerOnlyRouter()
	st.SetPaused("cluster-a", true)

	req := httptest.NewRequest("POST", "/api/clusters/cluster-a/resume", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["paused"] != false {
		t.Errorf("expected paused=false, got %v", body["paused"])
	}

	if st.IsPaused("cluster-a") {
		t.Error("expected cluster-a to be resumed in tracker")
	}
}

func TestListTopicsEndpoint(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "")
	s.UpsertTopic(ctx, "cluster-a", "users", 1, false, "")

	router := newTestRouter(s)
	req := httptest.NewRequest("GET", "/api/topics?cluster=cluster-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var topics []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&topics); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(topics) < 2 {
		t.Fatalf("expected at least 2 topics, got %d", len(topics))
	}

	for _, topic := range topics {
		if _, ok := topic["progress"]; !ok {
			t.Error("expected progress field in topic response")
		}
	}
}

func TestGetTopicDetailEndpoint(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "tid-001")

	router := newTestRouter(s)

	t.Run("existing topic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/topics/orders?cluster=cluster-a", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if body["name"] != "orders" {
			t.Errorf("expected name=orders, got %v", body["name"])
		}
		if _, ok := body["progress"]; !ok {
			t.Error("expected progress field")
		}
	})

	t.Run("missing topic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/topics/nonexistent?cluster=cluster-a", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListKeysEndpoint(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "")
	now := time.Now()
	s.UpsertKey(ctx, id, "key-1", 0, 10, now)
	s.UpsertKey(ctx, id, "key-2", 0, 20, now)
	s.IncrementTopicStats(ctx, id, 2, 2, now, "json")

	router := newTestRouter(s)
	req := httptest.NewRequest("GET", "/api/topics/orders/keys?cluster=cluster-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 keys, got %d", len(data))
	}

	total, ok := body["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("expected total=2, got %v", body["total"])
	}
}

func TestListKeysEndpointTopicNotFound(t *testing.T) {
	s := setupTestStore(t)

	router := newTestRouter(s)
	req := httptest.NewRequest("GET", "/api/topics/nonexistent/keys?cluster=cluster-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestReconsumeEndpoint(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 3, true, "")
	now := time.Now()
	s.SaveMessageBatch(ctx, []store.PendingMessage{
		{TopicID: id, Key: "k1", Body: []byte(`{}`), Format: "json", Partition: 0, Offset: 0, Timestamp: now},
	})

	st := status.NewTracker()
	st.Set("cluster-a", status.Connected)
	pt := progress.NewTracker()
	router := NewRouter(s, st, pt, nil, "test", time.Now(), &config.Config{})

	req := httptest.NewRequest("POST", "/api/topics/orders/reconsume?cluster=cluster-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body["topic"] != "orders" {
		t.Errorf("expected topic=orders, got %v", body["topic"])
	}

	topics := pt.PopReconsumeRequests("cluster-a")
	if len(topics) != 1 || topics[0] != "orders" {
		t.Errorf("expected reconsume request for orders, got %v", topics)
	}
}
