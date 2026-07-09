package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/config"
)

// baseConfig devuelve una configuración mínima válida en estructura, apuntando
// a recursos inexistentes. Sirve para ejercitar los caminos de error de wire
// sin depender de infraestructura real (Postgres / NATS).
func baseConfig() *config.Config {
	return &config.Config{
		GRPCPort: 9090,
		HTTPPort: 8080,
		Postgres: config.PostgresConfig{
			DSN:      "host=localhost port=notanumber", // DSN inválido → ParseConfig falla
			MaxConns: 5,
			MinConns: 1,
		},
		NATS: config.NATSConfig{URL: "nats://127.0.0.1:1"},
	}
}

// wire debe fallar en el primer paso (pool de Postgres) cuando el DSN es
// inválido, envolviendo el error con el prefijo esperado y sin devolver app.
func TestWire_PostgresPoolError(t *testing.T) {
	app, err := wire(context.Background(), baseConfig())
	if err == nil {
		if app != nil {
			app.close()
		}
		t.Fatal("wire() error = nil, want error por DSN inválido")
	}
	if app != nil {
		t.Errorf("wire() app = %v, want nil en caso de error", app)
	}
	if !strings.Contains(err.Error(), "wire: init postgres pool") {
		t.Errorf("wire() error = %q, want que contenga %q", err.Error(), "wire: init postgres pool")
	}
}

// close sobre un app con todos los campos nil no debe entrar en panic.
func TestApp_Close_NilFields(t *testing.T) {
	a := &app{}
	a.close() // no debe panicar
}

// close debe apagar el servidor HTTP: tras cerrarlo, el listener deja de aceptar
// conexiones. pool y natsConn quedan nil para no tocar infraestructura real.
func TestApp_Close_ShutsDownHTTPServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := &http.Server{Handler: http.NewServeMux()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	// Damos un instante a que el servidor arranque.
	time.Sleep(20 * time.Millisecond)

	a := &app{httpSrv: srv}
	a.close()

	// Serve debe retornar con ErrServerClosed tras el Shutdown de close().
	select {
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			t.Errorf("Serve() retornó %v, want http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close() no apagó el servidor HTTP dentro del timeout")
	}

	// El puerto debe quedar libre: una nueva conexión debe fallar.
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Errorf("el servidor sigue aceptando conexiones en %s tras close()", addr)
	}
}
