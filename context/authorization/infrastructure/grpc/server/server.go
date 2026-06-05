// Package server contiene el servidor gRPC del BC Authorization.
// Expone el servicio AuthorizationService definido en pkg/proto/authorization/v1.
// Solo disponible para herramientas de operación — no está en el critical path.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	authv1 "github.com/juantevez/go-posnet/pkg/proto/authorization/v1"
)

// AuthorizationServer implementa authv1.AuthorizationServiceServer.
// Delega todas las operaciones al QueryService de la capa de aplicación.
type AuthorizationServer struct {
	authv1.UnimplementedAuthorizationServiceServer
	queryService port.QueryService
}

// NewAuthorizationServer construye el servidor gRPC.
func NewAuthorizationServer(queryService port.QueryService) *AuthorizationServer {
	return &AuthorizationServer{queryService: queryService}
}

// Start arranca el servidor gRPC en el puerto dado.
// Bloqueante — llamar en una goroutine separada desde main.go.
func Start(srv *AuthorizationServer, grpcPort int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("grpc server: listen on port %d: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(observability.GRPCUnaryServerInterceptor()),
	)

	authv1.RegisterAuthorizationServiceServer(grpcServer, srv)

	slog.Info("gRPC server listening", slog.Int("port", grpcPort))
	return grpcServer.Serve(listener)
}

// ─── AuthorizationService handlers ───────────────────────────────────────────

// GetTransactionStatus retorna el estado de una transacción por ID.
func (s *AuthorizationServer) GetTransactionStatus(
	ctx context.Context,
	req *authv1.GetTransactionStatusRequest,
) (*authv1.GetTransactionStatusResponse, error) {
	ctx, span := observability.StartSpan(ctx, "grpc.GetTransactionStatus")
	defer span.End()

	txID, err := domain.ParseTransactionID(req.TransactionId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid transaction_id: %v", err)
	}

	result, err := s.queryService.GetTransactionStatus(ctx, txID)
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			return nil, status.Errorf(codes.NotFound, "transaction %q not found", req.TransactionId)
		}
		observability.RecordError(ctx, err)
		return nil, status.Errorf(codes.Internal, "internal error")
	}

	return toProtoStatusResponse(result), nil
}

// ListTerminalTransactions lista las transacciones de un terminal en un día.
func (s *AuthorizationServer) ListTerminalTransactions(
	ctx context.Context,
	req *authv1.ListTerminalTransactionsRequest,
) (*authv1.ListTerminalTransactionsResponse, error) {
	// TODO: implementar en la iteración siguiente con un ListQuery dedicado.
	return nil, status.Errorf(codes.Unimplemented, "ListTerminalTransactions not yet implemented")
}

// ─── Mapper proto ─────────────────────────────────────────────────────────────

func toProtoStatusResponse(r *port.TransactionStatusResult) *authv1.GetTransactionStatusResponse {
	resp := &authv1.GetTransactionStatusResponse{
		TransactionId:   r.TransactionID,
		State:           toProtoState(r.State),
		AmountCents:     r.AmountCents,
		Currency:        r.Currency,
		AuthCode:        r.AuthCode,
		RejectionCode:   r.RejectionCode,
		RejectionReason: r.RejectionReason,
	}

	if r.AuthorizedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.AuthorizedAt); err == nil {
			resp.AuthorizedAt = timestamppb.New(t)
		}
	}
	if r.RejectedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.RejectedAt); err == nil {
			resp.RejectedAt = timestamppb.New(t)
		}
	}

	return resp
}

func toProtoState(s string) authv1.TransactionState {
	states := map[string]authv1.TransactionState{
		"RECEIVED":       authv1.TransactionState_TRANSACTION_STATE_RECEIVED,
		"FRAUD_CHECKING": authv1.TransactionState_TRANSACTION_STATE_FRAUD_CHECKING,
		"PROCESSING":     authv1.TransactionState_TRANSACTION_STATE_PROCESSING,
		"APPROVED":       authv1.TransactionState_TRANSACTION_STATE_APPROVED,
		"REJECTED":       authv1.TransactionState_TRANSACTION_STATE_REJECTED,
		"INDETERMINATE":  authv1.TransactionState_TRANSACTION_STATE_INDETERMINATE,
		"REVERSED":       authv1.TransactionState_TRANSACTION_STATE_REVERSED,
	}
	if v, ok := states[s]; ok {
		return v
	}
	return authv1.TransactionState_TRANSACTION_STATE_UNSPECIFIED
}
