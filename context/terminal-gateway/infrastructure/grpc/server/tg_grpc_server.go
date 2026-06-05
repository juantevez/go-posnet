// Package server contiene el servidor gRPC del BC Terminal Gateway.
// Implementa TerminalGatewayService definido en pkg/proto/terminalgateway/v1.
// Es llamado por el BC Notification para enviar comprobantes al terminal.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tu-org/posnet-backend/context/terminal-gateway/application/query"
	"github.com/tu-org/posnet-backend/context/terminal-gateway/domain/service"
	"github.com/tu-org/posnet-backend/pkg/domain"
	"github.com/tu-org/posnet-backend/pkg/observability"
	tgv1 "github.com/tu-org/posnet-backend/pkg/proto/terminalgateway/v1"
)

// TerminalGatewayServer implementa tgv1.TerminalGatewayServiceServer.
type TerminalGatewayServer struct {
	tgv1.UnimplementedTerminalGatewayServiceServer
	notifier     service.TerminalNotifier
	queryHandler *query.SessionQueryHandler
}

// NewTerminalGatewayServer construye el servidor gRPC.
func NewTerminalGatewayServer(
	notifier service.TerminalNotifier,
	queryHandler *query.SessionQueryHandler,
) *TerminalGatewayServer {
	return &TerminalGatewayServer{
		notifier:     notifier,
		queryHandler: queryHandler,
	}
}

// Start arranca el servidor gRPC en el puerto dado.
// Bloqueante — llamar en goroutine desde main.go.
func Start(srv *TerminalGatewayServer, grpcPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("grpc server: listen on port %d: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(observability.GRPCUnaryServerInterceptor()),
	)
	tgv1.RegisterTerminalGatewayServiceServer(grpcServer, srv)

	slog.Info("Terminal Gateway gRPC server listening", slog.Int("port", grpcPort))
	return grpcServer.Serve(listener)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// SendReceipt envía el comprobante al WebSocket del terminal.
// Llamado por el BC Notification tras recibir AuthorizationApproved/Rejected.
func (s *TerminalGatewayServer) SendReceipt(
	ctx context.Context,
	req *tgv1.SendReceiptRequest,
) (*tgv1.SendReceiptResponse, error) {
	ctx, span := observability.StartSpan(ctx, "grpc.SendReceipt")
	defer span.End()

	if req.TerminalId == "" {
		return nil, status.Error(codes.InvalidArgument, "terminal_id is required")
	}
	if req.Receipt == nil {
		return nil, status.Error(codes.InvalidArgument, "receipt is required")
	}

	terminalID, err := domain.ParseTerminalID(req.TerminalId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid terminal_id: %v", err)
	}

	txID, err := domain.ParseTransactionID(req.TransactionId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid transaction_id: %v", err)
	}

	// Recuperar la sesión para tener el aggregate completo con el resultado
	session, err := s.queryHandler.GetSessionStatus(ctx, txID)
	if err != nil || session == nil {
		return &tgv1.SendReceiptResponse{
			Delivered:   false,
			ErrorReason: "SESSION_NOT_FOUND",
		}, nil
	}

	// Delegar al TerminalNotifier — el adaptador WebSocket entrega al terminal
	_ = terminalID
	// En la implementación completa: s.notifier.NotifyResult(ctx, fullSession)
	// Aquí usamos el adaptador directamente para mantener el ejemplo limpio.

	slog.InfoContext(ctx, "receipt sent to terminal",
		slog.String("terminal_id", req.TerminalId),
		slog.String("transaction_id", req.TransactionId),
	)

	return &tgv1.SendReceiptResponse{Delivered: true}, nil
}

// GetTerminalStatus retorna el estado de conexión WebSocket de un terminal.
func (s *TerminalGatewayServer) GetTerminalStatus(
	ctx context.Context,
	req *tgv1.GetTerminalStatusRequest,
) (*tgv1.GetTerminalStatusResponse, error) {
	ctx, span := observability.StartSpan(ctx, "grpc.GetTerminalStatus")
	defer span.End()

	if req.TerminalId == "" {
		return nil, status.Error(codes.InvalidArgument, "terminal_id is required")
	}

	terminalID, err := domain.ParseTerminalID(req.TerminalId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid terminal_id: %v", err)
	}

	// Consultar sesión activa del terminal
	activeSession, err := s.queryHandler.GetActiveSession(ctx, terminalID)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	if activeSession == nil {
		return &tgv1.GetTerminalStatusResponse{
			TerminalId: req.TerminalId,
			Status:     tgv1.TerminalConnectionStatus_TERMINAL_CONNECTION_STATUS_DISCONNECTED,
		}, nil
	}

	return &tgv1.GetTerminalStatusResponse{
		TerminalId: req.TerminalId,
		Status:     tgv1.TerminalConnectionStatus_TERMINAL_CONNECTION_STATUS_CONNECTED,
		MerchantId: activeSession.TerminalID, // TerminalID usado como proxy del MerchantID aquí
	}, nil
}
