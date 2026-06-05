// Package http contiene el adaptador HTTP del BC Terminal Gateway.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tu-org/posnet-backend/context/terminal-gateway/application/port"
	"github.com/tu-org/posnet-backend/context/terminal-gateway/application/query"
	"github.com/tu-org/posnet-backend/pkg/domain"
	pkgerrors "github.com/tu-org/posnet-backend/pkg/errors"
	"github.com/tu-org/posnet-backend/pkg/observability"
	"github.com/tu-org/posnet-backend/pkg/pgutil"
)

// Handler contiene todos los handlers HTTP del BC Terminal Gateway.
type Handler struct {
	sessionService port.SessionService
	queryHandler   *query.SessionQueryHandler
	pool           *pgxpool.Pool
}

func NewHandler(
	sessionService port.SessionService,
	queryHandler *query.SessionQueryHandler,
	pool *pgxpool.Pool,
) *Handler {
	return &Handler{
		sessionService: sessionService,
		queryHandler:   queryHandler,
		pool:           pool,
	}
}

// RegisterRoutes registra todas las rutas HTTP del BC.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET  /healthz", h.handleHealth)
	mux.HandleFunc("GET  /readyz", h.handleReady)
	mux.HandleFunc("GET  /sessions/{id}", h.handleGetSession)
	mux.HandleFunc("POST /sessions/{id}/cancel", h.handleCancelSession)
	mux.HandleFunc("POST /sessions/{id}/reversal", h.handleRequestReversal)
	mux.HandleFunc("POST /batch-close", h.handleBatchClose)
}

// GET /healthz — liveness probe
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /readyz — readiness probe
func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := pgutil.HealthCheck(r.Context(), h.pool); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready", "reason": "database unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// GET /sessions/{id} — estado de una sesión (operaciones/soporte)
func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetSession")
	defer span.End()

	txID, err := domain.ParseTransactionID(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_ID", "invalid session id"))
		return
	}

	result, err := h.queryHandler.GetSessionStatus(ctx, txID)
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

	writeJSON(w, http.StatusOK, result)
}

// POST /sessions/{id}/cancel — cancelación manual por el cajero
func (h *Handler) handleCancelSession(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.CancelSession")
	defer span.End()

	// El terminal_id viene del header mTLS (CN del certificado)
	terminalID := r.Header.Get("X-Terminal-ID")

	cmd := port.CancelSessionCommand{
		TransactionID: r.PathValue("id"),
		TerminalID:    terminalID,
	}

	if err := h.sessionService.CancelSession(ctx, cmd); err != nil {
		observability.RecordError(ctx, err)
		slog.ErrorContext(ctx, "cancel session failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "cancel failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// POST /sessions/{id}/reversal — anulación de transacción aprobada
func (h *Handler) handleRequestReversal(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.RequestReversal")
	defer span.End()

	terminalID := r.Header.Get("X-Terminal-ID")

	cmd := port.RequestReversalCommand{
		OriginalTransactionID: r.PathValue("id"),
		TerminalID:            terminalID,
	}

	if err := h.sessionService.RequestReversal(ctx, cmd); err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "reversal request failed"))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reversal_requested"})
}

// POST /batch-close — cierre de lote del terminal
func (h *Handler) handleBatchClose(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.BatchClose")
	defer span.End()

	var req struct {
		TerminalID     string `json:"terminal_id"`
		MerchantID     string `json:"merchant_id"`
		BatchDate      string `json:"batch_date"`
		TerminalCount  int    `json:"terminal_count"`
		TerminalAmount int64  `json:"terminal_amount"`
		Currency       string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_BODY", "invalid request body"))
		return
	}

	cmd := port.RequestBatchCloseCommand{
		TerminalID:     req.TerminalID,
		MerchantID:     req.MerchantID,
		BatchDate:      req.BatchDate,
		TerminalCount:  req.TerminalCount,
		TerminalAmount: req.TerminalAmount,
		Currency:       req.Currency,
	}

	if err := h.sessionService.RequestBatchClose(ctx, cmd); err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "batch close failed"))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "batch_close_requested"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"error": code, "message": msg}
}
