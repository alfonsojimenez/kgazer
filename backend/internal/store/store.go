package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTopicNotFound = errors.New("topic not found")

type Store struct {
	pool      *pgxpool.Pool
	writePool *pgxpool.Pool
}

type Topic struct {
	ID           int       `json:"id"`
	Cluster      string    `json:"cluster"`
	Name         string    `json:"name"`
	Partitions   int       `json:"partitions"`
	Compacted    bool      `json:"compacted"`
	KafkaTopicID string    `json:"kafka_topic_id"`
	SyncedAt     time.Time `json:"synced_at"`
}

type TopicSummary struct {
	Name          string    `json:"name"`
	Cluster       string    `json:"cluster"`
	Partitions    int       `json:"partitions"`
	Compacted     bool      `json:"compacted"`
	MessageFormat string    `json:"message_format"`
	MessageCount  int       `json:"message_count"`
	KeyCount      int       `json:"key_count"`
	LastUpdated   time.Time `json:"last_updated"`
}

type KeySummary struct {
	Key          string    `json:"key"`
	MessageCount int       `json:"message_count"`
	Partition    int32     `json:"partition"`
	Offset       int64     `json:"offset"`
	LastUpdated  time.Time `json:"last_updated"`
}

type Message struct {
	ID        int64           `json:"id"`
	Key       string          `json:"key"`
	Body      json.RawMessage `json:"body"`
	Partition int32           `json:"partition"`
	Offset    int64           `json:"offset"`
	Timestamp time.Time       `json:"timestamp"`
}

type TopicDetail struct {
	Name             string `json:"name"`
	Cluster          string `json:"cluster"`
	Partitions       int    `json:"partitions"`
	Compacted        bool   `json:"compacted"`
	MessageFormat    string `json:"message_format"`
	ConsumedMessages int    `json:"consumed_messages"`
	UniqueKeys       int    `json:"unique_keys"`
}

