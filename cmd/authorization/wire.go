package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/tu-org/posnet-backend/context/authorization/application/command"
	"github.com/tu-org/posnet-backend/context/authorization/application/query"
	"github.com/tu-org/posnet-backend/context/authorization/config"
	grpcserver "github.com/tu-org/posnet-backend/context/authorization/infrastructure/grpc/server"
	httpinfra "github.com/tu-org/posnet-backend/context/authorization/infrastructure/http"
	natsinfra "github.com/tu-org/posnet-backend/context/authorization/infrastructure/nats"
	pginfra "github.com/tu-org/posnet-backend/context/authorization/infrastructure/postgres"
	"github.com/tu-org/posnet-backend/pkg/natsutil"
	"github.com/tu-org/posnet-backend/pkg/pgutil"
)

// app agrupa todos los componentes del servicio y sus recursos abiertos.
// La función close() libera todos los recursos en orden inverso al de creación.
type app struct {
	pool       *pgutil.Pool
	natsConn   *natsutil.Conn
	subscriber *natsinfra.Subscriber
	grpcSrv    *grpcserver.AuthorizationServer
	httpSrv    *http.Server
}

// wire construye el grafo de dependencias completo del BC Authorization.
// El orden importa: infraestructura → repositorios → servicios → handlers → adaptadores.
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
	if err := natsutil.EnsureConsumers(js); err != nil {
		return nil, fmt.Errorf("wire: ensure NATS consumers: %w", err)
	}
	slog.Info("NATS ready — streams and consumers configured")

	// ── Infraestructura ────────────────────────────────────────────────────────
	txRepo := pginfra.NewTransactionRepo(pool)
	idempotency := natsutil.NewIdempotencyStore(pool, "authorization")
	natsPub := natsutil.NewPublisher(js)
	eventPub := natsinfra.NewEventPublisher(natsPub)

	// AcquirerGateway — implementación ISO 8583 sobre TCP/TLS.
	// TODO: instanciar acquirer.NewISO8583Gateway(cfg.Acquirer) cuando esté implementado.
	// Por ahora se usa nil y se reemplaza en la siguiente iteración.
	var acquirerGW interface{} = nil
	_ = acquirerGW

	// ── Aplicación ─────────────────────────────────────────────────────────────
	authHandler := command.NewAuthorizationHandler(
		txRepo,
		nil, // acquirerGW — reemplazar con instancia real
		eventPub,
		idempotency,
		pool,
	)
	queryHandler := query.NewTransactionQueryHandler(txRepo)

	// ── Adaptadores de entrada ─────────────────────────────────────────────────

	// NATS Subscriber — consume eventos y delega a los command handlers
	subscriber := natsinfra.NewSubscriber(js, authHandler)

	// gRPC Server — consultas de operación (lado Q del CQRS)
	grpcSrv := grpcserver.NewAuthorizationServer(queryHandler)

	// HTTP Server — healthz, readyz, metrics, consultas REST
	router := httpinfra.NewRouter(queryHandler, pool)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: router,
	}

	return &app{
		pool:       pool,
		natsConn:   natsConn,
		subscriber: subscriber,
		grpcSrv:    grpcSrv,
		httpSrv:    httpSrv,
	}, nil
}

// start arranca todos los servidores del BC en goroutines independientes.
// Los errores fatales cancelan el contexto raíz vía log + os.Exit.
func (a *app) start(ctx context.Context) error {
	// NATS consumers — arranque síncrono, subscribe bloquea hasta error
	if err := a.subscriber.Subscribe(); err != nil {
		return fmt.Errorf("start: subscribe NATS consumers: %w", err)
	}
	slog.Info("NATS consumers active")

	// gRPC server — goroutine independiente
	go func() {
		if err := grpcserver.Start(a.grpcSrv, 9090); err != nil {
			slog.Error("gRPC server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("gRPC server starting", slog.Int("port", 9090))

	// HTTP server — goroutine independiente
	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("HTTP server starting", slog.String("addr", a.httpSrv.Addr))

	return nil
}

// close libera todos los recursos en orden inverso al de creación.
// Llamado mediante defer en main.run() para garantizar limpieza en shutdown.
func (a *app) close() {
	slog.Info("closing resources")

	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
		defer cancel()
		_ = a.httpSrv.Shutdown(ctx)
	}

	if a.natsConn != nil {
		a.natsConn.Drain() //nolint:errcheck — best effort en shutdown
		a.natsConn.Close()
	}

	if a.pool != nil {
		a.pool.Close()
	}

	slog.Info("all resources closed")
}
