# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-04-30

### Added

- Key diff timeline visualisation on the history page showing change frequency and body size over time
- `GET /api/topics/{topic}/timeline` endpoint returning lightweight version metadata
- SVG timeline with colour-coded bars (blue→violet→amber by body size), hover tooltips and click-to-navigate
- Automatic time bucketing for keys with 200+ versions

### Changed

- Dropped redundant `idx_messages_topic_id_key` index (2 GB freed)
- Replaced messages `BIGSERIAL` primary key with composite `(topic_id, partition, offset_id)` (1.1 GB freed)
- `GetMaxOffsets` now reads from the keys table instead of scanning the messages table
- `GetKeyHistory` collapsed from 3 round trips to a single CTE query
- `ListKeys` uses `COUNT(*) OVER()` window function to eliminate separate count query when filters are active
- `ANALYZE` runs automatically after bulk deletes in `PurgeTopicData`

## [0.3.0] - 2026-04-30

### Added

- Key value search: search message values using `field:value` syntax in the key browser
- Field autocomplete dropdown suggesting known JSON fields per topic
- Filter chips for active value filters with one-click removal
- `GET /api/topics/{topic}/fields` endpoint returning top-level JSON field names
- `value_filter` query parameter on the keys endpoint for JSONB containment queries
- GIN index (`jsonb_path_ops`) on the keys table body column for fast value search
- Latest message body stored in the keys table and updated on each consumer flush

### Fixed

- Avro union unwrapping for `array`, `map`, `enum` and `fixed` types (previously rendered as `{"array": [...]}` instead of `[...]`)

### Changed

- Updated Go dependencies (confluent-kafka-go v2.14.1, pgx v5.9.2)
- Updated frontend dependencies (ESLint 10, diff 9, React 19.2.5, Vitest 4.1.5)
- Migrated Vitest config to separate `vitest.config.ts` (Vitest 4 compatibility)

## [0.2.0] - 2026-04-30

### Added

- Helm chart for Kubernetes deployment with bundled PostgreSQL (Bitnami subchart)
- Support for external PostgreSQL databases via `externalDatabase` values
- Optional Ingress resource gated by `ingress.enabled`
- Kubernetes deployment instructions in README

## [0.1.1] - 2026-04-30

### Fixed

- Consumer group lag calculation for uncommitted partitions
- Lag label display
- Test failures after ListKeys optimisation

### Changed

- Use index-only scan when counting keys (performance)
- Increase PostgreSQL `max_wal_size` to 4 GB to reduce checkpoint frequency
- Skip `npm install` if `node_modules` already exists
- Improve Docker dev setup reliability
- Clean non-UTF-8 bytes when consuming messages
- Improve structured logging

## [0.1.0] - 2026-04-28

### Added

- Multi-cluster Kafka support with manual partition assignment
- Key browser with search, partition filter and offset filter
- Message history with syntax-highlighted JSON
- Consumer group monitoring with per-partition lag
- Offset management (reset to earliest, latest or specific values)
- Avro deserialisation via Schema Registry (Confluent-compatible)
- Topic lifecycle management (discovery, stale cleanup and recreation detection)
- Re-consume support (wipe and re-consume from the beginning)
- PostgreSQL storage with idempotent writes
- REST API with OpenAPI 3.0 specification
- React 19 frontend with Vite, Tailwind and TanStack Query
- Docker Compose development stack
- CI pipeline (Go tests, TypeScript checks and Vitest)
- Release pipeline (GHCR image publish on tag push)

[0.4.0]: https://github.com/alfonsojimenez/kgazer/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/alfonsojimenez/kgazer/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/alfonsojimenez/kgazer/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/alfonsojimenez/kgazer/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/alfonsojimenez/kgazer/releases/tag/v0.1.0
