package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/application/port"
	"github.com/juantevez/go-posnet/context/terminal-gateway/application/query"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// QRHandler expone los endpoints JSON del flujo de pago con QR.
// No contiene HTML — el frontend vive en un repo separado (posnet-frontend).
type QRHandler struct {
	sessionService port.SessionService
	queryHandler   *query.SessionQueryHandler
	natsPub        *natsutil.Publisher
}

func NewQRHandler(
	sessionService port.SessionService,
	queryHandler *query.SessionQueryHandler,
	natsPub *natsutil.Publisher,
) *QRHandler {
	return &QRHandler{
		sessionService: sessionService,
		queryHandler:   queryHandler,
		natsPub:        natsPub,
	}
}

// RegisterQRRoutes registra los endpoints en el mux existente.
func (h *QRHandler) RegisterQRRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/sessions/create", h.handleCreateQRSession)
	mux.HandleFunc("POST /api/sessions/{id}/pay", h.handleSimulatePay)
	mux.HandleFunc("GET  /api/sessions/{id}/status", h.handleSessionStatus)
}

// POST /api/sessions/create
// Crea una PaymentSession y retorna los datos necesarios para que
// el frontend genere el QR y muestre el monto.
//
// Request:
//
//	{ "amount_cents": 150000, "currency": "ARS",
//	  "terminal_id": "...", "merchant_id": "..." }
//
// Response:
//
//	{ "transaction_id": "...", "qr_content": "http://192.168.x.x:5173/pay/{id}",
//	  "ttl_seconds": 300, "amount_cents": 150000, "currency": "ARS" }
func (h *QRHandler) handleCreateQRSession(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.CreateQRSession")
	defer span.End()

	var req struct {
		TerminalID  string `json:"terminal_id"`
		MerchantID  string `json:"merchant_id"`
		AmountCents int64  `json:"amount_cents"`
		Currency    string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_BODY", "invalid request body"))
		return
	}

	// Defaults para el simulador
	if req.TerminalID == "" {
		req.TerminalID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	}
	if req.MerchantID == "" {
		req.MerchantID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
	}
	if req.Currency == "" {
		req.Currency = "ARS"
	}
	if req.AmountCents <= 0 {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_AMOUNT", "amount_cents must be positive"))
		return
	}

	stan := int(time.Now().UnixNano()%999998 + 1)

	cmd := port.CreateSessionCommand{
		TerminalID:     req.TerminalID,
		MerchantID:     req.MerchantID,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		STAN:           stan,
		PaymentChannel: "QR",
	}

	result, err := h.sessionService.CreateSession(ctx, cmd)
	if err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", err.Error()))
		return
	}

	// qr_content apunta al frontend (posnet-frontend) con la IP local
	// El celular escanea este QR y abre la página de pago del frontend
	localIP := getLocalIP()
	qrContent := fmt.Sprintf("http://%s:5173/pay/%s", localIP, result.TransactionID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"transaction_id": result.TransactionID,
		"expires_at":     result.ExpiresAt,
		"ttl_seconds":    result.TTLSeconds,
		"amount_cents":   req.AmountCents,
		"currency":       req.Currency,
		"qr_content":     qrContent,
	})
}

// POST /api/sessions/{id}/pay
// Simula el pago del cliente: publica TransactionReceived a NATS
// disparando la Saga completa (Fraud → Acquirer → APPROVED/REJECTED).
//
// Llamado desde la página /pay/{id} del frontend cuando el cliente confirma.
//
// Request:
//
//	{ "card_last4": "1234", "card_network": "VISA", "entry_mode": "QR" }
//
// Response 202:
//
//	{ "status": "processing", "transaction_id": "..." }
func (h *QRHandler) handleSimulatePay(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.SimulatePay")
	defer span.End()

	txID := r.PathValue("id")
	parsedID, err := domain.ParseTransactionID(txID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_ID", "invalid transaction id"))
		return
	}

	var req struct {
		CardLast4   string `json:"card_last4"`
		CardNetwork string `json:"card_network"`
		EntryMode   string `json:"entry_mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.CardLast4 == "" {
		req.CardLast4 = "0000"
	}
	if req.CardNetwork == "" {
		req.CardNetwork = "VISA"
	}
	if req.EntryMode == "" {
		req.EntryMode = "QR"
	}

	session, err := h.queryHandler.GetSessionStatus(ctx, parsedID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "session not found or expired"))
		return
	}

	eventID := domain.NewTransactionID().String()
	payload := events.TransactionReceivedPayload{
		TransactionID: txID,
		TerminalID:    session.TerminalID,
		MerchantID:    session.MerchantID,
		AmountCents:   session.AmountCents,
		Currency:      session.Currency,
		STAN:          int(time.Now().UnixNano() % 999998 + 1),
		EntryMode:     req.EntryMode,
		CardLast4:     req.CardLast4,
		CardNetwork:   req.CardNetwork,
		EMVDataBase64: "",
		ReceivedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	_, err = h.natsPub.Publish(ctx,
		events.SubjectTransactionReceived,
		events.SubjectTransactionReceived,
		txID, "PaymentSession",
		txID, eventID,
		payload,
	)
	if err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("NATS_ERROR", "failed to publish payment"))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":         "processing",
		"transaction_id": txID,
	})
}

// GET /api/sessions/{id}/status
// Polling del estado de la sesión — usado por el cajero y por la página del celular.
//
// Response:
//
//	{ "transaction_id": "...", "state": "APPROVED|REJECTED|PROCESSING|EXPIRED",
//	  "auth_code": "A00002", "rejection_reason": "...",
//	  "amount_cents": 150000, "currency": "ARS", "ttl_seconds": 247 }
func (h *QRHandler) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.SessionStatus")
	defer span.End()

	txID := r.PathValue("id")
	parsedID, err := domain.ParseTransactionID(txID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_ID", "invalid transaction id"))
		return
	}

	session, err := h.queryHandler.GetSessionStatus(ctx, parsedID)
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "session not found"))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"transaction_id":   session.TransactionID,
		"state":            session.State,
		"amount_cents":     session.AmountCents,
		"currency":         session.Currency,
		"auth_code":        session.AuthCode,
		"rejection_code":   session.RejectionCode,
		"rejection_reason": session.RejectionReason,
		"ttl_seconds":      session.TTLSeconds,
		"expires_at":       session.ExpiresAt,
	})
}

// getLocalIP detecta la IP local de red — para que el QR sea accesible desde celulares
// en la misma WiFi sin configuración manual.
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
