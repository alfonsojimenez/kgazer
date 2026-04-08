package progress

import (
	"sync"
	"testing"
)

func TestNewTracker(t *testing.T) {
	tr := NewTracker()
	if tr == nil {
		t.Fatal("NewTracker returned nil")
	}
}

func TestSeedConsumed(t *testing.T) {
	tr := NewTracker()

	tr.SeedConsumed("c1", "topic-a", 0, 100)
	tr.SeedConsumed("c1", "topic-a", 1, 200)

	p := tr.Get("c1", "topic-a")
	if p.Total != 0 {
		t.Errorf("expected total=0 without watermarks, got %d", p.Total)
	}
}

func TestSeedConsumedHigherOffsetWins(t *testing.T) {
	tr := NewTracker()

	tr.SeedConsumed("c1", "topic-a", 0, 100)
	tr.SeedConsumed("c1", "topic-a", 0, 50)

	tr.mu.RLock()
	pk := partitionKey{"topic-a", 0}
	got := tr.consumed["c1"][pk]
	tr.mu.RUnlock()

	if got != 100 {
		t.Errorf("expected consumed=100 (higher wins), got %d", got)
	}
}

func TestSeedConsumedLowerIgnored(t *testing.T) {
	tr := NewTracker()

	tr.SeedConsumed("c1", "topic-a", 0, 200)
	tr.SeedConsumed("c1", "topic-a", 0, 100)

	tr.mu.RLock()
	pk := partitionKey{"topic-a", 0}
	got := tr.consumed["c1"][pk]
	tr.mu.RUnlock()

	if got != 200 {
		t.Errorf("expected consumed=200, got %d", got)
	}
}

func TestUpdateConsumed(t *testing.T) {
	tr := NewTracker()

	tr.UpdateConsumed("c1", "topic-a", 0, 50)

	tr.mu.RLock()
	pk := partitionKey{"topic-a", 0}
	got := tr.consumed["c1"][pk]
	tr.mu.RUnlock()

	if got != 50 {
		t.Errorf("expected consumed=50, got %d", got)
	}
}

func TestUpdateConsumedHigherOffsetWins(t *testing.T) {
	tr := NewTracker()

	tr.UpdateConsumed("c1", "topic-a", 0, 100)
	tr.UpdateConsumed("c1", "topic-a", 0, 50)

	tr.mu.RLock()
	pk := partitionKey{"topic-a", 0}
	got := tr.consumed["c1"][pk]
	tr.mu.RUnlock()

	if got != 100 {
		t.Errorf("expected consumed=100 (higher wins), got %d", got)
	}
}

func TestUpdateConsumedSetsLastSeen(t *testing.T) {
	tr := NewTracker()

	tr.UpdateConsumed("c1", "topic-a", 0, 100)

	tr.mu.RLock()
	ls, ok := tr.lastSeen["c1"]["topic-a"]
	tr.mu.RUnlock()

	if !ok {
		t.Fatal("expected lastSeen to be set")
	}
	if ls.IsZero() {
		t.Error("expected lastSeen to be non-zero")
	}
}

func TestGetWithoutWatermarksReturnsZero(t *testing.T) {
	tr := NewTracker()

	tr.SeedConsumed("c1", "topic-a", 0, 100)

	p := tr.Get("c1", "topic-a")
	if p.Total != 0 || p.Consumed != 0 || p.Percent != 0 || p.Done {
		t.Errorf("expected zero progress without watermarks, got %+v", p)
	}
}

func TestGetUnknownCluster(t *testing.T) {
	tr := NewTracker()
	p := tr.Get("nonexistent", "topic-a")
	if p.Total != 0 || p.Consumed != 0 || p.Percent != 0 || p.Done {
		t.Errorf("expected zero progress for unknown cluster, got %+v", p)
	}
}

func TestGetWithWatermarks(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 100},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 99)

	p := tr.Get("c1", "topic-a")
	if p.Total != 100 {
		t.Errorf("expected total=100, got %d", p.Total)
	}
	if p.Consumed != 100 {
		t.Errorf("expected consumed=100, got %d", p.Consumed)
	}
	if p.Percent != 100 {
		t.Errorf("expected percent=100, got %f", p.Percent)
	}
	if !p.Done {
		t.Error("expected done=true")
	}
}

