//go:build integration

// Package main — integration tests del entrypoint del BC Authorization.
//
// Ejercitan el camino feliz completo del wiring (wire → start → close) contra
// infraestructura real (Postgres + NATS JetStream) levantada con Testcontainers.
// Complementan a los unit tests de wire_test.go / main_test.go, que solo cubren
// los caminos de error y la liberación de recursos.
//
// Requisitos: un daemon de Docker accesible.
// Ejecución:
//
//	go test -tags=integration -run Integration ./cmd/authorization/ -v
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/config"

	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres levanta un Postgres efímero con el schema del proyecto ya
// aplicado (01-init.sql) y devuelve su DSN.
func startPostgres(ctx context.Context, t *testing.T) string {
	t.Helper()

	initScript, err := filepath.Abs(
		"../../deployments/docker/docker-entrypoint-initdb.d/01-init.sql",
	)
	if err != nil {
		t.Fatalf("resolver init script: %v", err)
	}

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("posnet"),
		tcpostgres.WithUsername("posnet"),
		tcpostgres.WithPassword("posnet"),
		tcpostgres.WithInitScripts(initScript),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("arrancar contenedor postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("obtener DSN postgres: %v", err)
	}
	return dsn
}

// startNATS levanta un NATS con JetStream habilitado y devuelve su URL.
func startNATS(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := tcnats.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		t.Fatalf("arrancar contenedor nats: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	url, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("obtener URL nats: %v", err)
	}
	return url
}

// freePort reserva y libera un puerto TCP local, devolviendo su número para
// evitar colisiones en el servidor HTTP del app bajo test.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reservar puerto libre: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// getWithRetry hace GET a url reintentando hasta que responde o se agota el
// deadline (el servidor HTTP arranca de forma asíncrona en app.start).
func getWithRetry(t *testing.T, url string, timeout time.Duration) *http.Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx // test helper
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("GET %s no respondió dentro de %s: %v", url, timeout, lastErr)
	return nil
}

// TestIntegration_WireStartClose ejercita el ciclo de vida completo del app:
// wire construye el grafo de dependencias contra Postgres y NATS reales,
// start arranca los servidores, se verifican los probes HTTP y close libera
// todos los recursos de forma limpia.
func TestIntegration_WireStartClose(t *testing.T) {
	ctx := context.Background()

	dsn := startPostgres(ctx, t)
	natsURL := startNATS(ctx, t)
	httpPort := freePort(t)

	cfg := &config.Config{
		GRPCPort: 9090,
		HTTPPort: httpPort,
		Postgres: config.PostgresConfig{
			DSN:             dsn,
			MaxConns:        5,
			MinConns:        1,
			MaxConnLifetime: 30 * time.Minute,
			MaxConnIdleTime: 5 * time.Minute,
			MigrationsDir:   "migrations/pn_authorization",
		},
		NATS: config.NATSConfig{
			URL:           natsURL,
			MaxReconnect:  -1,
			ReconnectWait: 2 * time.Second,
		},
	}

	// ── wire: construcción del grafo de dependencias ────────────────────────────
	app, err := wire(ctx, cfg)
	if err != nil {
		t.Fatalf("wire() error = %v, want nil", err)
	}
	if app.pool == nil || app.natsConn == nil || app.subscriber == nil ||
		app.grpcSrv == nil || app.httpSrv == nil {
		t.Fatalf("wire() dejó campos nil en app: %+v", app)
	}
	if !app.natsConn.IsConnected() {
		t.Error("natsConn no está conectado tras wire()")
	}

	// ── start: arranque de subscriber + servidores ──────────────────────────────
	if err := app.start(ctx); err != nil {
		app.close()
		t.Fatalf("start() error = %v, want nil", err)
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	// /healthz — liveness (estático, confirma que el servidor HTTP está arriba).
	resp := getWithRetry(t, base+"/healthz", 5*time.Second)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status = %d, want 200", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// /readyz — readiness: hace un ping real a Postgres a través del pool wireado.
	resp = getWithRetry(t, base+"/readyz", 5*time.Second)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /readyz status = %d, want 200 (Postgres debería estar disponible)", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// ── close: shutdown graceful ─────────────────────────────────────────────────
	app.close()

	// Tras close el servidor HTTP ya no debe aceptar conexiones.
	if _, err := http.Get(base + "/healthz"); err == nil { //nolint:noctx // test
		t.Error("el servidor HTTP sigue respondiendo tras close()")
	}
	if app.natsConn.IsConnected() {
		t.Error("natsConn sigue conectado tras close()")
	}
}
