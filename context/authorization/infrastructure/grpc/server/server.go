// Package server contiene el servidor gRPC del BC Authorization.
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

	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	authv1 "github.com/juantevez/go-posnet/pkg/proto/authorization/v1"
)

// AuthorizationServer implementa authv1.AuthorizationServiceServer.
type AuthorizationServer struct {
	authv1.UnimplementedAuthorizationServiceServer
	queryService port.QueryService
}

func NewAuthorizationServer(queryService port.QueryService) *AuthorizationServer {
	return &AuthorizationServer{queryService: queryService}
}

// Start arranca el servidor gRPC en el puerto dado.
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

// ListTerminalTransactions — no implementado aún.
func (s *AuthorizationServer) ListTerminalTransactions(
	ctx context.Context,
	req *authv1.ListTerminalTransactionsRequest,
) (*authv1.ListTerminalTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ListTerminalTransactions not yet implemented")
}

// ─── Mapper ───────────────────────────────────────────────────────────────────

func toProtoStatusResponse(r *port.TransactionStatusResult) *authv1.GetTransactionStatusResponse {
	resp := &authv1.GetTransactionStatusResponse{
		TransactionId:   r.TransactionID,
		State:           toProtoState(r.State),
		AmountCents:     r.AmountCents,
		Currency:        r.Currency,
		AuthCode:        r.AuthCode,
		RejectionCode:   r.RejectionCode,
		RejectionReason: r.RejectionReason,
		// Timestamps como string RFC3339 — sin timestamppb
		AuthorizedAt: r.AuthorizedAt,
		RejectedAt:   r.RejectedAt,
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

// parseRFC3339 parsea un string RFC3339 — helper local.
func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}
