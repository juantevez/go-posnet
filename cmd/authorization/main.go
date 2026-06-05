// Package main es el entrypoint del BC Authorization.
// Su única responsabilidad es orquestar el arranque y el shutdown graceful.
// Todo el wiring de dependencias está en wire.go.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tu-org/posnet-backend/context/authorization/config"
	"github.com/tu-org/posnet-backend/pkg/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	// ── 1. Configuración ───────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── 2. Logger ──────────────────────────────────────────────────────────────
	observability.InitLogger(cfg.OTEL.Environment, slog.LevelInfo)
	log := slog.Default().With(slog.String("service", cfg.OTEL.ServiceName))

	// ── 3. Contexto raíz con cancelación ──────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── 4. Tracer OpenTelemetry ────────────────────────────────────────────────
	shutdownTracer, err := observability.InitTracer(ctx, observability.TracerConfig{
		ServiceName:    cfg.OTEL.ServiceName,
		ServiceVersion: cfg.OTEL.ServiceVersion,
		OTLPEndpoint:   cfg.OTEL.OTLPEndpoint,
		Environment:    cfg.OTEL.Environment,
	})
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	// ── 5. Meter Prometheus ────────────────────────────────────────────────────
	shutdownMeter, err := observability.InitMeter(ctx, cfg.OTEL.ServiceName)
	if err != nil {
		return fmt.Errorf("init meter: %w", err)
	}
	defer func() { _ = shutdownMeter(ctx) }()

	log.Info("starting authorization BC",
		slog.String("version", cfg.OTEL.ServiceVersion),
		slog.String("env", cfg.OTEL.Environment),
	)

	// ── 6. Wiring de dependencias ──────────────────────────────────────────────
	app, err := wire(ctx, cfg)
	if err != nil {
		return fmt.Errorf("wire dependencies: %w", err)
	}
	defer app.close()

	// ── 7. Arrancar servicios ──────────────────────────────────────────────────
	if err := app.start(ctx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	log.Info("authorization BC ready — waiting for messages")

	// ── 8. Graceful shutdown ───────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutdown signal received — stopping services")
	cancel()

	log.Info("authorization BC stopped cleanly")
	return nil
}