func TestGetPartialProgress(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 200},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 99)

	p := tr.Get("c1", "topic-a")
	if p.Total != 200 {
		t.Errorf("expected total=200, got %d", p.Total)
	}
	if p.Consumed != 100 {
		t.Errorf("expected consumed=100, got %d", p.Consumed)
	}
	if p.Percent != 50 {
		t.Errorf("expected percent=50, got %f", p.Percent)
	}
	if p.Done {
		t.Error("expected done=false for 50%% progress")
	}
}

func TestGetMultiplePartitions(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 100},
		{topic: "topic-a", partition: 1}: {low: 0, high: 100},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 99)
	tr.UpdateConsumed("c1", "topic-a", 1, 99)

	p := tr.Get("c1", "topic-a")
	if p.Total != 200 {
		t.Errorf("expected total=200, got %d", p.Total)
	}
	if p.Consumed != 200 {
		t.Errorf("expected consumed=200, got %d", p.Consumed)
	}
	if !p.Done {
		t.Error("expected done=true")
	}
}

func TestGetFiltersOtherTopics(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 100},
		{topic: "topic-b", partition: 0}: {low: 0, high: 500},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 99)

	p := tr.Get("c1", "topic-a")
	if p.Total != 100 {
		t.Errorf("expected total=100 (only topic-a), got %d", p.Total)
	}
}

func TestGetConsumedCappedAtSpan(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 50},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 999)

	p := tr.Get("c1", "topic-a")
	if p.Consumed != 50 {
		t.Errorf("expected consumed capped at span=50, got %d", p.Consumed)
	}
}

func TestRemoveTopic(t *testing.T) {
	tr := NewTracker()

	tr.mu.Lock()
	tr.marks["c1"] = map[partitionKey]watermarks{
		{topic: "topic-a", partition: 0}: {low: 0, high: 100},
	}
	tr.mu.Unlock()

	tr.UpdateConsumed("c1", "topic-a", 0, 50)

	tr.RemoveTopic("c1", "topic-a")

	p := tr.Get("c1", "topic-a")
	if p.Total != 0 {
		t.Errorf("expected total=0 after remove, got %d", p.Total)
	}
	if p.Consumed != 0 {
		t.Errorf("expected consumed=0 after remove, got %d", p.Consumed)
	}

	tr.mu.RLock()
	_, hasLastSeen := tr.lastSeen["c1"]["topic-a"]
	tr.mu.RUnlock()
	if hasLastSeen {
		t.Error("expected lastSeen cleared after RemoveTopic")
	}
}

func TestRemoveTopicNonexistent(t *testing.T) {
	tr := NewTracker()
	tr.RemoveTopic("c1", "nonexistent")
}

func TestRequestReconsumeAndPop(t *testing.T) {
	tr := NewTracker()

	tr.RequestReconsume("c1", "topic-a")
	tr.RequestReconsume("c1", "topic-b")

	topics := tr.PopReconsumeRequests("c1")
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}

	got := make(map[string]bool)
	for _, topic := range topics {
		got[topic] = true
	}
	if !got["topic-a"] || !got["topic-b"] {
		t.Errorf("expected topic-a and topic-b, got %v", topics)
	}

	second := tr.PopReconsumeRequests("c1")
	if second != nil {
		t.Errorf("expected nil on second pop, got %v", second)
	}
}

func TestRequestReconsumeDeduplicate(t *testing.T) {
	tr := NewTracker()

	tr.RequestReconsume("c1", "topic-a")
	tr.RequestReconsume("c1", "topic-a")

	topics := tr.PopReconsumeRequests("c1")
	if len(topics) != 1 {
		t.Fatalf("expected 1 topic (deduplicated), got %d", len(topics))
	}
}

func TestPopReconsumeNonexistentCluster(t *testing.T) {
	tr := NewTracker()
	topics := tr.PopReconsumeRequests("nonexistent")
	if topics != nil {
		t.Errorf("expected nil for nonexistent cluster, got %v", topics)
	}
}

func TestConcurrentSeedAndGet(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(offset int64) {
			defer wg.Done()
			tr.SeedConsumed("c1", "topic-a", 0, offset)
		}(int64(i))
		go func() {
			defer wg.Done()
			_ = tr.Get("c1", "topic-a")
		}()
	}

	wg.Wait()
}
