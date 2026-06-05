// Package server contiene el servidor gRPC del BC Notification.
// Notification no expone servicios gRPC — actúa solo como cliente
// (llama a Terminal Gateway). Este archivo es un placeholder.
package server

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/juantevez/go-posnet/pkg/observability"
	"google.golang.org/grpc"
)

// NotificationServer es el servidor gRPC del BC.
type NotificationServer struct{}

func NewNotificationServer() *NotificationServer {
	return &NotificationServer{}
}

// Start arranca el servidor gRPC en el puerto dado.
func Start(srv *NotificationServer, grpcPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("grpc server: listen on port %d: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(observability.GRPCUnaryServerInterceptor()),
	)

	// Notification no expone servicios gRPC propios.
	// El servidor queda abierto para consultas de salud futuras.

	slog.Info("Notification gRPC server listening", slog.Int("port", grpcPort))
	return grpcServer.Serve(listener)
}
