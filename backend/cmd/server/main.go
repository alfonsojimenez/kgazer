package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/alfonsojimenez/kgazer/backend/internal/api"
	"github.com/alfonsojimenez/kgazer/backend/internal/config"
	"github.com/alfonsojimenez/kgazer/backend/internal/consumer"
	"github.com/alfonsojimenez/kgazer/backend/internal/progress"
	"github.com/alfonsojimenez/kgazer/backend/internal/status"
	"github.com/alfonsojimenez/kgazer/backend/internal/store"
	"github.com/alfonsojimenez/kgazer/backend/internal/syncer"
)

var version = "dev"

func main() {
	slog.Info("starting kgazer", "version", version)
	configPath := os.Getenv("KGAZER_CONFIG")
	if configPath == "" {
		configPath = "config.yml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	dsn := cfg.KGazer.DB.DSN()

	if err := runMigrations(dsn); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s, err := store.New(ctx, dsn)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer s.Close()

	tracker := status.NewTracker()
	pt := progress.NewTracker()

	admins, err := api.NewAdminClients(cfg.Kafka.Clusters)
	if err != nil {
		slog.Error("failed to create kafka admin clients", "error", err)
		os.Exit(1)
	}
	defer admins.Close()

	syncInterval := 60 * time.Second
	for _, cluster := range cfg.Kafka.Clusters {
		syncer.Start(ctx, cluster, s, tracker, cfg.KGazer.IsCompactedOnly(), syncInterval)
		pt.StartWatermarkSync(ctx, cluster, 30*time.Second)
		go func() {
			if err := consumer.Start(ctx, cluster, s, tracker, pt, syncInterval); err != nil {
				slog.Error("consumer stopped", "error", err, "cluster", cluster.Name)
			}
		}()
	}

	startedAt := time.Now()
	router := api.NewRouter(s, tracker, pt, admins, version, startedAt, cfg)
	addr := fmt.Sprintf(":%d", cfg.KGazer.Server.Port)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
	}

	slog.Info("server stopped")
}

func runMigrations(dsn string) error {
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("migrations complete")
	return nil
}
