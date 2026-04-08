package syncer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/alfonsojimenez/kgazer/backend/internal/config"
	"github.com/alfonsojimenez/kgazer/backend/internal/status"
	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

func Start(ctx context.Context, cluster config.Cluster, s *store.Store, t *status.Tracker, compactedOnly bool, interval time.Duration) {
	t.Set(cluster.Name, status.Connecting)

	go func() {
		if err := sync(ctx, cluster, s, compactedOnly); err != nil {
			slog.Error("initial topic sync failed", "cluster", cluster.Name, "error", err)
			t.Set(cluster.Name, status.Disconnected)
		} else {
			t.Set(cluster.Name, status.Connected)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sync(ctx, cluster, s, compactedOnly); err != nil {
					slog.Error("topic sync failed", "cluster", cluster.Name, "error", err)
					t.Set(cluster.Name, status.Disconnected)
				} else {
					t.Set(cluster.Name, status.Connected)
				}
			}
		}
	}()
}

func sync(ctx context.Context, cluster config.Cluster, s *store.Store, compactedOnly bool) error {
	cfg := &kafka.ConfigMap{
		"bootstrap.servers": cluster.BootstrapServers,
	}

	for k, v := range cluster.Properties {
		if err := cfg.SetKey(k, v); err != nil {
			return fmt.Errorf("setting kafka property %s: %w", k, err)
		}
	}

	admin, err := kafka.NewAdminClient(cfg)
	if err != nil {
		return fmt.Errorf("creating admin client: %w", err)
	}
	defer admin.Close()

	metadata, err := admin.GetMetadata(nil, true, 10000)
	if err != nil {
		return fmt.Errorf("fetching metadata: %w", err)
	}

	topicUUIDs := describeTopicUUIDs(ctx, admin, metadata)

	var activeNames []string
	var count int
	for topicName, topicMeta := range metadata.Topics {
		if strings.HasPrefix(topicName, "_") {
			continue
		}

		compacted := false
		results, err := admin.DescribeConfigs(ctx, []kafka.ConfigResource{{
			Type: kafka.ResourceTopic,
			Name: topicName,
		}})
		if err != nil {
			slog.Warn("failed to describe topic config, assuming compacted",
				"topic", topicName, "error", err)
			compacted = true
		} else if len(results) > 0 {
			if results[0].Error.Code() != kafka.ErrNoError {
				slog.Warn("topic config error, assuming compacted",
					"topic", topicName, "error", results[0].Error)
				compacted = true
			} else {
				for _, entry := range results[0].Config {
					if entry.Name == "cleanup.policy" && strings.Contains(entry.Value, "compact") {
						compacted = true
						break
					}
				}
			}
		}

		if compactedOnly && !compacted {
			slog.Debug("skipping non-compacted topic", "topic", topicName)
			continue
		}

		kafkaTopicID := topicUUIDs[topicName]

		id, recreated, err := s.UpsertTopic(ctx, cluster.Name, topicName, len(topicMeta.Partitions), compacted, kafkaTopicID)
		if err != nil {
			slog.Error("failed to upsert topic", "topic", topicName, "error", err)
			continue
		}

		if recreated {
			slog.Warn("topic recreated, purging old data",
				"cluster", cluster.Name, "topic", topicName, "topic_id", id)
			if err := s.PurgeTopicData(ctx, id); err != nil {
				slog.Error("failed to purge recreated topic", "topic", topicName, "error", err)
			}
		}

		activeNames = append(activeNames, topicName)
		count++
	}

	removed, err := s.RemoveStaleTopics(ctx, cluster.Name, activeNames)
	if err != nil {
		slog.Error("failed to remove stale topics", "cluster", cluster.Name, "error", err)
	}
	if len(removed) > 0 {
		slog.Info("removed stale topics", "cluster", cluster.Name, "topics", removed)
	}

	slog.Info("topic sync complete", "cluster", cluster.Name, "topics", count)
	return nil
}

func describeTopicUUIDs(ctx context.Context, admin *kafka.AdminClient, metadata *kafka.Metadata) map[string]string {
	var names []string
	for topicName := range metadata.Topics {
		if !strings.HasPrefix(topicName, "_") {
			names = append(names, topicName)
		}
	}
	if len(names) == 0 {
		return nil
	}

	uuids := make(map[string]string, len(names))

	descCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := admin.DescribeTopics(descCtx, kafka.NewTopicCollectionOfTopicNames(names))
	if err != nil {
		slog.Warn("DescribeTopics unavailable, skipping recreation detection", "error", err)
		return uuids
	}

	for _, desc := range result.TopicDescriptions {
		if desc.Error.Code() == kafka.ErrNoError {
			uuidStr := desc.TopicID.String()
			if uuidStr != "" {
				uuids[desc.Name] = uuidStr
			}
		}
	}
	return uuids
}

func TopicNames(ctx context.Context, s *store.Store, cluster string) ([]string, error) {
	topics, err := s.ListTopicsByCluster(ctx, cluster)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.Name
	}
	return names, nil
}
