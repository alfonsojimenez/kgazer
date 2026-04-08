package progress

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/alfonsojimenez/kgazer/backend/internal/config"
)

type TopicProgress struct {
	Total    int64   `json:"total"`
	Consumed int64   `json:"consumed"`
	Percent  float64 `json:"percent"`
	Done     bool    `json:"done"`
}

type partitionKey struct {
	topic     string
	partition int32
}

type watermarks struct {
	low  int64
	high int64
}

type Tracker struct {
	mu                sync.RWMutex
	marks             map[string]map[partitionKey]watermarks
	consumed          map[string]map[partitionKey]int64
	lastSeen          map[string]map[string]time.Time
	reconsumeRequests map[string]map[string]bool
}

func NewTracker() *Tracker {
	return &Tracker{
		marks:             make(map[string]map[partitionKey]watermarks),
		consumed:          make(map[string]map[partitionKey]int64),
		lastSeen:          make(map[string]map[string]time.Time),
		reconsumeRequests: make(map[string]map[string]bool),
	}
}

func (t *Tracker) RequestReconsume(cluster, topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.reconsumeRequests[cluster] == nil {
		t.reconsumeRequests[cluster] = make(map[string]bool)
	}
	t.reconsumeRequests[cluster][topic] = true
}

func (t *Tracker) PopReconsumeRequests(cluster string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	topics := t.reconsumeRequests[cluster]
	if len(topics) == 0 {
		return nil
	}
	result := make([]string, 0, len(topics))
	for topic := range topics {
		result = append(result, topic)
	}
	delete(t.reconsumeRequests, cluster)
	return result
}

func (t *Tracker) SeedConsumed(cluster, topic string, partition int32, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.consumed[cluster] == nil {
		t.consumed[cluster] = make(map[partitionKey]int64)
	}
	pk := partitionKey{topic, partition}
	if offset > t.consumed[cluster][pk] {
		t.consumed[cluster][pk] = offset
	}
}

func (t *Tracker) UpdateConsumed(cluster, topic string, partition int32, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.consumed[cluster] == nil {
		t.consumed[cluster] = make(map[partitionKey]int64)
	}
	pk := partitionKey{topic, partition}
	if offset > t.consumed[cluster][pk] {
		t.consumed[cluster][pk] = offset
	}

	if t.lastSeen[cluster] == nil {
		t.lastSeen[cluster] = make(map[string]time.Time)
	}
	t.lastSeen[cluster][topic] = time.Now()
}

func (t *Tracker) Get(cluster, topic string) TopicProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	clusterMarks := t.marks[cluster]
	clusterConsumed := t.consumed[cluster]
	if clusterMarks == nil {
		return TopicProgress{}
	}

	var total, consumed int64
	for pk, wm := range clusterMarks {
		if pk.topic != topic {
			continue
		}
		span := wm.high - wm.low
		total += span

		if clusterConsumed != nil {
			off := clusterConsumed[pk]
			progress := off + 1 - wm.low
			if progress < 0 {
				progress = 0
			}
			if progress > span {
				progress = span
			}
			consumed += progress
		}
	}

	var pct float64
	done := false
	if total > 0 {
		pct = float64(consumed) / float64(total) * 100
		done = pct >= 99.5
	}

	if !done && consumed > 0 {
		ls := t.lastSeen[cluster]
		lastTime, hasLastSeen := ls[topic]
		if !hasLastSeen || time.Since(lastTime) > 30*time.Second {
			done = true
			pct = 100
		}
	}

	if done {
		pct = 100
	}

	return TopicProgress{Total: total, Consumed: consumed, Percent: pct, Done: done}
}

func (t *Tracker) RemoveTopic(cluster, topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if clusterConsumed, ok := t.consumed[cluster]; ok {
		for pk := range clusterConsumed {
			if pk.topic == topic {
				delete(clusterConsumed, pk)
			}
		}
	}

	if clusterMarks, ok := t.marks[cluster]; ok {
		for pk := range clusterMarks {
			if pk.topic == topic {
				delete(clusterMarks, pk)
			}
		}
	}

	if ls, ok := t.lastSeen[cluster]; ok {
		delete(ls, topic)
	}
}

func (t *Tracker) StartWatermarkSync(ctx context.Context, cluster config.Cluster, interval time.Duration) {
	go func() {
		t.syncWatermarks(cluster)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.syncWatermarks(cluster)
			}
		}
	}()
}

func (t *Tracker) syncWatermarks(cluster config.Cluster) {
	cfg := &kafka.ConfigMap{
		"bootstrap.servers":  cluster.BootstrapServers,
		"group.id":           fmt.Sprintf("kgazer-watermark-%s", cluster.Name),
		"enable.auto.commit": false,
	}
	for k, v := range cluster.Properties {
		cfg.SetKey(k, v)
	}

	c, err := kafka.NewConsumer(cfg)
	if err != nil {
		slog.Error("watermark sync: failed to create consumer", "cluster", cluster.Name, "error", err)
		return
	}
	defer c.Close()

	meta, err := c.GetMetadata(nil, true, 10000)
	if err != nil {
		slog.Error("watermark sync: failed to get metadata", "cluster", cluster.Name, "error", err)
		return
	}

	local := make(map[partitionKey]watermarks)
	for topicName, topicMeta := range meta.Topics {
		for _, p := range topicMeta.Partitions {
			low, high, err := c.QueryWatermarkOffsets(topicName, p.ID, 5000)
			if err != nil {
				continue
			}
			local[partitionKey{topicName, p.ID}] = watermarks{low, high}
		}
	}

	t.mu.Lock()
	t.marks[cluster.Name] = local
	t.mu.Unlock()

	counted := map[string]bool{}
	for pk := range local {
		if !counted[pk.topic] {
			counted[pk.topic] = true
		}
	}
	slog.Info("watermark sync complete", "cluster", cluster.Name, "topics", len(counted), "partitions", len(local))
}
