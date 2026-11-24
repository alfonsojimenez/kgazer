package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	Key          string     `json:"key"`
	MessageCount int        `json:"message_count"`
	Partition    int32      `json:"partition"`
	Offset       int64      `json:"offset"`
	LastUpdated  time.Time  `json:"last_updated"`
	DeletedAt    *time.Time `json:"deleted_at"`
}

const (
	KeyStatusAll     = ""
	KeyStatusLive    = "live"
	KeyStatusDeleted = "deleted"
)

type Header struct {
	Key    string  `json:"key"`
	Value  *string `json:"value"`
	Base64 bool    `json:"base64,omitempty"`
}

type Message struct {
	ID        int64           `json:"id"`
	Key       string          `json:"key"`
	Body      json.RawMessage `json:"body"`
	Headers   json.RawMessage `json:"headers"`
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
	DeletedKeys      int    `json:"deleted_keys"`
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
	Headers   []byte
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
	b.WriteString("INSERT INTO messages (topic_id, key, body, headers, format, partition, offset_id, timestamp) VALUES ")
	args := make([]interface{}, 0, len(msgs)*8)
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte(',')
		}
		base := i * 8
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
		args = append(args, m.TopicID, m.Key, m.Body, m.Headers, m.Format, m.Partition, m.Offset, m.Timestamp)
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
	// One primary-key probe per partition instead of aggregating every key of the topic.
	rows, err := s.pool.Query(ctx, `
		SELECT p.partition, o.offset_id
		FROM generate_series(0, (SELECT partitions - 1 FROM topics WHERE id = $1)) AS p(partition)
		CROSS JOIN LATERAL (
			SELECT offset_id FROM messages WHERE topic_id = $1 AND partition = p.partition
			ORDER BY offset_id DESC LIMIT 1
		) o
		ORDER BY p.partition`, topicID)
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
		SELECT t.name, t.cluster, t.partitions, t.compacted, t.message_count, t.key_count, t.message_format,
		       (SELECT COUNT(*) FROM keys k WHERE k.topic_id = t.id AND k.deleted_at IS NOT NULL)
		FROM topics t
		WHERE t.cluster = $1 AND t.name = $2
		LIMIT 1`

	var d TopicDetail
	err := s.pool.QueryRow(ctx, query, cluster, topicName).Scan(
		&d.Name, &d.Cluster, &d.Partitions, &d.Compacted,
		&d.ConsumedMessages, &d.UniqueKeys, &d.MessageFormat, &d.DeletedKeys,
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
		INSERT INTO keys (topic_id, key, message_count, partition, offset_id, last_updated, body, deleted_at)
		VALUES ($1, $2, 1, $3, $4, $5, $6, CASE WHEN $6::jsonb = 'null'::jsonb THEN $5::timestamptz END)
		ON CONFLICT (topic_id, key) DO UPDATE SET
			message_count = keys.message_count + 1,
			partition = EXCLUDED.partition,
			offset_id = EXCLUDED.offset_id,
			last_updated = GREATEST(keys.last_updated, EXCLUDED.last_updated),
			body = EXCLUDED.body,
			deleted_at = EXCLUDED.deleted_at`
	_, err := s.writePool.Exec(ctx, query, topicID, key, partition, offset, timestamp, body)
	return err
}

// UpsertKeyBatch returns, per topic ID, how many keys were inserted (not updated).
func (s *Store) UpsertKeyBatch(ctx context.Context, entries []KeyEntry) (map[int]int, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	query := `INSERT INTO keys (topic_id, key, message_count, partition, offset_id, last_updated, body, deleted_at) VALUES `
	args := make([]interface{}, 0, len(entries)*8)
	for i, e := range entries {
		if i > 0 {
			query += ","
		}
		base := i * 8
		query += fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
		var deletedAt *time.Time
		if string(e.Body) == "null" {
			deletedAt = &e.Timestamp
		}
		args = append(args, e.TopicID, e.Key, e.Count, e.Partition, e.Offset, e.Timestamp, e.Body, deletedAt)
	}
	query += ` ON CONFLICT (topic_id, key) DO UPDATE SET
		message_count = keys.message_count + EXCLUDED.message_count,
		partition = EXCLUDED.partition,
		offset_id = EXCLUDED.offset_id,
		last_updated = GREATEST(keys.last_updated, EXCLUDED.last_updated),
		body = EXCLUDED.body,
		deleted_at = EXCLUDED.deleted_at
		RETURNING topic_id, xmax = 0`
	rows, err := s.writePool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	newKeys := make(map[int]int)
	for rows.Next() {
		var topicID int
		var inserted bool
		if err := rows.Scan(&topicID, &inserted); err != nil {
			return nil, err
		}
		if inserted {
			newKeys[topicID]++
		}
	}
	return newKeys, rows.Err()
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

	var body []byte
	err = s.pool.QueryRow(ctx,
		`SELECT body FROM keys WHERE topic_id = $1 AND body IS NOT NULL LIMIT 1`,
		topicID).Scan(&body)
	if err != nil {
		return []string{}, nil
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return []string{}, nil
	}

	fields := extractFields(data, "")
	if fields == nil {
		fields = []string{}
	}
	return fields, nil
}

