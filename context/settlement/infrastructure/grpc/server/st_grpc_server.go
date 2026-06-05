// Package server contiene el servidor gRPC del BC Settlement.
// Settlement no expone servicios gRPC en el flujo crítico —
// toda su comunicación es asíncrona vía NATS.
// Este archivo es un placeholder para servicios futuros.
package server

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/juantevez/go-posnet/pkg/observability"
	"google.golang.org/grpc"
)

// SettlementServer es el servidor gRPC del BC.
type SettlementServer struct{}

func NewSettlementServer() *SettlementServer {
	return &SettlementServer{}
}

// Start arranca el servidor gRPC en el puerto dado.
func Start(srv *SettlementServer, grpcPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("grpc server: listen on port %d: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(observability.GRPCUnaryServerInterceptor()),
	)

	// TODO: registrar servicios gRPC cuando se definan
	// ej: settlementv1.RegisterSettlementServiceServer(grpcServer, srv)

	slog.Info("Settlement gRPC server listening", slog.Int("port", grpcPort))
	return grpcServer.Serve(listener)
}
