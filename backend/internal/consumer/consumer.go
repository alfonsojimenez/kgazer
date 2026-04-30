package consumer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/alfonsojimenez/kgazer/backend/internal/config"
	"github.com/alfonsojimenez/kgazer/backend/internal/decoder"
	"github.com/alfonsojimenez/kgazer/backend/internal/progress"
	"github.com/alfonsojimenez/kgazer/backend/internal/status"
	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

const (
	batchSize     = 500
	flushInterval = 500 * time.Millisecond
)

func Start(ctx context.Context, cluster config.Cluster, s *store.Store, tracker *status.Tracker, pt *progress.Tracker, syncInterval time.Duration) error {
	var dec *decoder.Decoder
	if cluster.SchemaRegistry != "" {
		username, password := "", ""
		if cluster.SchemaRegistryAuth != nil {
			username = cluster.SchemaRegistryAuth.Username
			password = cluster.SchemaRegistryAuth.Password
		}
		dec = decoder.New(cluster.SchemaRegistry, username, password)
		slog.Info("schema registry configured", "cluster", cluster.Name, "url", cluster.SchemaRegistry)
	}
	cfg := &kafka.ConfigMap{
		"bootstrap.servers":  cluster.BootstrapServers,
		"group.id":           fmt.Sprintf("kgazer-%s", cluster.Name),
		"enable.auto.commit": false,
	}

	for k, v := range cluster.Properties {
		if err := cfg.SetKey(k, v); err != nil {
			return fmt.Errorf("setting kafka property %s: %w", k, err)
		}
	}

	slog.Info("consumer waiting for topic sync", "cluster", cluster.Name)
	time.Sleep(5 * time.Second)

	c, err := kafka.NewConsumer(cfg)
	if err != nil {
		return fmt.Errorf("creating consumer: %w", err)
	}

	topicIDs := make(map[string]int)
	kafkaTopicIDs := make(map[string]string)
	storedOffsets := make(map[string]map[int32]int64)

	dbTopics, err := s.ListTopicsByCluster(ctx, cluster.Name)
	if err != nil {
		c.Close()
		return fmt.Errorf("fetching topics from DB: %w", err)
	}

	for _, t := range dbTopics {
		topicIDs[t.Name] = t.ID
		kafkaTopicIDs[t.Name] = t.KafkaTopicID

		offsets, err := s.GetMaxOffsets(ctx, t.ID)
		if err == nil && len(offsets) > 0 {
			m := make(map[int32]int64)
			for _, po := range offsets {
				m[po.Partition] = po.Offset
				pt.SeedConsumed(cluster.Name, t.Name, po.Partition, po.Offset)
			}
			storedOffsets[t.Name] = m
		}
	}

	if len(topicIDs) > 0 {
		topicNames := make([]string, 0, len(topicIDs))
		for name := range topicIDs {
			topicNames = append(topicNames, name)
		}

		partitions, err := resolvePartitions(c, topicNames, storedOffsets)
		if err != nil {
			c.Close()
			return fmt.Errorf("resolving partitions: %w", err)
		}

		if err := c.Assign(partitions); err != nil {
			c.Close()
			return fmt.Errorf("assigning partitions: %w", err)
		}
	}

	slog.Info("consumer started", "cluster", cluster.Name, "topics", len(topicIDs))

	go func() {
		defer c.Close()

		type keyAgg struct {
			count     int
			partition int32
			offset    int64
			timestamp time.Time
			body      []byte
		}
		type topicKeyID struct {
			topicID int
			key     string
		}
		type topicBatchStats struct {
			messages int
			keys     map[string]bool
			formats  map[string]bool
			latestTS time.Time
		}

		batch := make([]store.PendingMessage, 0, batchSize)
		batchStats := make(map[int]*topicBatchStats)
		keyAggs := make(map[topicKeyID]*keyAgg)
		lastFlush := time.Now()
		var lastAuthWarn time.Time
		var lastErrorLog time.Time
		var totalCount int64

		flush := func() {
			if len(batch) == 0 {
				return
			}
			inserted, err := s.SaveMessageBatch(ctx, batch)
			if err != nil {
				slog.Error("batch save failed", "cluster", cluster.Name, "error", err, "batch_size", len(batch))
			}
			if inserted > 0 {
				for tid, stats := range batchStats {
					batchFormat := "other"
					if len(stats.formats) == 1 {
						for f := range stats.formats {
							batchFormat = f
						}
					}
					if err := s.IncrementTopicStats(ctx, tid, stats.messages, len(stats.keys), stats.latestTS, batchFormat); err != nil {
						slog.Error("stats increment failed", "topic_id", tid, "error", err)
					}
				}
				entries := make([]store.KeyEntry, 0, len(keyAggs))
				for tk, agg := range keyAggs {
					entries = append(entries, store.KeyEntry{
						TopicID:   tk.topicID,
						Key:       tk.key,
						Count:     agg.count,
						Partition: agg.partition,
						Offset:    agg.offset,
						Timestamp: agg.timestamp,
						Body:      agg.body,
					})
				}
				if err := s.UpsertKeyBatch(ctx, entries); err != nil {
					slog.Error("key upsert failed", "cluster", cluster.Name, "error", err)
				}
			}
			batch = batch[:0]
			for k := range batchStats {
				delete(batchStats, k)
			}
			for k := range keyAggs {
				delete(keyAggs, k)
			}
			lastFlush = time.Now()
		}

		resyncTicker := time.NewTicker(syncInterval)
		defer resyncTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				flush()
				slog.Info("consumer shutting down", "cluster", cluster.Name)
				return
			case <-resyncTicker.C:
				flush()
				refreshTopics(ctx, c, s, cluster.Name, topicIDs, kafkaTopicIDs, pt)
				continue
			default:
			}

			if reconsumeTopics := pt.PopReconsumeRequests(cluster.Name); len(reconsumeTopics) > 0 {
				flush()
				handleReconsumeRequests(c, cluster.Name, reconsumeTopics, pt)
			}

			if tracker.IsPaused(cluster.Name) {
				flush()
				time.Sleep(500 * time.Millisecond)
				continue
			}

			ev := c.Poll(100)
			if ev == nil {
				if time.Since(lastFlush) >= flushInterval {
					flush()
				}
				continue
			}

			switch msg := ev.(type) {
			case *kafka.Message:
				topicName := *msg.TopicPartition.Topic
				topicID, ok := topicIDs[topicName]
				if !ok {
					continue
				}

				key := ""
				if msg.Key != nil {
					key = strings.ToValidUTF8(strings.ReplaceAll(string(msg.Key), "\x00", ""), "\uFFFD")
				}

				body, format := sanitizeBody(msg.Value, dec)
				body = bytes.ToValidUTF8(body, []byte("\uFFFD"))

				ts := msg.Timestamp
				if ts.IsZero() {
					ts = time.Now()
				}

				batch = append(batch, store.PendingMessage{
					TopicID:   topicID,
					Key:       key,
					Body:      body,
					Format:    format,
					Partition: msg.TopicPartition.Partition,
					Offset:    int64(msg.TopicPartition.Offset),
					Timestamp: ts,
				})

				if batchStats[topicID] == nil {
					batchStats[topicID] = &topicBatchStats{keys: make(map[string]bool), formats: make(map[string]bool)}
				}
				bs := batchStats[topicID]
				bs.messages++
				bs.keys[key] = true
				bs.formats[format] = true
				if ts.After(bs.latestTS) {
					bs.latestTS = ts
				}

				tk := topicKeyID{topicID, key}
				if keyAggs[tk] == nil {
					keyAggs[tk] = &keyAgg{}
				}
				ka := keyAggs[tk]
				ka.count++
				ka.partition = msg.TopicPartition.Partition
				ka.offset = int64(msg.TopicPartition.Offset)
				ka.body = body
				if ts.After(ka.timestamp) {
					ka.timestamp = ts
				}

				pt.UpdateConsumed(cluster.Name, topicName, msg.TopicPartition.Partition, int64(msg.TopicPartition.Offset))

				totalCount++
				if len(batch) >= batchSize {
					flush()
				}
				if totalCount%5000 == 0 {
					slog.Info("consumption progress", "cluster", cluster.Name, "messages", totalCount)
				}

			case kafka.Error:
				if msg.Code() == kafka.ErrTopicAuthorizationFailed {
					if time.Since(lastAuthWarn) > time.Minute {
						slog.Warn("kafka authorization error, some topics may lack READ permission",
							"cluster", cluster.Name, "error", msg)
						lastAuthWarn = time.Now()
					}
				} else {
					if time.Since(lastErrorLog) > time.Minute {
						slog.Error("kafka error", "cluster", cluster.Name, "error", msg)
						lastErrorLog = time.Now()
					}
				}
			}
		}
	}()

	return nil
}