func New(ctx context.Context, dsn string) (*Store, error) {
	readCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}
	readCfg.MaxConns = 10
	readCfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, readCfg)
	if err != nil {
		return nil, fmt.Errorf("creating read pool: %w", err)
	}

	writeCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}
	writeCfg.MaxConns = 10
	writeCfg.MinConns = 2

	writePool, err := pgxpool.NewWithConfig(ctx, writeCfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("creating write pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		writePool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &Store{pool: pool, writePool: writePool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
	s.writePool.Close()
}

func (s *Store) WritePool() *pgxpool.Pool {
	return s.writePool
}

type KeyEntry struct {
	TopicID   int
	Key       string
	Count     int
	Partition int32
	Offset    int64
	Timestamp time.Time
	Body      []byte
}

type PendingMessage struct {
	TopicID   int
	Key       string
	Body      []byte
	Format    string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

func (s *Store) SaveMessageBatch(ctx context.Context, msgs []PendingMessage) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	var b strings.Builder
	b.Grow(len(msgs) * 40)
	b.WriteString("INSERT INTO messages (topic_id, key, body, format, partition, offset_id, timestamp) VALUES ")
	args := make([]interface{}, 0, len(msgs)*7)
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 7
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7)
		args = append(args, m.TopicID, m.Key, m.Body, m.Format, m.Partition, m.Offset, m.Timestamp)
	}
	b.WriteString(" ON CONFLICT (topic_id, partition, offset_id) DO NOTHING")

	tag, err := s.writePool.Exec(ctx, b.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("batch saving messages: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) UpsertTopic(ctx context.Context, cluster, name string, partitions int, compacted bool, kafkaTopicID string) (id int, recreated bool, err error) {
	query := `
		WITH prev AS (
			SELECT id, kafka_topic_id FROM topics WHERE cluster = $1 AND name = $2
		)
		INSERT INTO topics (cluster, name, partitions, compacted, kafka_topic_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (cluster, name) DO UPDATE
		SET partitions = EXCLUDED.partitions,
		    compacted = EXCLUDED.compacted,
		    kafka_topic_id = EXCLUDED.kafka_topic_id,
		    synced_at = NOW()
		RETURNING id, (SELECT kafka_topic_id FROM prev)`

	var prevKafkaTopicID *string
	err = s.writePool.QueryRow(ctx, query, cluster, name, partitions, compacted, kafkaTopicID).Scan(&id, &prevKafkaTopicID)
	if err != nil {
		return 0, false, fmt.Errorf("upserting topic: %w", err)
	}

	recreated = prevKafkaTopicID != nil && *prevKafkaTopicID != "" && kafkaTopicID != "" && *prevKafkaTopicID != kafkaTopicID
	return id, recreated, nil
}

func (s *Store) PurgeTopicData(ctx context.Context, topicID int) error {
	if _, err := s.writePool.Exec(ctx, `DELETE FROM messages WHERE topic_id = $1`, topicID); err != nil {
		return fmt.Errorf("purging messages: %w", err)
	}
	if _, err := s.writePool.Exec(ctx, `DELETE FROM keys WHERE topic_id = $1`, topicID); err != nil {
		return fmt.Errorf("purging keys: %w", err)
	}
	_, err := s.writePool.Exec(ctx,
		`UPDATE topics SET message_count = 0, key_count = 0, last_message_at = NULL, message_format = 'unknown' WHERE id = $1`,
		topicID)
	if err != nil {
		return fmt.Errorf("resetting topic stats: %w", err)
	}
	s.writePool.Exec(ctx, `ANALYZE messages`)
	s.writePool.Exec(ctx, `ANALYZE keys`)
	return nil
}

func (s *Store) DeleteTopic(ctx context.Context, topicID int) error {
	if _, err := s.writePool.Exec(ctx, `DELETE FROM messages WHERE topic_id = $1`, topicID); err != nil {
		return fmt.Errorf("deleting messages: %w", err)
	}
	if _, err := s.writePool.Exec(ctx, `DELETE FROM keys WHERE topic_id = $1`, topicID); err != nil {
		return fmt.Errorf("deleting keys: %w", err)
	}
	if _, err := s.writePool.Exec(ctx, `DELETE FROM topics WHERE id = $1`, topicID); err != nil {
		return fmt.Errorf("deleting topic: %w", err)
	}
	return nil
}

func (s *Store) RemoveStaleTopics(ctx context.Context, cluster string, activeNames []string) ([]string, error) {
	dbTopics, err := s.ListTopicsByCluster(ctx, cluster)
	if err != nil {
		return nil, err
	}

	activeSet := make(map[string]bool, len(activeNames))
	for _, name := range activeNames {
		activeSet[name] = true
	}

	var removed []string
	for _, t := range dbTopics {
		if !activeSet[t.Name] {
			if err := s.DeleteTopic(ctx, t.ID); err != nil {
				return removed, fmt.Errorf("deleting stale topic %s: %w", t.Name, err)
			}
			removed = append(removed, t.Name)
		}
	}
	return removed, nil
}

type ClusterStats struct {
	Name         string `json:"name"`
	TopicCount   int    `json:"topic_count"`
	MessageCount int    `json:"message_count"`
	KeyCount     int    `json:"key_count"`
}

func (s *Store) GetClusterStats(ctx context.Context) ([]ClusterStats, error) {
	query := `
		SELECT cluster,
		       COUNT(*) as topic_count,
		       COALESCE(SUM(message_count), 0) as message_count,
		       COALESCE(SUM(key_count), 0) as key_count
		FROM topics
		GROUP BY cluster
		ORDER BY cluster`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("getting cluster stats: %w", err)
	}
	defer rows.Close()

	var stats []ClusterStats
	for rows.Next() {
		var cs ClusterStats
		if err := rows.Scan(&cs.Name, &cs.TopicCount, &cs.MessageCount, &cs.KeyCount); err != nil {
			return nil, fmt.Errorf("scanning cluster stats: %w", err)
		}
		stats = append(stats, cs)
	}
	return stats, rows.Err()
}

