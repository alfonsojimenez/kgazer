<p align="center">
  <img src="docs/logo.png" alt="KGazer" width="180" />
</p>

<p align="center"><strong>Kafka Compacted Topic Explorer</strong></p>

KGazer is a developer tool for exploring and inspecting [Kafka compacted topics](https://kafka.apache.org/documentation/#compaction). It continuously consumes messages from your Kafka clusters, stores them in PostgreSQL and provides a web interface to browse keys, view message history and compare changes over time.

If you've ever needed to answer "what's the current value for this key?" or "what changed in this key's history?", KGazer gives you that visibility without writing throwaway consumer scripts.

## Features

- **Multi-cluster support** — Connect to multiple Kafka clusters simultaneously
- **Key browser** — Search, filter and paginate through all keys in a compacted topic
- **Message history** — View every version of a key with syntax-highlighted JSON and inline diffs
- **Consumer group monitoring** — See which consumer groups are reading a topic, per-partition lag and consumer assignments
- **Offset management** — Reset consumer group offsets to earliest, latest or specific per-partition values
- **Avro support** — Automatic deserialisation via Schema Registry (Confluent-compatible)
- **Topic lifecycle** — Detects new topics, cleans up deleted topics and handles topic recreation transparently
- **Re-consume** — Wipe stored data and re-consume a topic from the beginning with one click

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)

### 1. Configure your clusters

```bash
cp config.yml.example config.yml
```

Edit `config.yml` with your Kafka cluster details:

```yaml
kafka:
  clusters:
    - name: my-cluster
      bootstrapServers: localhost:9092

    - name: production
      bootstrapServers: kafka.example.com:9092
      properties:
        security.protocol: SASL_SSL
        sasl.mechanism: PLAIN
        sasl.jaas.config: >-
          org.apache.kafka.common.security.plain.PlainLoginModule required
          username="user" password="pass";
      schemaRegistry: https://schema-registry.example.com
      schemaRegistryAuth:
        username: sr-user
        password: sr-pass

kgazer:
  server:
    port: 8080
  db:
    host: localhost
    port: 5432
    name: kgazer
    user: kgazer
    password: kgazer
    sslmode: disable
  compactedOnly: true
```

Cluster names must be URL-safe: letters, digits, hyphens, dots and underscores only.

### 2. Start the stack

```bash
docker compose up
```

This starts three services:

