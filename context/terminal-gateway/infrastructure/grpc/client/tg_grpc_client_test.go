package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"

	tgv1 "github.com/juantevez/go-posnet/pkg/proto/terminalgateway/v1"
)

// fakeTGClient implementa tgv1.TerminalGatewayServiceClient directamente —
// la interfaz generada por protoc solo tiene 2 métodos, así que no hace
// falta el truco de embeber una interfaz nil.
type fakeTGClient struct {
	sendReceiptReq  *tgv1.SendReceiptRequest
	sendReceiptResp *tgv1.SendReceiptResponse
	sendReceiptErr  error
	statusReq       *tgv1.GetTerminalStatusRequest
	statusResp      *tgv1.GetTerminalStatusResponse
	statusErr       error
}

func (f *fakeTGClient) SendReceipt(_ context.Context, in *tgv1.SendReceiptRequest, _ ...grpc.CallOption) (*tgv1.SendReceiptResponse, error) {
	f.sendReceiptReq = in
	if f.sendReceiptErr != nil {
		return nil, f.sendReceiptErr
	}
	return f.sendReceiptResp, nil
}

func (f *fakeTGClient) GetTerminalStatus(_ context.Context, in *tgv1.GetTerminalStatusRequest, _ ...grpc.CallOption) (*tgv1.GetTerminalStatusResponse, error) {
	f.statusReq = in
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.statusResp, nil
}

// ─── NewTerminalGatewayClient / Close ─────────────────────────────────────────

func TestNewTerminalGatewayClient_Success(t *testing.T) {
	// grpc.NewClient conecta perezosamente — no hace falta un servidor real
	// para construir el cliente exitosamente.
	c, err := NewTerminalGatewayClient("localhost:0")
	if err != nil {
		t.Fatalf("NewTerminalGatewayClient() error = %v", err)
	}
	if c == nil {
		t.Fatal("client is nil, want non-nil")
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// ─── SendReceipt ──────────────────────────────────────────────────────────────

func TestSendReceipt_Success(t *testing.T) {
	fake := &fakeTGClient{sendReceiptResp: &tgv1.SendReceiptResponse{Delivered: true}}
	c := &TerminalGatewayClient{client: fake}

	req := SendReceiptRequest{
		TerminalID:    "term-1",
		TransactionID: "tx-1",
		MerchantName:  "Merchant Inc",
		Result:        "APPROVED",
		AuthCode:      "AUTH123",
		AmountCents:   5000,
		Currency:      "ARS",
	}
	delivered, reason, err := c.SendReceipt(context.Background(), req)
	if err != nil {
		t.Fatalf("SendReceipt() error = %v", err)
	}
	if !delivered {
		t.Error("delivered = false, want true")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}

	if fake.sendReceiptReq.TerminalId != "term-1" {
		t.Errorf("request.TerminalId = %q, want %q", fake.sendReceiptReq.TerminalId, "term-1")
	}
	if fake.sendReceiptReq.Receipt.MerchantName != "Merchant Inc" {
		t.Errorf("request.Receipt.MerchantName = %q, want %q", fake.sendReceiptReq.Receipt.MerchantName, "Merchant Inc")
	}
	if fake.sendReceiptReq.Receipt.Result != tgv1.ReceiptResult_RECEIPT_RESULT_APPROVED {
		t.Errorf("request.Receipt.Result = %v, want APPROVED", fake.sendReceiptReq.Receipt.Result)
	}
	if fake.sendReceiptReq.Receipt.AmountCents != 5000 {
		t.Errorf("request.Receipt.AmountCents = %d, want 5000", fake.sendReceiptReq.Receipt.AmountCents)
	}
}

func TestSendReceipt_NotDelivered(t *testing.T) {
	fake := &fakeTGClient{sendReceiptResp: &tgv1.SendReceiptResponse{Delivered: false, ErrorReason: "TERMINAL_NOT_CONNECTED"}}
	c := &TerminalGatewayClient{client: fake}

	delivered, reason, err := c.SendReceipt(context.Background(), SendReceiptRequest{Result: "REJECTED"})
	if err != nil {
		t.Fatalf("SendReceipt() error = %v", err)
	}
	if delivered {
		t.Error("delivered = true, want false")
	}
	if reason != "TERMINAL_NOT_CONNECTED" {
		t.Errorf("reason = %q, want %q", reason, "TERMINAL_NOT_CONNECTED")
	}
}

func TestSendReceipt_Error(t *testing.T) {
	fake := &fakeTGClient{sendReceiptErr: errors.New("connection refused")}
	c := &TerminalGatewayClient{client: fake}

	_, _, err := c.SendReceipt(context.Background(), SendReceiptRequest{})
	if err == nil || !strings.Contains(err.Error(), "terminal_gateway client: SendReceipt") {
		t.Fatalf("error = %v, want it to mention terminal_gateway client: SendReceipt", err)
	}
}

// ─── GetTerminalStatus ────────────────────────────────────────────────────────

func TestGetTerminalStatus_Success(t *testing.T) {
	fake := &fakeTGClient{statusResp: &tgv1.GetTerminalStatusResponse{
		Status: tgv1.TerminalConnectionStatus_TERMINAL_CONNECTION_STATUS_CONNECTED,
	}}
	c := &TerminalGatewayClient{client: fake}

	status, err := c.GetTerminalStatus(context.Background(), "term-1")
	if err != nil {
		t.Fatalf("GetTerminalStatus() error = %v", err)
	}
	if status != "TERMINAL_CONNECTION_STATUS_CONNECTED" {
		t.Errorf("status = %q, want %q", status, "TERMINAL_CONNECTION_STATUS_CONNECTED")
	}
	if fake.statusReq.TerminalId != "term-1" {
		t.Errorf("request.TerminalId = %q, want %q", fake.statusReq.TerminalId, "term-1")
	}
}

func TestGetTerminalStatus_Error(t *testing.T) {
	fake := &fakeTGClient{statusErr: errors.New("connection refused")}
	c := &TerminalGatewayClient{client: fake}

	_, err := c.GetTerminalStatus(context.Background(), "term-1")
	if err == nil || !strings.Contains(err.Error(), "terminal_gateway client: GetTerminalStatus") {
		t.Fatalf("error = %v, want it to mention terminal_gateway client: GetTerminalStatus", err)
	}
}

// ─── toProtoResult ────────────────────────────────────────────────────────────

func TestToProtoResult(t *testing.T) {
	tests := []struct {
		input string
		want  tgv1.ReceiptResult
	}{
		{"APPROVED", tgv1.ReceiptResult_RECEIPT_RESULT_APPROVED},
		{"REJECTED", tgv1.ReceiptResult_RECEIPT_RESULT_REJECTED},
		{"REVERSED", tgv1.ReceiptResult_RECEIPT_RESULT_REVERSED},
		{"UNKNOWN", tgv1.ReceiptResult_RECEIPT_RESULT_UNSPECIFIED},
		{"", tgv1.ReceiptResult_RECEIPT_RESULT_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := toProtoResult(tc.input); got != tc.want {
				t.Errorf("toProtoResult(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
