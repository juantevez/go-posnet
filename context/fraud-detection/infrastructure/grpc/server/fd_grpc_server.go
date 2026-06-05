// Package server contiene el servidor gRPC del BC Fraud Detection.
// Este BC no expone un servicio gRPC propio en el flujo crítico —
// toda la comunicación es asíncrona vía NATS.
// El servidor gRPC queda disponible para consultas de operación futuras.
package server

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/juantevez/go-posnet/pkg/observability"
	"google.golang.org/grpc"
)

// FraudDetectionServer es el servidor gRPC del BC.
// Por ahora no implementa ningún servicio proto propio —
// el BC Fraud Detection no tiene un .proto dedicado porque
// toda su comunicación es asíncrona via NATS JetStream.
// Este archivo existe como placeholder para servicios futuros
// (ej: consulta de reglas desde otros BCs vía gRPC).
type FraudDetectionServer struct{}

func NewFraudDetectionServer() *FraudDetectionServer {
	return &FraudDetectionServer{}
}

// Start arranca el servidor gRPC en el puerto dado.
// Bloqueante — llamar en goroutine desde main.go.
func Start(srv *FraudDetectionServer, grpcPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("grpc server: listen on port %d: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(observability.GRPCUnaryServerInterceptor()),
	)

	// TODO: registrar servicios gRPC cuando se definan
	// ej: fraudv1.RegisterFraudDetectionServiceServer(grpcServer, srv)

	slog.Info("Fraud Detection gRPC server listening", slog.Int("port", grpcPort))
	return grpcServer.Serve(listener)
}
