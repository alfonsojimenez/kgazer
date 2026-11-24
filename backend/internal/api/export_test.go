package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

func seedExportTopic(t *testing.T) http.Handler {
	t.Helper()
	s := setupTestStore(t)
	ctx := context.Background()
	id, _, _ := s.UpsertTopic(ctx, "cluster-a", "orders", 1, true, "")
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	msgs := []store.PendingMessage{
		{TopicID: id, Key: "a", Body: []byte(`{"n":1}`), Format: "json", Offset: 0, Timestamp: t0},
		{TopicID: id, Key: "a", Body: []byte(`{"n":2,"s":"x,\"y\""}`), Headers: []byte(`[{"key":"h","value":"v"}]`), Format: "json", Offset: 1, Timestamp: t0.Add(time.Minute)},
		{TopicID: id, Key: "b", Body: []byte(`null`), Format: "tombstone", Offset: 2, Timestamp: t0.Add(2 * time.Minute)},
	}
	if _, err := s.SaveMessageBatch(ctx, msgs); err != nil {
		t.Fatalf("SaveMessageBatch: %v", err)
	}
	_, err := s.UpsertKeyBatch(ctx, []store.KeyEntry{
		{TopicID: id, Key: "a", Count: 2, Offset: 1, Timestamp: t0.Add(time.Minute), Body: msgs[1].Body},
		{TopicID: id, Key: "b", Count: 1, Offset: 2, Timestamp: t0.Add(2 * time.Minute), Body: msgs[2].Body},
	})
	if err != nil {
		t.Fatalf("UpsertKeyBatch: %v", err)
	}
	return newTestRouter(s)
}

func get(router http.Handler, url string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	return w
}

func TestExportKeysEndpoint(t *testing.T) {
	router := seedExportTopic(t)

	t.Run("json honours filters and includes bodies", func(t *testing.T) {
		w := get(router, "/api/topics/orders/keys/export?cluster=cluster-a&status=live")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body)
		}
		if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="orders-keys.json"` {
			t.Errorf("unexpected Content-Disposition %q", cd)
		}
		var rows []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, w.Body)
		}
		if len(rows) != 1 || rows[0]["key"] != "a" || rows[0]["body"].(map[string]any)["n"] != float64(2) {
			t.Errorf("unexpected rows: %v", rows)
		}
	})

	t.Run("csv escapes JSON bodies", func(t *testing.T) {
		w := get(router, "/api/topics/orders/keys/export?cluster=cluster-a&format=csv&sort_by=offset&sort=asc")
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Errorf("unexpected Content-Type %q", ct)
		}
		records, err := csv.NewReader(w.Body).ReadAll()
		if err != nil {
			t.Fatalf("invalid CSV: %v", err)
		}
		if len(records) != 3 || records[0][0] != "key" || records[0][6] != "body" {
			t.Fatalf("unexpected records: %v", records)
		}
		if records[1][0] != "a" || records[1][6] != `{"n": 2, "s": "x,\"y\""}` {
			t.Errorf("unexpected row a: %v", records[1])
		}
		if records[2][0] != "b" || records[2][5] != "2026-05-01T10:02:00Z" || records[2][6] != "null" {
			t.Errorf("unexpected row b: %v", records[2])
		}
	})

	t.Run("empty result is an empty JSON array", func(t *testing.T) {
		w := get(router, "/api/topics/orders/keys/export?cluster=cluster-a&search=nope")
		if strings.TrimSpace(w.Body.String()) != "[]" {
			t.Errorf("expected [], got %q", w.Body)
		}
	})

	t.Run("errors", func(t *testing.T) {
		if w := get(router, "/api/topics/orders/keys/export?cluster=cluster-a&format=xml"); w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for bad format, got %d", w.Code)
		}
		if w := get(router, "/api/topics/missing/keys/export?cluster=cluster-a"); w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for unknown topic, got %d", w.Code)
		}
	})
}

func TestExportKeyHistoryEndpoint(t *testing.T) {
	router := seedExportTopic(t)

	w := get(router, "/api/topics/orders/history/export?cluster=cluster-a&key=a&format=csv")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="orders-a-history.csv"` {
		t.Errorf("unexpected Content-Disposition %q", cd)
	}
	records, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(records) != 3 || records[1][2] != "1" || records[1][4] != `[{"key": "h", "value": "v"}]` || records[2][4] != "[]" {
		t.Errorf("unexpected records: %v", records)
	}

	if w := get(router, "/api/topics/orders/history/export?cluster=cluster-a"); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without key, got %d", w.Code)
	}
}