func extractFields(data interface{}, prefix string) []string {
	var fields []string
	switch v := data.(type) {
	case map[string]interface{}:
		for _, key := range sortedKeys(v) {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			childFields := extractFields(v[key], path)
			if len(childFields) > 0 {
				fields = append(fields, childFields...)
			} else {
				fields = append(fields, path)
			}
		}
	case []interface{}:
		if len(v) > 0 {
			fields = append(fields, extractFields(v[0], prefix+"[]")...)
		} else {
			fields = append(fields, prefix+"[]")
		}
	default:
		if prefix != "" {
			fields = append(fields, prefix)
		}
	}
	return fields
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AsOf resolves each key's latest version at or before that instant from the
// messages table instead of the current-state keys table.
type ListKeysParams struct {
	Search      string
	SortBy      string
	SortDir     string
	Partitions  []int
	MinOffset   int64
	ValueFilter string
	Status      string
	AsOf        *time.Time
	Page        int
	Limit       int
}

type keysQuery struct {
	topicID     int
	keyCount    int
	source      string
	where       string
	orderBy     string
	args        []interface{}
	hasFilters  bool
	countColumn string
	// branches holds the two halves of an as_of snapshot so ListKeys can push
	// ORDER BY/LIMIT into each one; empty when AsOf is nil.
	branches []string
}

// Columns every key source exposes; filters, sorting and paging run on top of them.
const keyColumns = "topic_id, key, partition, offset_id, last_updated, body, message_count, deleted_at"

func (s *Store) buildKeysQuery(ctx context.Context, cluster, topic string, p ListKeysParams) (*keysQuery, error) {
	q := &keysQuery{source: "keys", countColumn: "message_count"}
	err := s.pool.QueryRow(ctx,
		`SELECT id, key_count FROM topics WHERE cluster = $1 AND name = $2`,
		cluster, topic).Scan(&q.topicID, &q.keyCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTopicNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting topic: %w", err)
	}

	q.hasFilters = p.Search != "" || len(p.Partitions) > 0 || p.MinOffset > 0 || p.ValueFilter != "" || p.AsOf != nil || p.Status != KeyStatusAll

	conditions := []string{"topic_id = $1"}
	q.args = []interface{}{q.topicID}

	if p.AsOf != nil {
		q.args = append(q.args, *p.AsOf)
		asOfIdx := len(q.args)
		// Keys untouched since as_of are already correct in the keys table; only keys
		// changed afterwards need their historical version looked up in messages.
		unchanged := fmt.Sprintf(`SELECT %s FROM keys WHERE topic_id = $1 AND last_updated <= $%d`, keyColumns, asOfIdx)
		// The body is fetched lazily by primary key so it is only detoasted for rows a
		// value filter or the export actually reads, not for every changed key.
		changed := fmt.Sprintf(`SELECT k.topic_id, k.key, m.partition, m.offset_id, m.timestamp AS last_updated,
				(SELECT body FROM messages b WHERE b.topic_id = $1 AND b.partition = m.partition AND b.offset_id = m.offset_id) AS body,
				NULL::int AS message_count, CASE WHEN m.format = 'tombstone' THEN m.timestamp END AS deleted_at
			FROM keys k CROSS JOIN LATERAL (
				SELECT partition, offset_id, timestamp, format FROM messages
				WHERE topic_id = k.topic_id AND key = k.key AND timestamp <= $%[1]d
				ORDER BY timestamp DESC, offset_id DESC LIMIT 1
			) m
			WHERE k.topic_id = $1 AND k.last_updated > $%[1]d`, asOfIdx)
		q.branches = []string{unchanged, changed}
		q.source = fmt.Sprintf("(%s UNION ALL %s) AS snapshot", unchanged, changed)
		q.countColumn = fmt.Sprintf(`(SELECT COUNT(*) FROM messages c
			WHERE c.topic_id = $1 AND c.key = snapshot.key AND c.timestamp <= $%d)`, asOfIdx)
	}

	switch p.Status {
	case KeyStatusLive:
		conditions = append(conditions, "deleted_at IS NULL")
	case KeyStatusDeleted:
		conditions = append(conditions, "deleted_at IS NOT NULL")
	}

	searchIdx := 0
	if p.Search != "" {
		q.args = append(q.args, p.Search)
		searchIdx = len(q.args)
		conditions = append(conditions, fmt.Sprintf("key ILIKE '%%' || $%d || '%%'", searchIdx))
	}

	if len(p.Partitions) > 0 {
		q.args = append(q.args, p.Partitions)
		conditions = append(conditions, fmt.Sprintf("partition = ANY($%d)", len(q.args)))
	}

	if p.MinOffset > 0 {
		q.args = append(q.args, p.MinOffset)
		conditions = append(conditions, fmt.Sprintf("offset_id >= $%d", len(q.args)))
	}

	if p.ValueFilter != "" {
		q.args = append(q.args, p.ValueFilter)
		conditions = append(conditions, fmt.Sprintf("body @> $%d::jsonb", len(q.args)))
	}

	q.where = "WHERE " + strings.Join(conditions, " AND ")

	sortColumn := "last_updated"
	switch {
	case p.SortBy == "offset":
		sortColumn = "offset_id"
	case p.SortBy == "deleted_at", p.Status == KeyStatusDeleted && p.AsOf == nil:
		// Tombstones set deleted_at = last_updated, so this keeps the order and uses the partial index.
		sortColumn = "deleted_at"
	}
	q.orderBy = sortColumn + " " + sanitizeSortDir(p.SortDir)
	if searchIdx > 0 {
		q.orderBy = fmt.Sprintf("(key = $%d) DESC, (key ILIKE $%d || '%%%%') DESC, %s", searchIdx, searchIdx, q.orderBy)
	}
	return q, nil
}

func (q *keysQuery) isOnlyLiveFilter(p ListKeysParams) bool {
	return p.Status == KeyStatusLive && p.Search == "" && len(p.Partitions) == 0 &&
		p.MinOffset == 0 && p.ValueFilter == "" && p.AsOf == nil
}

func (s *Store) readOnlyTx(ctx context.Context, valueFilter string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("beginning tx: %w", err)
	}
	if valueFilter != "" {
		if _, err := tx.Exec(ctx, "SET LOCAL enable_bitmapscan = off"); err != nil {
			tx.Rollback(ctx)
			return nil, fmt.Errorf("setting enable_bitmapscan: %w", err)
		}
	}
	return tx, nil
}

