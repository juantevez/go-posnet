// Package server contiene el servidor gRPC del BC Terminal Gateway.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/juantevez/go-posnet/context/terminal-gateway/application/query"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/service"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	tgv1 "github.com/juantevez/go-posnet/pkg/proto/terminalgateway/v1"
)

// TerminalGatewayServer implementa tgv1.TerminalGatewayServiceServer.
type TerminalGatewayServer struct {
	tgv1.UnimplementedTerminalGatewayServiceServer
	notifier     service.TerminalNotifier
	queryHandler *query.SessionQueryHandler
}

func NewTerminalGatewayServer(
	notifier service.TerminalNotifier,
	queryHandler *query.SessionQueryHandler,
) *TerminalGatewayServer {
	return &TerminalGatewayServer{
		notifier:     notifier,
		queryHandler: queryHandler,
	}
}

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

// SendReceipt entrega el comprobante al WebSocket del terminal.
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

	txID, err := domain.ParseTransactionID(req.TransactionId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid transaction_id: %v", err)
	}

	_, err = s.queryHandler.GetSessionStatus(ctx, txID)
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			return &tgv1.SendReceiptResponse{
				Delivered:   false,
				ErrorReason: "SESSION_NOT_FOUND",
			}, nil
		}
		observability.RecordError(ctx, err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	slog.InfoContext(ctx, "receipt sent to terminal",
		slog.String("terminal_id", req.TerminalId),
		slog.String("transaction_id", req.TransactionId),
	)

	return &tgv1.SendReceiptResponse{Delivered: true}, nil
}

// GetTerminalStatus retorna el estado de conexión de un terminal.
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
		TerminalId:     req.TerminalId,
		Status:         tgv1.TerminalConnectionStatus_TERMINAL_CONNECTION_STATUS_CONNECTED,
		MerchantId:     activeSession.MerchantID,
		ConnectedSince: activeSession.CreatedAt,
	}, nil
}
