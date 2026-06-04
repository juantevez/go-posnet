// Package http contiene el adaptador HTTP del BC Authorization.
// Expone únicamente endpoints de operación: health check y status de transacciones.
// No es la interfaz principal — esa es NATS.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tu-org/posnet-backend/pkg/domain"
	pkgerrors "github.com/tu-org/posnet-backend/pkg/errors"
	"github.com/tu-org/posnet-backend/pkg/observability"
	"github.com/tu-org/posnet-backend/pkg/pgutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tu-org/posnet-backend/context/authorization/application/port"
	"errors"
)

// Handler contiene todos los handlers HTTP del BC.
type Handler struct {
	queryService port.QueryService
	pool         *pgxpool.Pool
}

func NewHandler(queryService port.QueryService, pool *pgxpool.Pool) *Handler {
	return &Handler{queryService: queryService, pool: pool}
}

// RegisterRoutes registra todas las rutas HTTP.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz",                    h.handleHealth)
	mux.HandleFunc("GET /readyz",                     h.handleReady)
	mux.HandleFunc("GET /transactions/{id}",          h.handleGetTransaction)
}

// handleHealth — liveness probe: el proceso está vivo.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady — readiness probe: el servicio puede recibir tráfico.
func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := pgutil.HealthCheck(r.Context(), h.pool); err != nil {
		slog.Error("readiness check failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "database unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleGetTransaction — retorna el estado de una transacción por ID.
// Uso: operaciones, soporte, debugging. No está en el critical path.
func (h *Handler) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetTransaction")
	defer span.End()

	idStr := r.PathValue("id")
	txID, err := domain.ParseTransactionID(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse("INVALID_ID", "invalid transaction id format"))
		return
	}

	result, err := h.queryService.GetTransactionStatus(ctx, txID)
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errorResponse("NOT_FOUND", "transaction not found"))
			return
		}
		observability.RecordError(ctx, err)
		slog.ErrorContext(ctx, "get transaction status failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, errorResponse("INTERNAL", "internal error"))
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorResponse(code, message string) map[string]string {
	return map[string]string{"error": code, "message": message}
}
