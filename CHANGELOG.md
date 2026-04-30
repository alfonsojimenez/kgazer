# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.0]: https://github.com/alfonsojimenez/kgazer/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/alfonsojimenez/kgazer/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/alfonsojimenez/kgazer/releases/tag/v0.1.0