| Service | Default Port | Description |
|---------|-------------|-------------|
| **frontend** | [localhost:4200](http://localhost:4200) | React web interface with hot reload |
| **backend** | localhost:1899 | Go API server |
| **db** | localhost:5899 | PostgreSQL 16 |

Open [http://localhost:4200](http://localhost:4200) and KGazer will begin discovering and consuming topics automatically.

Ports are configurable via environment variables:

```bash
KGAZER_PORT_UI=3000 KGAZER_PORT_API=9090 KGAZER_PORT_DB=5433 docker compose up
```

### 3. Run the tests

```bash
docker compose --profile test run --rm test          # Backend (Go)
docker compose --profile test run --rm test-frontend  # Frontend (Vitest)
```

## Configuration Reference

| Field | Description | Default |
|-------|-------------|---------|
| `kafka.clusters[].name` | Cluster display name (URL-safe) | required |
| `kafka.clusters[].bootstrapServers` | Kafka broker addresses | required |
| `kafka.clusters[].properties` | librdkafka properties (SASL, SSL, etc.) | `{}` |
| `kafka.clusters[].schemaRegistry` | Schema Registry URL for Avro deserialisation | — |
| `kafka.clusters[].schemaRegistryAuth` | Schema Registry credentials (`username`, `password`) | — |
| `kgazer.server.port` | API server port | `8080` |
| `kgazer.db.*` | PostgreSQL connection details | — |
| `kgazer.compactedOnly` | Only consume topics with `cleanup.policy=compact` | `true` |

Environment variable overrides:

| Variable | Overrides |
|----------|-----------|
| `KGAZER_DB_HOST` | `kgazer.db.host` |
| `KGAZER_DB_PASSWORD` | `kgazer.db.password` |
| `KGAZER_KAFKA_BOOTSTRAP_SERVERS` | All clusters' `bootstrapServers` |

## How It Works

KGazer has four main components that work together:

```
┌─────────────────────────────────────────────────────┐
│                    Frontend (React)                  │
│                  localhost:4200                       │
└──────────────────────┬──────────────────────────────┘
                       │ REST API
┌──────────────────────▼──────────────────────────────┐
│                   Backend (Go)                       │
│                                                      │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │  Syncer   │  │ Consumer │  │    API Server     │  │
│  │ (per      │  │ (per     │  │  (chi router)     │  │
│  │  cluster) │  │  cluster) │  │                   │  │
│  └─────┬─────┘  └─────┬────┘  └────────┬──────────┘  │
│        │              │               │              │
│        ▼              ▼               ▼              │
│  ┌─────────────────────────────────────────────────┐ │
│  │              PostgreSQL                          │ │
│  │  topics │ messages │ keys                        │ │
│  └─────────────────────────────────────────────────┘ │
└──────────────────────┬──────────────────────────────┘
                       │
              ┌────────▼────────┐
              │  Kafka Clusters  │
              └─────────────────┘
```

### Syncer

One goroutine per cluster, polling Kafka metadata every 60 seconds. On each tick it:

1. Calls `GetMetadata` to discover all topics in the cluster
2. Filters to compacted topics only (when `compactedOnly` is enabled)
3. Calls `DescribeTopics` to get each topic's internal UUID
4. Upserts topics into PostgreSQL — new topics are inserted, existing ones are updated
5. **Stale topic cleanup** — topics present in the database but missing from metadata are removed (cascade: messages, keys, topic row)
6. **Recreation detection** — if a topic's UUID changed since the last sync, its data is purged and consumption restarts from the beginning

### Consumer

One `kafka.Consumer` per cluster using **manual partition assignment** (no consumer groups, no rebalancing). It:

1. Waits for the initial sync to complete
2. Queries the database for all known topics and their stored offsets
3. Assigns all partitions across all topics, resuming from `stored_offset + 1` (or `OffsetBeginning` for new topics)
4. Polls in a tight loop, batching messages (500 per batch or every 500ms)
5. Writes batches to PostgreSQL: `messages` table (with `ON CONFLICT DO NOTHING`), `keys` table (upsert with latest offset/timestamp), and incremental topic stats
6. **Dynamic topic discovery** — every sync interval, compares the database topic list with its in-memory assignment and adds/removes partitions accordingly
7. **Re-consume support** — checks for pending re-consume requests each poll cycle (~100ms) and resets partitions to `OffsetBeginning` when triggered

Messages are deserialised in this order:

1. If Schema Registry is configured and the message has the Confluent wire format (`0x00` magic byte) → **Avro**
2. If the bytes are valid JSON → **JSON**
3. Otherwise → base64-encoded as `{"_binary": "..."}`
4. Null values (tombstones) → stored as JSON `null` with format `tombstone`

### API Server

A [chi](https://github.com/go-chi/chi) HTTP router exposing a REST API. Key endpoint groups:

| Group | Endpoints | Description |
|-------|-----------|-------------|
| **Topics** | `GET /api/topics`, `GET /api/topics/{topic}` | List and detail with consumption progress |
| **Keys** | `GET /api/topics/{topic}/keys` | Paginated key listing with search, partition filter, and offset filter |
| **History** | `GET /api/topics/{topic}/keys/{key}/history` | Message versions for a key (most recent first) |
| **Consumer Groups** | `GET /api/topics/{topic}/consumer-groups`, `GET /{group}` | Live consumer groups with per-partition lag |
| **Offset Reset** | `POST /api/topics/{topic}/consumer-groups/{group}/reset-offsets` | Reset offsets to earliest, latest, or specific values |
| **Settings** | `GET /api/settings/info`, `GET /api/settings/orphaned-clusters`, `DELETE /api/settings/clusters/{cluster}` | Runtime stats, sanitised config, orphaned cluster management |

Full API documentation is available in [`backend/openapi.yaml`](backend/openapi.yaml) (OpenAPI 3.0).

### Progress Tracking

Consumption progress is tracked in-memory by a dedicated component that syncs Kafka watermarks (low/high offsets) every 30 seconds. Progress percentage is calculated as:

```
progress = (consumed_offset - low_watermark) / (high_watermark - low_watermark) × 100
```

A topic is marked as "done" when progress reaches 99.5% or when no new messages have been seen for 30 seconds (indicating the consumer has caught up).

### Database Schema

Three tables, defined in [`backend/migrations/`](backend/migrations/):

**`topics`** — one row per cluster+topic combination. Stores partition count, compacted flag, message format, Kafka topic UUID, and cached stats (message count, key count, last message timestamp).

**`messages`** — every consumed message. Keyed by `(topic_id, partition, offset_id)` with `ON CONFLICT DO NOTHING` for idempotent writes. Stores the deserialised body as JSONB.

**`keys`** — one row per unique key per topic. Tracks the latest partition, offset, and timestamp. Used for the key browser with indexes for both offset-based and time-based sorting.

## Project Structure

```
kgazer/
├── .github/workflows/      # CI + Release pipelines
├── backend/
│   ├── cmd/server/         # Entry point
│   ├── internal/
│   │   ├── api/            # HTTP handlers (chi router)
│   │   ├── config/         # YAML config loader with validation
│   │   ├── consumer/       # Kafka consumer with dynamic topic assignment
│   │   ├── decoder/        # Avro deserialisation via Schema Registry
│   │   ├── progress/       # Consumption progress tracking
│   │   ├── status/         # Cluster connection status
│   │   ├── store/          # PostgreSQL data access layer
│   │   └── syncer/         # Topic discovery and lifecycle management
│   ├── migrations/         # PostgreSQL schema (3 files)
│   └── openapi.yaml        # API specification (OpenAPI 3.0)
├── frontend/
│   ├── src/
│   │   ├── components/     # Shared UI (JsonViewer, MessageDiff, Sidebar, etc.)
│   │   ├── lib/            # API client and utilities
│   │   ├── pages/          # Route pages (Topics, Keys, History, ConsumerGroups, Settings)
│   │   └── test/           # Test setup
│   └── public/             # Static assets (logo)
├── docs/                   # Documentation assets
├── config.yml.example      # Configuration template
├── docker-compose.yml      # Development stack
├── VERSION                 # SemVer version source of truth
└── LICENCE                 # MIT
```

## URL Structure

KGazer uses path-based cluster routing:

```
/clusters/:cluster                                → Topic list
/clusters/:cluster/topics/:topic/keys             → Key browser
/clusters/:cluster/topics/:topic/keys/:key        → Message history + diffs
/clusters/:cluster/topics/:topic/consumer-groups  → Consumer group lag
/settings                                         → Runtime info, config, orphaned clusters
```

## Versioning

KGazer follows [Semantic Versioning](https://semver.org/). The version is defined in the `VERSION` file at the repository root and injected into the backend binary at build time via `-ldflags`.

To create a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the GitHub Actions release workflow, which builds and publishes a Docker image to `ghcr.io/alfonsojimenez/kgazer:0.1.0`.

## Development

### Running locally (without Docker)

**Backend:**

```bash
cd backend
go run ./cmd/server
```

Requires Go 1.25+, librdkafka, and a running PostgreSQL instance.

**Frontend:**

```bash
cd frontend
npm install
npm run dev
```

The Vite dev server proxies `/api` requests to `http://localhost:8080`. When using Docker Compose, the proxy targets the internal Docker network automatically.

### Building for production

```bash
cd backend
CGO_ENABLED=1 go build -tags musl -o kgazer ./cmd/server

cd frontend
npm run build  # outputs to dist/
```

## CI/CD

Two GitHub Actions workflows in `.github/workflows/`:

- **`ci.yml`** — Runs on push/PR to `main`. Backend: Go build + vet + tests (with Postgres service). Frontend: TypeScript check + Vitest.
- **`release.yml`** — Runs on tag push (`v*`). Builds the Docker image with the version baked in and publishes to GitHub Container Registry.

## Licence

MIT — see [LICENCE](LICENCE).

## Trademarks

Apache Kafka and Kafka are either registered trademarks or trademarks of the Apache Software Foundation in the United States and/or other countries. KGazer is not affiliated with, endorsed by, or sponsored by the Apache Software Foundation.
