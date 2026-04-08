FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev librdkafka-dev pkgconf

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 go build -tags musl -ldflags "-X main.version=${VERSION}" -o server ./cmd/server

FROM alpine:3.20

ARG VERSION=dev
LABEL org.opencontainers.image.title="KGazer" \
      org.opencontainers.image.description="Kafka Compacted Topic Explorer" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/alfonsojimenez/kgazer"

RUN apk add --no-cache librdkafka ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./server"]