func (s *Store) ListKeys(ctx context.Context, cluster, topic string, p ListKeysParams) ([]KeySummary, int, error) {
	q, err := s.buildKeysQuery(ctx, cluster, topic, p)
	if err != nil {
		return nil, 0, err
	}

	countArgs := q.args
	args := append(append([]interface{}{}, q.args...), p.Limit, (p.Page-1)*p.Limit)
	limitIdx, offsetIdx := len(args)-1, len(args)

	var total int
	if !q.hasFilters {
		total = q.keyCount
	}
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", q.source, q.where)
	if q.isOnlyLiveFilter(p) {
		// live = all keys minus tombstoned ones; the latter is a cheap partial-index count.
		countQuery = "SELECT $2::int - COUNT(*) FROM keys WHERE topic_id = $1 AND deleted_at IS NOT NULL"
		countArgs = []interface{}{q.topicID, q.keyCount}
	}

	source := q.source
	if len(q.branches) > 0 {
		// Each branch is ordered and limited before the union so PostgreSQL walks indexes
		// for just the requested page instead of materialising the whole snapshot.
		perBranch := make([]string, len(q.branches))
		for i, b := range q.branches {
			perBranch[i] = fmt.Sprintf("(SELECT * FROM (%s) AS b %s ORDER BY %s LIMIT $%d::bigint + $%d::bigint)",
				b, q.where, q.orderBy, limitIdx, offsetIdx)
		}
		source = fmt.Sprintf("(%s) AS snapshot", strings.Join(perBranch, " UNION ALL "))
	}

	dataQuery := fmt.Sprintf(
		"SELECT key, %s, partition, offset_id, last_updated, deleted_at FROM %s %s ORDER BY %s LIMIT $%d OFFSET $%d",
		q.countColumn, source, q.where, q.orderBy, limitIdx, offsetIdx)

	var (
		keys     []KeySummary
		dataErr  error
		countErr error
		wg       sync.WaitGroup
	)

	if q.hasFilters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			countCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()
			countErr = s.pool.QueryRow(countCtx, countQuery, countArgs...).Scan(&total)
			if countErr != nil {
				total = -1
				countErr = nil
			}
		}()
	}

	tx, err := s.readOnlyTx(ctx, p.ValueFilter)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k KeySummary
		if err := rows.Scan(&k.Key, &k.MessageCount, &k.Partition, &k.Offset, &k.LastUpdated, &k.DeletedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning key: %w", err)
		}
		keys = append(keys, k)
	}
	dataErr = rows.Err()

	wg.Wait()
	if countErr != nil {
		return nil, 0, fmt.Errorf("counting keys: %w", countErr)
	}

	return keys, total, dataErr
}

