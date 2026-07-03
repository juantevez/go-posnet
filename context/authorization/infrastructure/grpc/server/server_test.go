package server

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	authv1 "github.com/juantevez/go-posnet/pkg/proto/authorization/v1"
)

// ─── fakeQueryService ────────────────────────────────────────────────────────

type fakeQueryService struct {
	result     *port.TransactionStatusResult
	err        error
	calledWith domain.TransactionID
}

var _ port.QueryService = (*fakeQueryService)(nil)

func (f *fakeQueryService) GetTransactionStatus(_ context.Context, id domain.TransactionID) (*port.TransactionStatusResult, error) {
	f.calledWith = id
	return f.result, f.err
}

// ─── GetTransactionStatus ─────────────────────────────────────────────────────

func TestGetTransactionStatus_InvalidTransactionID(t *testing.T) {
	srv := NewAuthorizationServer(&fakeQueryService{})

	_, err := srv.GetTransactionStatus(context.Background(), &authv1.GetTransactionStatusRequest{TransactionId: "not-a-uuid"})
	if err == nil {
		t.Fatal("GetTransactionStatus() error = nil, want error")
	}
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want %v", code, codes.InvalidArgument)
	}
}

func TestGetTransactionStatus_NotFound(t *testing.T) {
	id := domain.NewTransactionID()
	qs := &fakeQueryService{err: pkgerrors.NewNotFoundError("Transaction", id.String())}
	srv := NewAuthorizationServer(qs)

	_, err := srv.GetTransactionStatus(context.Background(), &authv1.GetTransactionStatusRequest{TransactionId: id.String()})
	if err == nil {
		t.Fatal("GetTransactionStatus() error = nil, want error")
	}
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("code = %v, want %v", code, codes.NotFound)
	}
}

func TestGetTransactionStatus_InternalError(t *testing.T) {
	id := domain.NewTransactionID()
	qs := &fakeQueryService{err: errors.New("db unreachable")}
	srv := NewAuthorizationServer(qs)

	_, err := srv.GetTransactionStatus(context.Background(), &authv1.GetTransactionStatusRequest{TransactionId: id.String()})
	if err == nil {
		t.Fatal("GetTransactionStatus() error = nil, want error")
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("code = %v, want %v", code, codes.Internal)
	}
	// El mensaje interno no debe filtrarse al cliente gRPC.
	if strings.Contains(status.Convert(err).Message(), "db unreachable") {
		t.Errorf("internal error message leaked to client: %q", status.Convert(err).Message())
	}
}

func TestGetTransactionStatus_Success(t *testing.T) {
	id := domain.NewTransactionID()
	result := &port.TransactionStatusResult{
		TransactionID: id.String(),
		State:         "APPROVED",
		AmountCents:   5000,
		Currency:      "ARS",
		AuthCode:      "AB1234",
		AuthorizedAt:  "2026-01-01T10:00:00Z",
	}
	qs := &fakeQueryService{result: result}
	srv := NewAuthorizationServer(qs)

	resp, err := srv.GetTransactionStatus(context.Background(), &authv1.GetTransactionStatusRequest{TransactionId: id.String()})
	if err != nil {
		t.Fatalf("GetTransactionStatus() error = %v", err)
	}
	if resp.TransactionId != id.String() {
		t.Errorf("TransactionId = %q, want %q", resp.TransactionId, id.String())
	}
	if resp.State != authv1.TransactionState_TRANSACTION_STATE_APPROVED {
		t.Errorf("State = %v, want %v", resp.State, authv1.TransactionState_TRANSACTION_STATE_APPROVED)
	}
	if resp.AmountCents != 5000 {
		t.Errorf("AmountCents = %d, want 5000", resp.AmountCents)
	}
	if resp.Currency != "ARS" {
		t.Errorf("Currency = %q, want %q", resp.Currency, "ARS")
	}
	if resp.AuthCode != "AB1234" {
		t.Errorf("AuthCode = %q, want %q", resp.AuthCode, "AB1234")
	}
	if resp.AuthorizedAt != "2026-01-01T10:00:00Z" {
		t.Errorf("AuthorizedAt = %q, want %q", resp.AuthorizedAt, "2026-01-01T10:00:00Z")
	}
	if !qs.calledWith.Equals(id) {
		t.Errorf("queryService called with %v, want %v", qs.calledWith, id)
	}
}

// ─── ListTerminalTransactions ─────────────────────────────────────────────────

func TestListTerminalTransactions_Unimplemented(t *testing.T) {
	srv := NewAuthorizationServer(&fakeQueryService{})

	_, err := srv.ListTerminalTransactions(context.Background(), &authv1.ListTerminalTransactionsRequest{})
	if err == nil {
		t.Fatal("ListTerminalTransactions() error = nil, want error")
	}
	if code := status.Code(err); code != codes.Unimplemented {
		t.Errorf("code = %v, want %v", code, codes.Unimplemented)
	}
}

// ─── toProtoState ──────────────────────────────────────────────────────────────

