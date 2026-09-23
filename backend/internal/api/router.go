package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/alfonsojimenez/kgazer/backend/internal/config"
	"github.com/alfonsojimenez/kgazer/backend/internal/progress"
	"github.com/alfonsojimenez/kgazer/backend/internal/status"
	"github.com/alfonsojimenez/kgazer/backend/internal/store"
)

func NewRouter(s *store.Store, t *status.Tracker, pt *progress.Tracker, admins *AdminClients, version string, startedAt time.Time, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/api/health", handleHealth(version))
	r.Get("/api/clusters", handleListClusters(t))
	r.Get("/api/topics", handleListTopics(s, pt))
	r.Get("/api/topics/{topic}", handleGetTopicDetail(s, pt))
	r.Post("/api/topics/{topic}/reconsume", handleReconsume(s, pt))
	r.Post("/api/clusters/{cluster}/pause", handlePauseCluster(t, true))
	r.Post("/api/clusters/{cluster}/resume", handlePauseCluster(t, false))
	r.Get("/api/topics/{topic}/keys", handleListKeys(s))
	r.Get("/api/topics/{topic}/keys/{key}/history", handleGetKeyHistory(s))
	r.Get("/api/topics/{topic}/history", handleGetKeyHistoryByQuery(s))
	r.Get("/api/topics/{topic}/fields", handleGetTopicFields(s))
	r.Get("/api/topics/{topic}/timeline", handleGetKeyTimeline(s))

	r.Get("/api/settings/info", handleSettingsInfo(s, t, version, startedAt, cfg))
	r.Get("/api/settings/orphaned-clusters", handleOrphanedClusters(s, t))
	r.Delete("/api/settings/clusters/{cluster}", handleDeleteClusterData(s))

	r.Get("/api/topics/{topic}/consumer-groups", handleTopicConsumerGroups(admins))
	r.Get("/api/topics/{topic}/consumer-groups/{group}", handleConsumerGroupLag(admins))
	r.Post("/api/topics/{topic}/consumer-groups/{group}/reset-offsets", handleResetConsumerGroupOffsets(admins))

	return r
}

func handleHealth(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	}
}

func handleListClusters(t *status.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, t.All())
	}
}

func handlePauseCluster(t *status.Tracker, pause bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cluster := chi.URLParam(r, "cluster")
		t.SetPaused(cluster, pause)
		action := "resumed"
		if pause {
			action = "paused"
		}
		slog.Info("consumer "+action, "cluster", cluster)
		writeJSON(w, http.StatusOK, map[string]any{"cluster": cluster, "paused": pause})
	}
}

func handleGetTopicDetail(s *store.Store, pt *progress.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		detail, err := s.GetTopicDetail(r.Context(), cluster, topic)
		if err != nil {
			slog.Error("getting topic detail", "error", err, "topic", topic)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}

		p := pt.Get(cluster, topic)
		writeJSON(w, http.StatusOK, map[string]any{
			"name":              detail.Name,
			"cluster":           detail.Cluster,
			"partitions":        detail.Partitions,
			"compacted":         detail.Compacted,
			"message_format":    detail.MessageFormat,
			"consumed_messages": detail.ConsumedMessages,
			"unique_keys":       detail.UniqueKeys,
			"progress":          p,
		})
	}
}

func handleReconsume(s *store.Store, pt *progress.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		deleted, err := s.DeleteTopicMessages(r.Context(), cluster, topic)
		if err != nil {
			slog.Error("deleting messages", "error", err, "topic", topic)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete messages"})
			return
		}
		pt.RemoveTopic(cluster, topic)
		pt.RequestReconsume(cluster, topic)
		slog.Info("reconsume triggered", "topic", topic, "cluster", cluster, "deleted", deleted)
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "topic": topic})
	}
}