func (s *Store) DeleteClusterData(ctx context.Context, cluster string) error {
	topics, err := s.ListTopicsByCluster(ctx, cluster)
	if err != nil {
		return fmt.Errorf("listing topics for cluster %s: %w", cluster, err)
	}
	for _, t := range topics {
		if err := s.DeleteTopic(ctx, t.ID); err != nil {
			return fmt.Errorf("deleting topic %s: %w", t.Name, err)
		}
	}
	return nil
}

type PartitionOffset struct {
	Partition int32
	Offset    int64
}

func (s *Store) GetMaxOffsets(ctx context.Context, topicID int) ([]PartitionOffset, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT partition, MAX(offset_id) FROM keys WHERE topic_id = $1 GROUP BY partition ORDER BY partition`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PartitionOffset
	for rows.Next() {
		var po PartitionOffset
		if err := rows.Scan(&po.Partition, &po.Offset); err != nil {
			return nil, err
		}
		result = append(result, po)
	}
	return result, rows.Err()
}

func (s *Store) GetTopicID(ctx context.Context, cluster, name string) (int, error) {
	var id int
	err := s.pool.QueryRow(ctx, `SELECT id FROM topics WHERE cluster = $1 AND name = $2`, cluster, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("getting topic id: %w", err)
	}
	return id, nil
}

func (s *Store) ListTopicsByCluster(ctx context.Context, cluster string) ([]Topic, error) {
	query := `SELECT id, cluster, name, partitions, compacted, COALESCE(kafka_topic_id, ''), synced_at FROM topics WHERE cluster = $1 ORDER BY name`

	rows, err := s.pool.Query(ctx, query, cluster)
	if err != nil {
		return nil, fmt.Errorf("listing topics by cluster: %w", err)
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		if err := rows.Scan(&t.ID, &t.Cluster, &t.Name, &t.Partitions, &t.Compacted, &t.KafkaTopicID, &t.SyncedAt); err != nil {
			return nil, fmt.Errorf("scanning topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

func (s *Store) IncrementTopicStats(ctx context.Context, topicID, newMessages, newKeys int, latestTimestamp time.Time, format string) error {
	query := `
		UPDATE topics SET
			message_count = message_count + $2,
			key_count = key_count + $3,
			last_message_at = GREATEST(last_message_at, $4),
			message_format = CASE
				WHEN $5 IN ('tombstone', 'other') THEN message_format
				WHEN message_format = 'unknown' THEN $5
				WHEN message_format = $5 THEN $5
				ELSE 'other'
			END
		WHERE id = $1`
	_, err := s.writePool.Exec(ctx, query, topicID, newMessages, newKeys, latestTimestamp, format)
	return err
}

func (s *Store) ResetTopicStats(ctx context.Context, cluster, topic string) error {
	query := `UPDATE topics SET message_count = 0, key_count = 0, last_message_at = NULL, message_format = 'unknown' WHERE cluster = $1 AND name = $2`
	_, err := s.writePool.Exec(ctx, query, cluster, topic)
	return err
}

func (s *Store) ListTopics(ctx context.Context, cluster, sortDir string) ([]TopicSummary, error) {
	query := `
		SELECT name, cluster, partitions, compacted, message_format,
		       message_count, key_count,
		       COALESCE(last_message_at, synced_at)
		FROM topics
		WHERE ($1 = '' OR cluster = $1)
		ORDER BY COALESCE(last_message_at, synced_at) ` + sanitizeSortDir(sortDir)

	rows, err := s.pool.Query(ctx, query, cluster)
	if err != nil {
		return nil, fmt.Errorf("listing topics: %w", err)
	}
	defer rows.Close()

	var topics []TopicSummary
	for rows.Next() {
		var t TopicSummary
		if err := rows.Scan(&t.Name, &t.Cluster, &t.Partitions, &t.Compacted, &t.MessageFormat, &t.MessageCount, &t.KeyCount, &t.LastUpdated); err != nil {
			return nil, fmt.Errorf("scanning topic: %w", err)
		}
		topics = append(topics, t)
	}

	return topics, rows.Err()
}

func (s *Store) GetTopicDetail(ctx context.Context, cluster, topicName string) (*TopicDetail, error) {
	query := `
		SELECT name, cluster, partitions, compacted, message_count, key_count, message_format
		FROM topics
		WHERE cluster = $1 AND name = $2
		LIMIT 1`

	var d TopicDetail
	err := s.pool.QueryRow(ctx, query, cluster, topicName).Scan(
		&d.Name, &d.Cluster, &d.Partitions, &d.Compacted,
		&d.ConsumedMessages, &d.UniqueKeys, &d.MessageFormat,
	)
	if err != nil {
		return nil, fmt.Errorf("getting topic detail: %w", err)
	}

	return &d, nil
}

func (s *Store) DeleteTopicMessages(ctx context.Context, cluster, topic string) (int64, error) {
	tag, err := s.writePool.Exec(ctx,
		`DELETE FROM messages WHERE topic_id = (SELECT id FROM topics WHERE cluster = $1 AND name = $2)`,
		cluster, topic)
	if err != nil {
		return 0, fmt.Errorf("deleting messages: %w", err)
	}
	s.DeleteTopicKeys(ctx, cluster, topic)
	s.ResetTopicStats(ctx, cluster, topic)
	return tag.RowsAffected(), nil
}

func (s *Store) UpsertKey(ctx context.Context, topicID int, key string, partition int32, offset int64, timestamp time.Time, body []byte) error {
	query := `
		INSERT INTO keys (topic_id, key, message_count, partition, offset_id, last_updated, body)
		VALUES ($1, $2, 1, $3, $4, $5, $6)
		ON CONFLICT (topic_id, key) DO UPDATE SET
			message_count = keys.message_count + 1,
			partition = EXCLUDED.partition,
			offset_id = EXCLUDED.offset_id,
			last_updated = GREATEST(keys.last_updated, EXCLUDED.last_updated),
			body = EXCLUDED.body`
	_, err := s.writePool.Exec(ctx, query, topicID, key, partition, offset, timestamp, body)
	return err
}

func (s *Store) UpsertKeyBatch(ctx context.Context, entries []KeyEntry) error {
	if len(entries) == 0 {
		return nil
	}
	query := `INSERT INTO keys (topic_id, key, message_count, partition, offset_id, last_updated, body) VALUES `
	args := make([]interface{}, 0, len(entries)*7)
	for i, e := range entries {
		if i > 0 {
			query += ","
		}
		base := i * 7
		query += fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7)
		args = append(args, e.TopicID, e.Key, e.Count, e.Partition, e.Offset, e.Timestamp, e.Body)
	}
	query += ` ON CONFLICT (topic_id, key) DO UPDATE SET
		message_count = keys.message_count + EXCLUDED.message_count,
		partition = EXCLUDED.partition,
		offset_id = EXCLUDED.offset_id,
		last_updated = GREATEST(keys.last_updated, EXCLUDED.last_updated),
		body = EXCLUDED.body`
	_, err := s.writePool.Exec(ctx, query, args...)
	return err
}

func (s *Store) DeleteTopicKeys(ctx context.Context, cluster, topic string) error {
	_, err := s.writePool.Exec(ctx,
		`DELETE FROM keys WHERE topic_id = (SELECT id FROM topics WHERE cluster = $1 AND name = $2)`,
		cluster, topic)
	return err
}

func (s *Store) GetTopicFields(ctx context.Context, cluster, topic string) ([]string, error) {
	var topicID int
	err := s.pool.QueryRow(ctx, `SELECT id FROM topics WHERE cluster = $1 AND name = $2`, cluster, topic).Scan(&topicID)
	if err != nil {
		return nil, fmt.Errorf("getting topic: %w", err)
	}

	query := `SELECT jsonb_object_keys(sub.body) FROM (SELECT body FROM keys WHERE topic_id = $1 AND body IS NOT NULL LIMIT 1) sub`
	rows, err := s.pool.Query(ctx, query, topicID)
	if err != nil {
		return nil, fmt.Errorf("getting topic fields: %w", err)
	}
	defer rows.Close()

	var fields []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("scanning field: %w", err)
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func (s *Store) ListKeys(ctx context.Context, cluster, topic, search, sortBy, sortDir string, partitions []int, minOffset int64, valueFilter string, page, limit int) ([]KeySummary, int, error) {
	pgOffset := (page - 1) * limit

	var topicID int
	var keyCount int
	err := s.pool.QueryRow(ctx,
		`SELECT id, key_count FROM topics WHERE cluster = $1 AND name = $2`,
		cluster, topic).Scan(&topicID, &keyCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrTopicNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("getting topic: %w", err)
	}

	hasFilters := search != "" || len(partitions) > 0 || minOffset > 0 || valueFilter != ""

	conditions := []string{"topic_id = $1"}
	args := []interface{}{topicID}
	searchIdx := 0

	if search != "" {
		args = append(args, search)
		searchIdx = len(args)
		conditions = append(conditions, fmt.Sprintf("key ILIKE '%%' || $%d || '%%'", searchIdx))
	}

	if len(partitions) > 0 {
		args = append(args, partitions)
		conditions = append(conditions, fmt.Sprintf("partition = ANY($%d)", len(args)))
	}

	if minOffset > 0 {
		args = append(args, minOffset)
		conditions = append(conditions, fmt.Sprintf("offset_id = $%d", len(args)))
	}

	if valueFilter != "" {
		args = append(args, valueFilter)
		conditions = append(conditions, fmt.Sprintf("body @> $%d::jsonb", len(args)))
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	sortColumn := "last_updated"
	if sortBy == "offset" {
		sortColumn = "offset_id"
	}
	dir := sanitizeSortDir(sortDir)

	orderBy := sortColumn + " " + dir
	if search != "" && searchIdx > 0 {
		orderBy = fmt.Sprintf("(key = $%d) DESC, (key ILIKE $%d || '%%%%') DESC, %s", searchIdx, searchIdx, orderBy)
	}

	args = append(args, limit, pgOffset)

	var total int
	if !hasFilters {
		total = keyCount
	}

	selectCols := "key, message_count, partition, offset_id, last_updated"
	if hasFilters {
		selectCols = "key, message_count, partition, offset_id, last_updated, COUNT(*) OVER() AS total"
	}

	query := fmt.Sprintf(
		"SELECT %s FROM keys %s ORDER BY %s LIMIT $%d OFFSET $%d",
		selectCols, where, orderBy, len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing keys: %w", err)
	}
	defer rows.Close()

	var keys []KeySummary
	for rows.Next() {
		var k KeySummary
		if hasFilters {
			if err := rows.Scan(&k.Key, &k.MessageCount, &k.Partition, &k.Offset, &k.LastUpdated, &total); err != nil {
				return nil, 0, fmt.Errorf("scanning key: %w", err)
			}
		} else {
			if err := rows.Scan(&k.Key, &k.MessageCount, &k.Partition, &k.Offset, &k.LastUpdated); err != nil {
				return nil, 0, fmt.Errorf("scanning key: %w", err)
			}
		}
		keys = append(keys, k)
	}

	return keys, total, rows.Err()
}

func (s *Store) GetKeyHistory(ctx context.Context, cluster, topic, key string, page, limit int) ([]Message, int, error) {
	pgOffset := (page - 1) * limit

	query := `
		WITH t AS (
			SELECT id FROM topics WHERE cluster = $1 AND name = $2
		)
		SELECT key, body, partition, offset_id, timestamp,
		       COUNT(*) OVER() AS total
		FROM messages
		WHERE topic_id = (SELECT id FROM t) AND key = $3
		ORDER BY timestamp DESC
		LIMIT $4 OFFSET $5`

	rows, err := s.pool.Query(ctx, query, cluster, topic, key, limit, pgOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying key history: %w", err)
	}
	defer rows.Close()

	var messages []Message
	var total int
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Key, &m.Body, &m.Partition, &m.Offset, &m.Timestamp, &total); err != nil {
			return nil, 0, fmt.Errorf("scanning message: %w", err)
		}
		m.ID = m.Offset
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return messages, total, nil
}

func sanitizeSortDir(dir string) string {
	if dir == "asc" {
		return "ASC"
	}
	return "DESC"
}
