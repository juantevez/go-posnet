package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/authorization/application/command"
	"github.com/juantevez/go-posnet/context/authorization/application/query"
	"github.com/juantevez/go-posnet/context/authorization/config"
	grpcserver "github.com/juantevez/go-posnet/context/authorization/infrastructure/grpc/server"
	httpinfra "github.com/juantevez/go-posnet/context/authorization/infrastructure/http"
	natsinfra "github.com/juantevez/go-posnet/context/authorization/infrastructure/nats"
	pginfra "github.com/juantevez/go-posnet/context/authorization/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/pgutil"
	natsclient "github.com/nats-io/nats.go"
)

// app agrupa todos los componentes del servicio y sus recursos abiertos.
type app struct {
	pool       *pgxpool.Pool    // tipo concreto de pgx — no pgutil.Pool
	natsConn   *natsclient.Conn // tipo concreto de nats.go — no natsutil.Conn
	subscriber *natsinfra.Subscriber
	grpcSrv    *grpcserver.AuthorizationServer
	httpSrv    *http.Server
}

// wire construye el grafo de dependencias completo del BC Authorization.
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
	if err := pgutil.RegisterPoolMetrics(pool); err != nil {
		return nil, fmt.Errorf("wire: register pgx pool metrics: %w", err)
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
		natsConn.Close()
		pool.Close()
		return nil, fmt.Errorf("wire: init JetStream: %w", err)
	}

	if err := natsutil.EnsureStreams(js); err != nil {
		natsConn.Close()
		pool.Close()
		return nil, fmt.Errorf("wire: ensure NATS streams: %w", err)
	}

	slog.Info("NATS ready — streams and consumers configured")

	// ── Infraestructura ────────────────────────────────────────────────────────
	txRepo := pginfra.NewTransactionRepo(pool)
	idempotency := natsutil.NewIdempotencyStore(pool, "pn_authorization")
	natsPub := natsutil.NewPublisher(js)
	eventPub := natsinfra.NewEventPublisher(natsPub)

	// ── Aplicación ─────────────────────────────────────────────────────────────
	acquirerGW := pginfra.NewMockAcquirerGateway()
	authHandler := command.NewAuthorizationHandler(
		txRepo,
		acquirerGW, // acquirerGW — reemplazar con instancia real cuando esté implementado
		eventPub,
		idempotency,
		pool,
	)
	authMetrics, err := command.NewMetrics()
	if err != nil {
		return nil, fmt.Errorf("init authorization metrics: %w", err)
	}
	authHandler.SetMetrics(authMetrics)
	queryHandler := query.NewTransactionQueryHandler(txRepo)

	// ── Adaptadores de entrada ─────────────────────────────────────────────────
	subscriber := natsinfra.NewSubscriber(js, authHandler)
	grpcSrv := grpcserver.NewAuthorizationServer(queryHandler)

	router := httpinfra.NewRouter(queryHandler, pool)
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
// func (a *app) start(ctx context.Context) error {
func (a *app) start(_ context.Context) error {
	if err := a.subscriber.Subscribe(); err != nil {
		return fmt.Errorf("start: subscribe NATS consumers: %w", err)
	}
	slog.Info("NATS consumers active")

	go func() {
		if err := grpcserver.Start(a.grpcSrv, 9090); err != nil {
			slog.Error("gRPC server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("gRPC server starting", slog.Int("port", 9090))

	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("HTTP server starting", slog.String("addr", a.httpSrv.Addr))

	return nil
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
