package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/terminal-gateway/application/command"
	"github.com/juantevez/go-posnet/context/terminal-gateway/application/query"
	"github.com/juantevez/go-posnet/context/terminal-gateway/config"
	grpcserver "github.com/juantevez/go-posnet/context/terminal-gateway/infrastructure/grpc/server"
	httpinfra "github.com/juantevez/go-posnet/context/terminal-gateway/infrastructure/http"
	natsinfra "github.com/juantevez/go-posnet/context/terminal-gateway/infrastructure/nats"
	pginfra "github.com/juantevez/go-posnet/context/terminal-gateway/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// app agrupa todos los componentes del servicio y sus recursos abiertos.
type app struct {
	pool       *pgxpool.Pool
	natsConn   *natsutil.Conn
	subscriber *natsinfra.Subscriber
	grpcSrv    *grpcserver.TerminalGatewayServer
	httpSrv    *http.Server
	// wsSrv   *websocket.Server  ← agregar cuando se implemente infrastructure/websocket/
}

// wire construye el grafo de dependencias completo del BC Terminal Gateway.
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
	sessionRepo := pginfra.NewPaymentSessionRepo(pool)
	terminalRepo := pginfra.NewTerminalRepo(pool)
	idempotency := natsutil.NewIdempotencyStore(pool, "terminal_gateway")
	natsPub := natsutil.NewPublisher(js)
	eventPub := natsinfra.NewEventPublisher(natsPub)

	// TerminalNotifier — implementado por el adaptador WebSocket.
	// TODO: instanciar websocket.NewNotifier(wsSrv) cuando esté implementado.
	// Por ahora nil; reemplazar en la siguiente iteración.
	var notifier interface{} = nil
	_ = notifier

	// ── Aplicación ─────────────────────────────────────────────────────────────
	sessionHandler := command.NewSessionHandler(
		sessionRepo,
		terminalRepo,
		nil, // notifier — reemplazar con instancia real
		eventPub,
		idempotency,
		pool,
	)
	queryHandler := query.NewSessionQueryHandler(sessionRepo)

	// ── Adaptadores de entrada ─────────────────────────────────────────────────

	// NATS Subscriber — consume AuthorizationApproved/Rejected y notifica al terminal
	subscriber := natsinfra.NewSubscriber(js, sessionHandler)

	// gRPC Server — recibe SendReceipt desde el BC Notification
	grpcSrv := grpcserver.NewTerminalGatewayServer(nil, queryHandler) // notifier = nil hasta websocket/

	// HTTP Server — healthz, readyz, metrics, operaciones REST
	router := httpinfra.NewRouter(sessionHandler, queryHandler, pool)
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// WebSocket Server — conexiones mTLS de los terminales POSNET
	// TODO: wsSrv := websocket.NewServer(cfg.TLS, sessionHandler, terminalRepo)
	// Se arrancará en cfg.WSPort en app.start()

	return &app{
		pool:       pool,
		natsConn:   natsConn,
		subscriber: subscriber,
		grpcSrv:    grpcSrv,
		httpSrv:    httpSrv,
	}, nil
}

// start arranca todos los servidores en goroutines independientes.
func (a *app) start(ctx context.Context) error {
	// NATS consumers
	if err := a.subscriber.Subscribe(); err != nil {
		return fmt.Errorf("start: subscribe NATS consumers: %w", err)
	}
	slog.Info("NATS consumers active")

	// gRPC server
	go func() {
		if err := grpcserver.Start(a.grpcSrv, 9091); err != nil {
			slog.Error("gRPC server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("gRPC server starting", slog.Int("port", 9091))

	// HTTP server
	go func() {
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server stopped", slog.String("error", err.Error()))
		}
	}()
	slog.Info("HTTP server starting", slog.String("addr", a.httpSrv.Addr))

	// WebSocket server — en puerto separado con mTLS
	// TODO: go func() { wsSrv.ListenAndServeTLS(cfg.WSPort, cfg.TLS) }()

	// Job de limpieza de sesiones expiradas
	go a.runExpiredSessionsCleaner(ctx)
	slog.Info("expired sessions cleaner active")

	return nil
}

// runExpiredSessionsCleaner es un job periódico que elimina sesiones vencidas.
// Corre en background y se detiene cuando el contexto es cancelado.
func (a *app) runExpiredSessionsCleaner(ctx context.Context) {
	// La frecuencia viene de config.Session.ExpiredCleanupEvery
	// Usando 1 minuto como valor por defecto aquí para simplificar.
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	repo := pginfra.NewPaymentSessionRepo(a.pool)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := repo.DeleteExpired(ctx)
			if err != nil {
				slog.Error("expired sessions cleanup failed", slog.String("error", err.Error()))
				continue
			}
			if deleted > 0 {
				slog.Info("expired sessions cleaned", slog.Int64("count", deleted))
			}
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