func TestToProtoState(t *testing.T) {
	tests := []struct {
		in   string
		want authv1.TransactionState
	}{
		{"RECEIVED", authv1.TransactionState_TRANSACTION_STATE_RECEIVED},
		{"FRAUD_CHECKING", authv1.TransactionState_TRANSACTION_STATE_FRAUD_CHECKING},
		{"PROCESSING", authv1.TransactionState_TRANSACTION_STATE_PROCESSING},
		{"APPROVED", authv1.TransactionState_TRANSACTION_STATE_APPROVED},
		{"REJECTED", authv1.TransactionState_TRANSACTION_STATE_REJECTED},
		{"INDETERMINATE", authv1.TransactionState_TRANSACTION_STATE_INDETERMINATE},
		{"REVERSED", authv1.TransactionState_TRANSACTION_STATE_REVERSED},
		{"BOGUS", authv1.TransactionState_TRANSACTION_STATE_UNSPECIFIED},
		{"", authv1.TransactionState_TRANSACTION_STATE_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := toProtoState(tc.in); got != tc.want {
				t.Errorf("toProtoState(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// ─── toProtoStatusResponse ───────────────────────────────────────────────────

func TestToProtoStatusResponse(t *testing.T) {
	r := &port.TransactionStatusResult{
		TransactionID:   "tx-1",
		State:           "REJECTED",
		AmountCents:     1000,
		Currency:        "USD",
		RejectionCode:   "05",
		RejectionReason: "Do Not Honor",
		RejectedAt:      "2026-01-01T10:00:00Z",
	}

	resp := toProtoStatusResponse(r)

	if resp.TransactionId != r.TransactionID {
		t.Errorf("TransactionId = %q, want %q", resp.TransactionId, r.TransactionID)
	}
	if resp.State != authv1.TransactionState_TRANSACTION_STATE_REJECTED {
		t.Errorf("State = %v, want %v", resp.State, authv1.TransactionState_TRANSACTION_STATE_REJECTED)
	}
	if resp.AmountCents != r.AmountCents {
		t.Errorf("AmountCents = %d, want %d", resp.AmountCents, r.AmountCents)
	}
	if resp.Currency != r.Currency {
		t.Errorf("Currency = %q, want %q", resp.Currency, r.Currency)
	}
	if resp.RejectionCode != r.RejectionCode {
		t.Errorf("RejectionCode = %q, want %q", resp.RejectionCode, r.RejectionCode)
	}
	if resp.RejectionReason != r.RejectionReason {
		t.Errorf("RejectionReason = %q, want %q", resp.RejectionReason, r.RejectionReason)
	}
	if resp.RejectedAt != r.RejectedAt {
		t.Errorf("RejectedAt = %q, want %q", resp.RejectedAt, r.RejectedAt)
	}
	if resp.AuthCode != "" {
		t.Errorf("AuthCode = %q, want empty", resp.AuthCode)
	}
	if resp.AuthorizedAt != "" {
		t.Errorf("AuthorizedAt = %q, want empty", resp.AuthorizedAt)
	}
}

// ─── parseRFC3339 ────────────────────────────────────────────────────────────

func TestParseRFC3339(t *testing.T) {
	t.Run("empty string returns zero time without error", func(t *testing.T) {
		got, err := parseRFC3339("")
		if err != nil {
			t.Fatalf("parseRFC3339(\"\") error = %v", err)
		}
		if !got.IsZero() {
			t.Errorf("parseRFC3339(\"\") = %v, want zero time", got)
		}
	})

	t.Run("valid RFC3339", func(t *testing.T) {
		got, err := parseRFC3339("2026-01-01T10:00:00Z")
		if err != nil {
			t.Fatalf("parseRFC3339() error = %v", err)
		}
		want := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("parseRFC3339() = %v, want %v", got, want)
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		if _, err := parseRFC3339("not-a-date"); err == nil {
			t.Fatal("parseRFC3339(\"not-a-date\") error = nil, want error")
		}
	})
}

// ─── Start ───────────────────────────────────────────────────────────────────

func TestStart_ListenError(t *testing.T) {
	// Ocupar un puerto libre primero para forzar el error de bind en Start().
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer occupied.Close()

	port := occupied.Addr().(*net.TCPAddr).Port

	srv := NewAuthorizationServer(&fakeQueryService{})
	err = Start(srv, port)
	if err == nil {
		t.Fatal("Start() error = nil, want bind error")
	}
	if !strings.Contains(err.Error(), "listen on port") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "listen on port")
	}
}

func TestStart_ServesSuccessfully(t *testing.T) {
	srv := NewAuthorizationServer(&fakeQueryService{})

	errCh := make(chan error, 1)
	go func() {
		// Puerto 0: el OS asigna uno libre. Start() no expone el listener ni el
		// *grpc.Server, así que no hay forma de apagarlo prolijamente desde el test;
		// el goroutine queda corriendo en background hasta que termine el binario
		// de test, que libera el socket al salir.
		errCh <- Start(srv, 0)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Start() returned early with error = %v, want it to keep serving", err)
	case <-time.After(200 * time.Millisecond):
		// No devolvió error en la ventana de espera → bind + register + Serve()
		// arrancaron correctamente.
	}
}