func handleListTopics(s *store.Store, pt *progress.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cluster := r.URL.Query().Get("cluster")
		sortDir := r.URL.Query().Get("sort")
		topics, err := s.ListTopics(r.Context(), cluster, sortDir)
		if err != nil {
			slog.Error("listing topics", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		if topics == nil {
			topics = []store.TopicSummary{}
		}

		result := make([]map[string]any, len(topics))
		for i, t := range topics {
			result[i] = map[string]any{
				"name":           t.Name,
				"cluster":        t.Cluster,
				"partitions":     t.Partitions,
				"compacted":      t.Compacted,
				"message_format": t.MessageFormat,
				"message_count":  t.MessageCount,
				"key_count":      t.KeyCount,
				"last_updated":   t.LastUpdated,
				"progress":       pt.Get(t.Cluster, t.Name),
			}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleListKeys(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		search := r.URL.Query().Get("search")
		sortBy := r.URL.Query().Get("sort_by")
		sortDir := r.URL.Query().Get("sort")
		page := queryInt(r, "page", 1)
		limit := queryInt(r, "limit", 50)

		var partitions []int
		if p := r.URL.Query().Get("partition"); p != "" {
			for _, s := range strings.Split(p, ",") {
				if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					partitions = append(partitions, v)
				}
			}
		}

		var minOffset int64
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.ParseInt(o, 10, 64); err == nil {
				minOffset = v
			}
		}

		valueFilter := r.URL.Query().Get("value_filter")

		keys, total, err := s.ListKeys(r.Context(), cluster, topic, search, sortBy, sortDir, partitions, minOffset, valueFilter, page, limit)
		if errors.Is(err, store.ErrTopicNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		if err != nil {
			slog.Error("listing keys", "error", err, "topic", topic)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		if keys == nil {
			keys = []store.KeySummary{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  keys,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

func handleGetTopicFields(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		fields, err := s.GetTopicFields(r.Context(), cluster, topic)
		if err != nil {
			slog.Error("getting topic fields", "error", err, "topic", topic)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		if fields == nil {
			fields = []string{}
		}
		writeJSON(w, http.StatusOK, fields)
	}
}

func handleGetKeyHistory(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		key := chi.URLParam(r, "key")
		page := queryInt(r, "page", 1)
		limit := queryInt(r, "limit", 50)

		messages, total, err := s.GetKeyHistory(r.Context(), cluster, topic, key, page, limit)
		if err != nil {
			slog.Error("getting key history", "error", err, "topic", topic, "key", key)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		if messages == nil {
			messages = []store.Message{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  messages,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

func handleGetKeyTimeline(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		key := r.URL.Query().Get("key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key parameter required"})
			return
		}
		points, err := s.GetKeyTimeline(r.Context(), cluster, topic, key)
		if err != nil {
			slog.Error("getting key timeline", "error", err, "topic", topic, "key", key)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, points)
	}
}

func handleGetKeyHistoryByQuery(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		cluster := r.URL.Query().Get("cluster")
		key := r.URL.Query().Get("key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key query param required"})
			return
		}
		page := queryInt(r, "page", 1)
		limit := queryInt(r, "limit", 50)

		messages, total, err := s.GetKeyHistory(r.Context(), cluster, topic, key, page, limit)
		if err != nil {
			slog.Error("getting key history", "error", err, "topic", topic, "key", key)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		if messages == nil {
			messages = []store.Message{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  messages,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

type consumerGroupInfo struct {
	GroupID       string `json:"group_id"`
	MemberCount   int    `json:"member_count"`
	State         string `json:"state"`
	ActiveOnTopic int    `json:"active_on_topic"`
	TotalLag      int64  `json:"total_lag"`
}

type partitionLag struct {
	Partition       int32  `json:"partition"`
	CommittedOffset int64  `json:"committed_offset"`
	EndOffset       int64  `json:"end_offset"`
	Lag             int64  `json:"lag"`
	ConsumerID      string `json:"consumer_id"`
	ClientID        string `json:"client_id"`
	Host            string `json:"host"`
}

type consumerGroupLagDetail struct {
	GroupID       string         `json:"group_id"`
	State         string         `json:"state"`
	ActiveMembers int            `json:"active_members"`
	Partitions    []partitionLag `json:"partitions"`
	TotalLag      int64          `json:"total_lag"`
}

// AdminClients holds one long-lived Kafka AdminClient per configured cluster,
// created once at startup and reused across requests. Creating an AdminClient
// bootstraps a broker connection and metadata handshake, so creating one per
// HTTP request adds significant latency to every consumer-group lookup.
// confluent-kafka-go's AdminClient (backed by librdkafka) is documented as
// safe for concurrent use, so sharing one instance across concurrent request
// handlers is safe.
type AdminClients struct {
	clients map[string]*kafka.AdminClient
}

// NewAdminClients creates one AdminClient per configured cluster.
func NewAdminClients(clusters []config.Cluster) (*AdminClients, error) {
	clients := make(map[string]*kafka.AdminClient, len(clusters))
	for _, cluster := range clusters {
		admin, err := createAdminClient(cluster)
		if err != nil {
			for _, c := range clients {
				c.Close()
			}
			return nil, fmt.Errorf("creating admin client for cluster %s: %w", cluster.Name, err)
		}
		clients[cluster.Name] = admin
	}
	return &AdminClients{clients: clients}, nil
}

// Close closes all underlying admin clients. Call once during graceful shutdown.
func (a *AdminClients) Close() {
	if a == nil {
		return
	}
	for _, c := range a.clients {
		c.Close()
	}
}

func (a *AdminClients) get(name string) (*kafka.AdminClient, bool) {
	if a == nil {
		return nil, false
	}
	admin, ok := a.clients[name]
	return admin, ok
}

func createAdminClient(cluster config.Cluster) (*kafka.AdminClient, error) {
	cfg := &kafka.ConfigMap{
		"bootstrap.servers": cluster.BootstrapServers,
	}
	for k, v := range cluster.Properties {
		if err := cfg.SetKey(k, v); err != nil {
			return nil, fmt.Errorf("setting kafka property %s: %w", k, err)
		}
	}
	return kafka.NewAdminClient(cfg)
}

// maxConcurrentGroupLookups bounds how many ListConsumerGroupOffsets calls
// run in parallel when scanning every consumer group in the cluster for
// activity on a topic. The confluent-kafka-go API only supports looking up
// offsets for one consumer group per call, so without this the endpoint made
// one sequential broker round-trip per consumer group in the cluster.
const maxConcurrentGroupLookups = 16

func handleTopicConsumerGroups(admins *AdminClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		clusterName := r.URL.Query().Get("cluster")

		admin, ok := admins.get(clusterName)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found in config"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		meta, err := admin.GetMetadata(&topic, false, 10000)
		if err != nil {
			slog.Error("getting topic metadata", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get topic metadata"})
			return
		}
		topicMeta, ok := meta.Topics[topic]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}

		var partitions []kafka.TopicPartition
		offsetSpecs := make(map[kafka.TopicPartition]kafka.OffsetSpec)
		for _, p := range topicMeta.Partitions {
			partitions = append(partitions, kafka.TopicPartition{Topic: &topic, Partition: p.ID})
			offsetSpecs[kafka.TopicPartition{Topic: &topic, Partition: p.ID}] = kafka.LatestOffsetSpec
		}

		endResult, err := admin.ListOffsets(ctx, offsetSpecs)
		if err != nil {
			slog.Error("listing end offsets", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get end offsets"})
			return
		}
		endOffsets := make(map[int32]int64)
		for tp, info := range endResult.ResultInfos {
			if tp.Topic != nil && *tp.Topic == topic && info.Error.Code() == kafka.ErrNoError {
				endOffsets[tp.Partition] = int64(info.Offset)
			}
		}

		listResult, err := admin.ListConsumerGroups(ctx)
		if err != nil {
			slog.Error("listing consumer groups", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list consumer groups"})
			return
		}

		var groupIDs []string
		for _, g := range listResult.Valid {
			groupIDs = append(groupIDs, g.GroupID)
		}

		if len(groupIDs) == 0 {
			writeJSON(w, http.StatusOK, []consumerGroupInfo{})
			return
		}

		descResult, err := admin.DescribeConsumerGroups(ctx, groupIDs)
		if err != nil {
			slog.Error("describing consumer groups", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to describe consumer groups"})
			return
		}

		resultsByGroup := make([]*consumerGroupInfo, len(descResult.ConsumerGroupDescriptions))
		sem := make(chan struct{}, maxConcurrentGroupLookups)
		var wg sync.WaitGroup

		for i, desc := range descResult.ConsumerGroupDescriptions {
			if desc.Error.Code() != kafka.ErrNoError {
				continue
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(i int, desc kafka.ConsumerGroupDescription) {
				defer wg.Done()
				defer func() { <-sem }()

				activeOnTopic := 0
				for _, member := range desc.Members {
					for _, tp := range member.Assignment.TopicPartitions {
						if tp.Topic != nil && *tp.Topic == topic {
							activeOnTopic++
							break
						}
					}
				}

				offsetResult, lagErr := admin.ListConsumerGroupOffsets(ctx, []kafka.ConsumerGroupTopicPartitions{{
					Group: desc.GroupID, Partitions: partitions,
				}})

				var totalLag int64
				hasOffsets := false
				if lagErr == nil && len(offsetResult.ConsumerGroupsTopicPartitions) > 0 {
					for _, tp := range offsetResult.ConsumerGroupsTopicPartitions[0].Partitions {
						if tp.Topic == nil || *tp.Topic != topic {
							continue
						}
						end := endOffsets[tp.Partition]
						var committed int64
						if tp.Offset >= 0 {
							hasOffsets = true
							committed = int64(tp.Offset)
						}
						if lag := end - committed; lag > 0 {
							totalLag += lag
						}
					}
				}

				if activeOnTopic == 0 && !hasOffsets {
					return
				}

				resultsByGroup[i] = &consumerGroupInfo{
					GroupID:       desc.GroupID,
					MemberCount:   len(desc.Members),
					State:         desc.State.String(),
					ActiveOnTopic: activeOnTopic,
					TotalLag:      totalLag,
				}
			}(i, desc)
		}
		wg.Wait()

		var result []consumerGroupInfo
		for _, info := range resultsByGroup {
			if info != nil {
				result = append(result, *info)
			}
		}

		sort.Slice(result, func(i, j int) bool {
			return result[i].GroupID < result[j].GroupID
		})

		if result == nil {
			result = []consumerGroupInfo{}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleConsumerGroupLag(admins *AdminClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		group := chi.URLParam(r, "group")
		clusterName := r.URL.Query().Get("cluster")

		admin, ok := admins.get(clusterName)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		meta, err := admin.GetMetadata(&topic, false, 10000)
		if err != nil {
			slog.Error("getting topic metadata", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get topic metadata"})
			return
		}
		topicMeta, ok := meta.Topics[topic]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}

		var partitions []kafka.TopicPartition
		for _, p := range topicMeta.Partitions {
			partitions = append(partitions, kafka.TopicPartition{Topic: &topic, Partition: p.ID})
		}

		descResult, err := admin.DescribeConsumerGroups(ctx, []string{group})
		if err != nil {
			slog.Error("describing consumer group", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to describe consumer group"})
			return
		}

		type memberInfo struct {
			ConsumerID string
			ClientID   string
			Host       string
		}
		memberMap := make(map[int32]memberInfo)
		detail := consumerGroupLagDetail{GroupID: group}

		if len(descResult.ConsumerGroupDescriptions) > 0 {
			desc := descResult.ConsumerGroupDescriptions[0]
			detail.State = desc.State.String()
			for _, member := range desc.Members {
				assignedToTopic := false
				for _, tp := range member.Assignment.TopicPartitions {
					if tp.Topic != nil && *tp.Topic == topic {
						assignedToTopic = true
						memberMap[tp.Partition] = memberInfo{
							ConsumerID: member.ConsumerID,
							ClientID:   member.ClientID,
							Host:       member.Host,
						}
					}
				}
				if assignedToTopic {
					detail.ActiveMembers++
				}
			}
		}

		offsetResult, err := admin.ListConsumerGroupOffsets(ctx, []kafka.ConsumerGroupTopicPartitions{{
			Group: group, Partitions: partitions,
		}})
		if err != nil {
			slog.Error("listing consumer group offsets", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get committed offsets"})
			return
		}

		committedMap := make(map[int32]int64)
		if len(offsetResult.ConsumerGroupsTopicPartitions) > 0 {
			for _, tp := range offsetResult.ConsumerGroupsTopicPartitions[0].Partitions {
				if tp.Topic != nil && *tp.Topic == topic && tp.Offset >= 0 {
					committedMap[tp.Partition] = int64(tp.Offset)
				}
			}
		}

		offsetSpecs := make(map[kafka.TopicPartition]kafka.OffsetSpec)
		for _, p := range topicMeta.Partitions {
			offsetSpecs[kafka.TopicPartition{Topic: &topic, Partition: p.ID}] = kafka.LatestOffsetSpec
		}
		endResult, err := admin.ListOffsets(ctx, offsetSpecs)
		if err != nil {
			slog.Error("listing end offsets", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get end offsets"})
			return
		}

		for tp, info := range endResult.ResultInfos {
			if tp.Topic == nil || *tp.Topic != topic || info.Error.Code() != kafka.ErrNoError {
				continue
			}
			endOffset := int64(info.Offset)
			committed := committedMap[tp.Partition]
			lag := endOffset - committed
			if lag < 0 {
				lag = 0
			}
			mi := memberMap[tp.Partition]
			detail.Partitions = append(detail.Partitions, partitionLag{
				Partition:       tp.Partition,
				CommittedOffset: committed,
				EndOffset:       endOffset,
				Lag:             lag,
				ConsumerID:      mi.ConsumerID,
				ClientID:        mi.ClientID,
				Host:            mi.Host,
			})
			detail.TotalLag += lag
		}

		sort.Slice(detail.Partitions, func(i, j int) bool {
			return detail.Partitions[i].Partition < detail.Partitions[j].Partition
		})

		if detail.Partitions == nil {
			detail.Partitions = []partitionLag{}
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func handleResetConsumerGroupOffsets(admins *AdminClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := chi.URLParam(r, "topic")
		group := chi.URLParam(r, "group")
		clusterName := r.URL.Query().Get("cluster")
		target := r.URL.Query().Get("target")

		if target != "earliest" && target != "latest" && target != "specific" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target must be 'earliest', 'latest', or 'specific'"})
			return
		}

		admin, ok := admins.get(clusterName)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		descResult, err := admin.DescribeConsumerGroups(ctx, []string{group})
		if err != nil {
			slog.Error("describing consumer group", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to describe consumer group"})
			return
		}
		if len(descResult.ConsumerGroupDescriptions) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "consumer group not found"})
			return
		}
		desc := descResult.ConsumerGroupDescriptions[0]
		for _, member := range desc.Members {
			for _, tp := range member.Assignment.TopicPartitions {
				if tp.Topic != nil && *tp.Topic == topic {
					writeJSON(w, http.StatusConflict, map[string]string{
						"error": "there are active consumers assigned to this topic in this group",
					})
					return
				}
			}
		}

		var newPartitions []kafka.TopicPartition

		if target == "specific" {
			var body struct {
				Offsets []struct {
					Partition int32 `json:"partition"`
					Offset    int64 `json:"offset"`
				} `json:"offsets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			if len(body.Offsets) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offsets array is required"})
				return
			}
			for _, o := range body.Offsets {
				newPartitions = append(newPartitions, kafka.TopicPartition{
					Topic: &topic, Partition: o.Partition, Offset: kafka.Offset(o.Offset),
				})
			}
		} else {
			meta, err := admin.GetMetadata(&topic, false, 10000)
			if err != nil {
				slog.Error("getting topic metadata", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get topic metadata"})
				return
			}
			topicMeta, ok := meta.Topics[topic]
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
				return
			}

			spec := kafka.LatestOffsetSpec
			if target == "earliest" {
				spec = kafka.EarliestOffsetSpec
			}
			offsetSpecs := make(map[kafka.TopicPartition]kafka.OffsetSpec)
			for _, p := range topicMeta.Partitions {
				offsetSpecs[kafka.TopicPartition{Topic: &topic, Partition: p.ID}] = spec
			}
			targetResult, err := admin.ListOffsets(ctx, offsetSpecs)
			if err != nil {
				slog.Error("listing target offsets", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve target offsets"})
				return
			}
			for tp, info := range targetResult.ResultInfos {
				if tp.Topic != nil && *tp.Topic == topic && info.Error.Code() == kafka.ErrNoError {
					newPartitions = append(newPartitions, kafka.TopicPartition{
						Topic: tp.Topic, Partition: tp.Partition, Offset: info.Offset,
					})
				}
			}
		}

		alterResult, err := admin.AlterConsumerGroupOffsets(ctx, []kafka.ConsumerGroupTopicPartitions{{
			Group: group, Partitions: newPartitions,
		}})
		if err != nil {
			slog.Error("altering consumer group offsets", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset offsets"})
			return
		}

		if len(alterResult.ConsumerGroupsTopicPartitions) > 0 {
			for _, tp := range alterResult.ConsumerGroupsTopicPartitions[0].Partitions {
				if tp.Error != nil {
					slog.Error("partition offset reset failed", "group", group, "topic", topic, "partition", tp.Partition, "error", tp.Error)
					writeJSON(w, http.StatusConflict, map[string]string{
						"error": fmt.Sprintf("failed to reset offset for partition %d: %s — all consumers in the group must be stopped", tp.Partition, tp.Error.Error()),
					})
					return
				}
			}
		}

		slog.Info("consumer group offsets reset", "group", group, "topic", topic, "target", target, "partitions", len(newPartitions))
		writeJSON(w, http.StatusOK, map[string]any{
			"group": group, "topic": topic, "target": target, "partitions": len(newPartitions),
		})
	}
}

func handleSettingsInfo(s *store.Store, t *status.Tracker, version string, startedAt time.Time, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clusters := t.All()

		clusterConfigs := make([]map[string]any, 0, len(cfg.Kafka.Clusters))
		for _, c := range cfg.Kafka.Clusters {
			cc := map[string]any{
				"name":              c.Name,
				"bootstrap_servers": c.BootstrapServers,
				"schema_registry":   c.SchemaRegistry != "",
			}
			props := make(map[string]string)
			for k, v := range c.Properties {
				if k == "sasl.password" || k == "sasl.jaas.config" {
					v = "***"
				}
				props[k] = v
			}
			if len(props) > 0 {
				cc["properties"] = props
			}
			clusterConfigs = append(clusterConfigs, cc)
		}

		stats, _ := s.GetClusterStats(r.Context())
		var totalTopics, totalMessages, totalKeys int
		for _, cs := range stats {
			totalTopics += cs.TopicCount
			totalMessages += cs.MessageCount
			totalKeys += cs.KeyCount
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"version":        version,
			"started_at":     startedAt,
			"uptime":         time.Since(startedAt).Round(time.Second).String(),
			"clusters":       clusterConfigs,
			"cluster_status": clusters,
			"stats": map[string]int{
				"total_topics":   totalTopics,
				"total_messages": totalMessages,
				"total_keys":     totalKeys,
			},
			"config": map[string]any{
				"server_port":    cfg.KGazer.Server.Port,
				"db_host":        cfg.KGazer.DB.Host,
				"db_name":        cfg.KGazer.DB.Name,
				"compacted_only": cfg.KGazer.IsCompactedOnly(),
			},
		})
	}
}

func handleOrphanedClusters(s *store.Store, t *status.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allStats, err := s.GetClusterStats(r.Context())
		if err != nil {
			slog.Error("getting cluster stats", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		configuredNames := t.ConfiguredClusterNames()
		configSet := make(map[string]bool, len(configuredNames))
		for _, name := range configuredNames {
			configSet[name] = true
		}

		var orphaned []store.ClusterStats
		for _, cs := range allStats {
			if !configSet[cs.Name] {
				orphaned = append(orphaned, cs)
			}
		}

		if orphaned == nil {
			orphaned = []store.ClusterStats{}
		}
		writeJSON(w, http.StatusOK, orphaned)
	}
}

func handleDeleteClusterData(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cluster := chi.URLParam(r, "cluster")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := s.DeleteClusterData(ctx, cluster); err != nil {
				slog.Error("deleting cluster data", "error", err, "cluster", cluster)
			} else {
				slog.Info("cluster data deleted", "cluster", cluster)
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]any{"cluster": cluster, "deleting": true})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return defaultVal
	}
	return v
}