func handleReconsumeRequests(c *kafka.Consumer, clusterName string, topics []string, pt *progress.Tracker) {
	assignment, _ := c.Assignment()
	positions, _ := c.Position(assignment)

	reconsumeSet := make(map[string]bool, len(topics))
	for _, name := range topics {
		reconsumeSet[name] = true
	}

	var allPartitions []kafka.TopicPartition
	for _, tp := range positions {
		if tp.Topic == nil {
			continue
		}
		if reconsumeSet[*tp.Topic] {
			allPartitions = append(allPartitions, kafka.TopicPartition{
				Topic:     tp.Topic,
				Partition: tp.Partition,
				Offset:    kafka.OffsetBeginning,
			})
		} else {
			allPartitions = append(allPartitions, tp)
		}
	}

	for _, name := range topics {
		pt.RemoveTopic(clusterName, name)
		slog.Info("reconsume: resetting topic to beginning", "cluster", clusterName, "topic", name)
	}

	if err := c.Assign(allPartitions); err != nil {
		slog.Error("reconsume: assign failed", "cluster", clusterName, "error", err)
	}
}

func refreshTopics(ctx context.Context, c *kafka.Consumer, s *store.Store, clusterName string, topicIDs map[string]int, kafkaTopicIDs map[string]string, pt *progress.Tracker) {
	dbTopics, err := s.ListTopicsByCluster(ctx, clusterName)
	if err != nil {
		slog.Error("refresh: failed to list topics", "cluster", clusterName, "error", err)
		return
	}

	dbSet := make(map[string]store.Topic, len(dbTopics))
	for _, t := range dbTopics {
		dbSet[t.Name] = t
	}

	var newNames []string
	var recreatedNames []string
	var removedNames []string

	for _, t := range dbTopics {
		if _, known := topicIDs[t.Name]; !known {
			newNames = append(newNames, t.Name)
		} else if oldUUID := kafkaTopicIDs[t.Name]; oldUUID != "" && t.KafkaTopicID != "" && oldUUID != t.KafkaTopicID {
			recreatedNames = append(recreatedNames, t.Name)
		}
	}
	for name := range topicIDs {
		if _, exists := dbSet[name]; !exists {
			removedNames = append(removedNames, name)
		}
	}

	if len(newNames) == 0 && len(recreatedNames) == 0 && len(removedNames) == 0 {
		return
	}

	slog.Info("topic changes detected", "cluster", clusterName,
		"new", len(newNames), "recreated", len(recreatedNames), "removed", len(removedNames))

	assignment, _ := c.Assignment()
	positions, _ := c.Position(assignment)

	removedSet := make(map[string]bool, len(removedNames))
	for _, name := range removedNames {
		removedSet[name] = true
	}
	recreatedSet := make(map[string]bool, len(recreatedNames))
	for _, name := range recreatedNames {
		recreatedSet[name] = true
	}

	var allPartitions []kafka.TopicPartition
	for _, tp := range positions {
		if tp.Topic == nil {
			continue
		}
		if removedSet[*tp.Topic] || recreatedSet[*tp.Topic] {
			continue
		}
		allPartitions = append(allPartitions, tp)
	}

	needMetadata := append(newNames, recreatedNames...)
	for _, name := range needMetadata {
		n := name
		meta, err := c.GetMetadata(&n, false, 10000)
		if err != nil {
			slog.Error("refresh: metadata failed", "topic", n, "error", err)
			continue
		}
		topicMeta, ok := meta.Topics[n]
		if !ok {
			continue
		}
		for _, p := range topicMeta.Partitions {
			allPartitions = append(allPartitions, kafka.TopicPartition{
				Topic:     &n,
				Partition: p.ID,
				Offset:    kafka.OffsetBeginning,
			})
		}
	}

	for _, name := range removedNames {
		delete(topicIDs, name)
		delete(kafkaTopicIDs, name)
		pt.RemoveTopic(clusterName, name)
		slog.Info("consumer: removed topic", "cluster", clusterName, "topic", name)
	}
	for _, name := range recreatedNames {
		t := dbSet[name]
		topicIDs[name] = t.ID
		kafkaTopicIDs[name] = t.KafkaTopicID
		pt.RemoveTopic(clusterName, name)
		slog.Info("consumer: reset recreated topic", "cluster", clusterName, "topic", name)
	}
	for _, name := range newNames {
		t := dbSet[name]
		topicIDs[name] = t.ID
		kafkaTopicIDs[name] = t.KafkaTopicID
		slog.Info("consumer: added new topic", "cluster", clusterName, "topic", name)
	}

	if err := c.Assign(allPartitions); err != nil {
		slog.Error("refresh: assign failed", "cluster", clusterName, "error", err)
	}
}

