package server_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/infrastructure/grpc/server"
)

func TestNewNotificationServer(t *testing.T) {
	srv := server.NewNotificationServer()
	if srv == nil {
		t.Fatal("NewNotificationServer() = nil, want non-nil")
	}
}

func TestStart_ListenError(t *testing.T) {
	// Ocupar un puerto libre primero para forzar el error de bind en Start().
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer occupied.Close()

	port := occupied.Addr().(*net.TCPAddr).Port

	srv := server.NewNotificationServer()
	err = server.Start(srv, port)
	if err == nil {
		t.Fatal("Start() error = nil, want bind error")
	}
	if !strings.Contains(err.Error(), "listen on port") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "listen on port")
	}
}

func TestStart_ServesSuccessfully(t *testing.T) {
	srv := server.NewNotificationServer()

	errCh := make(chan error, 1)
	go func() {
		// Puerto 0: el OS asigna uno libre. Start() no expone el listener ni el
		// *grpc.Server, así que no hay forma de apagarlo prolijamente desde el
		// test; el goroutine queda corriendo en background hasta que termine el
		// binario de test, que libera el socket al salir.
		errCh <- server.Start(srv, 0)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Start() returned early with error = %v, want it to keep serving", err)
	case <-time.After(200 * time.Millisecond):
		// No devolvió error en la ventana de espera → bind + Serve() arrancaron
		// correctamente.
	}
}
