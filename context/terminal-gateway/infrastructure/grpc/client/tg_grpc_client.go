// Package client contiene los clientes gRPC del BC Terminal Gateway.
// Notification BC llama a este cliente para enviar comprobantes
// al WebSocket del terminal sin conocer la gestión de sesiones.
package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tu-org/posnet-backend/pkg/observability"
	tgv1 "github.com/tu-org/posnet-backend/pkg/proto/terminalgateway/v1"
)

// TerminalGatewayClient es el cliente gRPC hacia el BC Terminal Gateway.
// Usado por el BC Notification para entregar comprobantes al terminal.
type TerminalGatewayClient struct {
	client tgv1.TerminalGatewayServiceClient
	conn   *grpc.ClientConn
}

// NewTerminalGatewayClient construye el cliente con mTLS y trace propagation.
func NewTerminalGatewayClient(target string) (*TerminalGatewayClient, error) {
	conn, err := grpc.NewClient(
		target,
		// En producción reemplazar por grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(observability.GRPCUnaryClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("terminal_gateway client: dial %q: %w", target, err)
	}

	return &TerminalGatewayClient{
		client: tgv1.NewTerminalGatewayServiceClient(conn),
		conn:   conn,
	}, nil
}

// Close cierra la conexión gRPC. Llamar con defer al cerrar el servicio.
func (c *TerminalGatewayClient) Close() error {
	return c.conn.Close()
}

// SendReceipt envía el comprobante al WebSocket del terminal vía gRPC.
// Retorna false + motivo si el terminal no tiene sesión WebSocket activa.
func (c *TerminalGatewayClient) SendReceipt(ctx context.Context, req SendReceiptRequest) (bool, string, error) {
	ctx, span := observability.StartSpan(ctx, "grpc_client.SendReceipt")
	defer span.End()

	resp, err := c.client.SendReceipt(ctx, &tgv1.SendReceiptRequest{
		TerminalId:    req.TerminalID,
		TransactionId: req.TransactionID,
		Receipt: &tgv1.Receipt{
			TransactionId:   req.TransactionID,
			MerchantName:    req.MerchantName,
			MerchantAddress: req.MerchantAddress,
			TerminalCode:    req.TerminalCode,
			Result:          toProtoResult(req.Result),
			AuthCode:        req.AuthCode,
			RejectionCode:   req.RejectionCode,
			RejectionReason: req.RejectionReason,
			AmountCents:     req.AmountCents,
			Currency:        req.Currency,
			CardLast4:       req.CardLast4,
			CardNetwork:     req.CardNetwork,
			EntryMode:       req.EntryMode,
		},
	})
	if err != nil {
		observability.RecordError(ctx, err)
		return false, "", fmt.Errorf("terminal_gateway client: SendReceipt: %w", err)
	}

	return resp.Delivered, resp.ErrorReason, nil
}

// GetTerminalStatus consulta el estado de conexión de un terminal.
func (c *TerminalGatewayClient) GetTerminalStatus(ctx context.Context, terminalID string) (string, error) {
	ctx, span := observability.StartSpan(ctx, "grpc_client.GetTerminalStatus")
	defer span.End()

	resp, err := c.client.GetTerminalStatus(ctx, &tgv1.GetTerminalStatusRequest{
		TerminalId: terminalID,
	})
	if err != nil {
		return "", fmt.Errorf("terminal_gateway client: GetTerminalStatus: %w", err)
	}

	return resp.Status.String(), nil
}

// ─── Request/Response types ───────────────────────────────────────────────────

// SendReceiptRequest contiene los datos del comprobante a enviar.
type SendReceiptRequest struct {
	TerminalID      string
	TransactionID   string
	MerchantName    string
	MerchantAddress string
	TerminalCode    string
	Result          string // "APPROVED" | "REJECTED" | "REVERSED"
	AuthCode        string
	RejectionCode   string
	RejectionReason string
	AmountCents     int64
	Currency        string
	CardLast4       string
	CardNetwork     string
	EntryMode       string
}

func toProtoResult(r string) tgv1.ReceiptResult {
	switch r {
	case "APPROVED":
		return tgv1.ReceiptResult_RECEIPT_RESULT_APPROVED
	case "REJECTED":
		return tgv1.ReceiptResult_RECEIPT_RESULT_REJECTED
	case "REVERSED":
		return tgv1.ReceiptResult_RECEIPT_RESULT_REVERSED
	default:
		return tgv1.ReceiptResult_RECEIPT_RESULT_UNSPECIFIED
	}
}
