package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/settlement/application/command"
	"github.com/juantevez/go-posnet/context/settlement/application/query"
	"github.com/juantevez/go-posnet/context/settlement/config"
	"github.com/juantevez/go-posnet/context/settlement/domain/service"
	grpcserver "github.com/juantevez/go-posnet/context/settlement/infrastructure/grpc/server"
	httpinfra "github.com/juantevez/go-posnet/context/settlement/infrastructure/http"
	natsinfra "github.com/juantevez/go-posnet/context/settlement/infrastructure/nats"
	pginfra "github.com/juantevez/go-posnet/context/settlement/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/pgutil"
	natsclient "github.com/nats-io/nats.go"
)

// app agrupa todos los componentes del servicio y sus recursos abiertos.
type app struct {
	pool       *pgxpool.Pool
	natsConn   *natsclient.Conn
	subscriber *natsinfra.Subscriber
	grpcSrv    *grpcserver.SettlementServer
	httpSrv    *http.Server
}

// wire construye el grafo de dependencias completo del BC Settlement.
func wire(ctx context.Context, cfg *config.Config) (*app, error) {

	// ── PostgreSQL ─────────────────────────────────────────────────────────────
	pool, err := pgutil.NewPool(ctx, pgutil.Config{
		DSN:             cfg.Postgres.DSN,
		MaxConns:        cfg.Postgres.MaxConns,
		MinConns:        cfg.Postgres.MinConns,
		MaxConnLifetime: cfg.Postgres.MaxConnLifetime,
		MaxConnIdleTime: cfg.Postgres.MaxConnIdleTime,
	})
	if err != nil {
		return nil, fmt.Errorf("wire: init postgres pool: %w", err)
	}

	if err := pgutil.Migrate(ctx, pool, cfg.Postgres.MigrationsDir); err != nil {
		pool.Close()
		return nil, fmt.Errorf("wire: run migrations: %w", err)
	}
	slog.Info("postgres ready — migrations applied")

	// ── NATS JetStream ─────────────────────────────────────────────────────────
	natsConn, err := natsutil.Connect(natsutil.Config{
		URL:           cfg.NATS.URL,
		NKeyPath:      cfg.NATS.NKeyPath,
		TLSCertPath:   cfg.NATS.TLSCertPath,
		TLSKeyPath:    cfg.NATS.TLSKeyPath,
		TLSCAPath:     cfg.NATS.TLSCAPath,
		MaxReconnect:  cfg.NATS.MaxReconnect,
		ReconnectWait: cfg.NATS.ReconnectWait,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("wire: connect to NATS: %w", err)
	}

	js, err := natsutil.JetStream(natsConn)
	if err != nil {
		return nil, fmt.Errorf("wire: init JetStream: %w", err)
	}

	if err := natsutil.EnsureStreams(js); err != nil {
		return nil, fmt.Errorf("wire: ensure NATS streams: %w", err)
	}
	slog.Info("NATS ready — streams and consumers configured")

	// ── Infraestructura ────────────────────────────────────────────────────────
	batchRepo := pginfra.NewSettlementBatchRepo(pool)
	idempotency := natsutil.NewIdempotencyStore(pool, "settlement")
	natsPub := natsutil.NewPublisher(js)
	eventPub := natsinfra.NewEventPublisher(natsPub)

	// SettlementProcessor — adaptador hacia el procesador externo (Visa/MC).
	// TODO: instanciar processor.NewISO8583Processor(cfg.Settlement) cuando esté implementado.
	// Por ahora nil — submitBatch (BatchHandler) y ResubmitBatch (AdminHandler) quedan como no-op.
	var processor service.SettlementProcessor

	// ── Aplicación ─────────────────────────────────────────────────────────────
	batchHandler := command.NewBatchHandler(
		batchRepo,
		eventPub,
		processor,
		idempotency,
		pool,
		cfg.Settlement.MDRPercent,
	)
	settlementMetrics, err := command.NewMetrics()
	if err != nil {
		return nil, fmt.Errorf("init settlement metrics: %w", err)
	}
	batchHandler.SetMetrics(settlementMetrics)
	adminHandler := command.NewAdminHandler(batchRepo, processor)
	queryHandler := query.NewBatchQueryHandler(batchRepo)

	// ── Adaptadores de entrada ─────────────────────────────────────────────────

	// NATS Subscriber — consume AuthApproved, ReversalCompleted, BatchCloseRequested
	subscriber := natsinfra.NewSubscriber(js, batchHandler)

	// gRPC Server — placeholder
	grpcSrv := grpcserver.NewSettlementServer()

	// HTTP Server — healthz, readyz, metrics, gestión de batches (operaciones)
	router := httpinfra.NewRouter(queryHandler, adminHandler, pool)
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &app{
		pool:       pool,
		natsConn:   natsConn,
		subscriber: subscriber,
		grpcSrv:    grpcSrv,
		httpSrv:    httpSrv,
	}, nil
}

// start arranca todos los servidores en goroutines independientes.
// Incluye el job de cierre automático de lotes si BatchCloseHour > 0.
func (a *app) start(ctx context.Context) error {
	// NATS consumers
	if err := a.subscriber.Subscribe(); err != nil {
		return fmt.Errorf("start: subscribe NATS consumers: %w", err)
	}
	slog.Info("NATS consumers active",
		slog.String("consumers", "settlement-auth, settlement-reversal, settlement-batch"),
	)

	// gRPC server
	go func() {
		if err := grpcserver.Start(a.grpcSrv, 9093); err != nil {
			slog.Error("gRPC server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("gRPC server starting", slog.Int("port", 9093))

	// HTTP server
	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("HTTP server starting", slog.String("addr", a.httpSrv.Addr))

	// Job de cierre automático de lotes — corre a BatchCloseHour UTC cada día
	go a.runBatchCloser(ctx)
	slog.Info("batch closer job active")

	return nil
}

// runBatchCloser es el job periódico que inicia el cierre de lotes a la hora configurada.
// Revisa cada minuto si llegó la hora de cierre y no se procesó aún.
func (a *app) runBatchCloser(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastClosedDate string

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			utc := now.UTC()
			today := utc.Format("2006-01-02")

			// Solo actuar a la hora de cierre configurada y una vez por día
			if utc.Hour() != 23 || lastClosedDate == today {
				continue
			}

			slog.Info("batch closer job triggered", slog.String("date", today))
			lastClosedDate = today

			// El cierre real ocurre cuando el terminal envía BatchCloseRequested.
			// Este job es solo un log/alerta para detectar terminales que no cerraron.
			// La implementación completa consultaría los batches OPEN del día
			// y generaría alertas para los que no se cerraron manualmente.
		}
	}
}

// close libera todos los recursos en orden inverso al de creación.
func (a *app) close() {
	slog.Info("closing resources")

	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.httpSrv.Shutdown(ctx)
	}

	if a.natsConn != nil {
		_ = a.natsConn.Drain()
		a.natsConn.Close()
	}

	if a.pool != nil {
		a.pool.Close()
	}

	slog.Info("all resources closed")
}