func sanitizeBody(value []byte, dec *decoder.Decoder) ([]byte, string) {
	if value == nil {
		return []byte("null"), "tombstone"
	}

	if dec != nil && len(value) > 5 && value[0] == 0x00 {
		if decoded, err := dec.Decode(value); err == nil {
			return stripNullBytes(decoded), "avro"
		}
	}

	if json.Valid(value) {
		buf := &bytes.Buffer{}
		if err := json.Compact(buf, value); err == nil {
			return stripNullBytes(buf.Bytes()), "json"
		}
	}

	encoded := base64.StdEncoding.EncodeToString(value)
	result, _ := json.Marshal(map[string]string{"_binary": encoded})
	return result, "other"
}

func stripNullBytes(b []byte) []byte {
	if !bytes.ContainsRune(b, 0) {
		return b
	}
	return bytes.ReplaceAll(b, []byte{0}, nil)
}

func resolvePartitions(c *kafka.Consumer, topicNames []string, storedOffsets map[string]map[int32]int64) ([]kafka.TopicPartition, error) {
	var partitions []kafka.TopicPartition

	for _, topic := range topicNames {
		meta, err := c.GetMetadata(&topic, false, 10000)
		if err != nil {
			return nil, fmt.Errorf("getting metadata for %s: %w", topic, err)
		}

		topicMeta, ok := meta.Topics[topic]
		if !ok {
			continue
		}

		for _, p := range topicMeta.Partitions {
			offset := kafka.OffsetBeginning
			if stored, ok := storedOffsets[topic]; ok {
				if off, ok := stored[p.ID]; ok {
					offset = kafka.Offset(off + 1)
				}
			}
			partitions = append(partitions, kafka.TopicPartition{
				Topic:     &topic,
				Partition: p.ID,
				Offset:    offset,
			})
		}
	}

	return partitions, nil
}