type ExportedKey struct {
	KeySummary
	Body json.RawMessage `json:"body"`
}

// ExportKeys ignores pagination and streams rows to fn without buffering them.
func (s *Store) ExportKeys(ctx context.Context, cluster, topic string, p ListKeysParams, fn func(ExportedKey) error) error {
	q, err := s.buildKeysQuery(ctx, cluster, topic, p)
	if err != nil {
		return err
	}

	tx, err := s.readOnlyTx(ctx, p.ValueFilter)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, fmt.Sprintf(
		"SELECT key, %s, partition, offset_id, last_updated, deleted_at, COALESCE(body, 'null'::jsonb) FROM %s %s ORDER BY %s",
		q.countColumn, q.source, q.where, q.orderBy), q.args...)
	if err != nil {
		return fmt.Errorf("exporting keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k ExportedKey
		if err := rows.Scan(&k.Key, &k.MessageCount, &k.Partition, &k.Offset, &k.LastUpdated, &k.DeletedAt, &k.Body); err != nil {
			return fmt.Errorf("scanning key: %w", err)
		}
		if err := fn(k); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) ExportKeyHistory(ctx context.Context, cluster, topic, key string, fn func(Message) error) error {
	rows, err := s.pool.Query(ctx, `
		SELECT key, body, COALESCE(headers, '[]'::jsonb), partition, offset_id, timestamp
		FROM messages
		WHERE topic_id = (SELECT id FROM topics WHERE cluster = $1 AND name = $2) AND key = $3
		ORDER BY timestamp DESC, offset_id DESC`, cluster, topic, key)
	if err != nil {
		return fmt.Errorf("exporting key history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Key, &m.Body, &m.Headers, &m.Partition, &m.Offset, &m.Timestamp); err != nil {
			return fmt.Errorf("scanning message: %w", err)
		}
		m.ID = m.Offset
		if err := fn(m); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) GetKeyHistory(ctx context.Context, cluster, topic, key string, page, limit int) ([]Message, int, error) {
	pgOffset := (page - 1) * limit

	query := `
		WITH t AS (
			SELECT id FROM topics WHERE cluster = $1 AND name = $2
		)
		SELECT key, body, COALESCE(headers, '[]'::jsonb), partition, offset_id, timestamp,
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
		if err := rows.Scan(&m.Key, &m.Body, &m.Headers, &m.Partition, &m.Offset, &m.Timestamp, &total); err != nil {
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

type TimelinePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Offset    int64     `json:"offset"`
	Partition int32     `json:"partition"`
	BodySize  int       `json:"body_size"`
}

func (s *Store) GetKeyTimeline(ctx context.Context, cluster, topic, key string) ([]TimelinePoint, error) {
	query := `
		WITH t AS (
			SELECT id FROM topics WHERE cluster = $1 AND name = $2
		)
		SELECT timestamp, offset_id, partition, octet_length(body::text)
		FROM messages
		WHERE topic_id = (SELECT id FROM t) AND key = $3
		ORDER BY timestamp ASC`

	rows, err := s.pool.Query(ctx, query, cluster, topic, key)
	if err != nil {
		return nil, fmt.Errorf("querying key timeline: %w", err)
	}
	defer rows.Close()

	var points []TimelinePoint
	for rows.Next() {
		var p TimelinePoint
		if err := rows.Scan(&p.Timestamp, &p.Offset, &p.Partition, &p.BodySize); err != nil {
			return nil, fmt.Errorf("scanning timeline point: %w", err)
		}
		points = append(points, p)
	}
	if points == nil {
		points = []TimelinePoint{}
	}
	return points, rows.Err()
}

func sanitizeSortDir(dir string) string {
	if dir == "asc" {
		return "ASC"
	}
	return "DESC"
}
