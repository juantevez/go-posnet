// Package http contiene el adaptador HTTP del BC Settlement.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/juantevez/go-posnet/context/settlement/application/command"
	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/application/query"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// Handler contiene todos los handlers HTTP del BC Settlement.
type Handler struct {
	queryHandler *query.BatchQueryHandler
	adminHandler *command.AdminHandler
	pool         pgutil.PgxPool
}

func NewHandler(
	queryHandler *query.BatchQueryHandler,
	adminHandler *command.AdminHandler,
	pool pgutil.PgxPool,
) *Handler {
	return &Handler{queryHandler: queryHandler, adminHandler: adminHandler, pool: pool}
}

// RegisterRoutes registra las rutas HTTP del BC.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET  /healthz", h.handleHealth)
	mux.HandleFunc("GET  /readyz", h.handleReady)
	mux.HandleFunc("GET  /batches/{id}", h.handleGetBatch)
	mux.HandleFunc("GET  /merchants/{merchant_id}/batches", h.handleListBatches)
	mux.HandleFunc("POST /batches/{id}/force-close", h.handleForceClose)
	mux.HandleFunc("POST /batches/{id}/resubmit", h.handleResubmitBatch)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := pgutil.HealthCheck(r.Context(), h.pool); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready", "reason": "database unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// GET /batches/{id}
func (h *Handler) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetBatch")
	defer span.End()

	result, err := h.queryHandler.GetBatch(ctx, r.PathValue("id"))
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "batch not found"))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /merchants/{merchant_id}/batches?date=2025-06-04
func (h *Handler) handleListBatches(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ListBatches")
	defer span.End()

	cmd := port.ListBatchesCommand{
		MerchantID: r.PathValue("merchant_id"),
		Date:       r.URL.Query().Get("date"),
	}

	results, err := h.queryHandler.ListBatchesByMerchant(ctx, cmd)
	if err != nil {
		var validationErr *pkgerrors.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": results, "count": len(results)})
}

// POST /batches/{id}/force-close
func (h *Handler) handleForceClose(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ForceClose")
	defer span.End()

	var body struct {
		OperatorID string `json:"operator_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_BODY", "invalid request body"))
		return
	}

	cmd := port.ForceCloseCommand{
		BatchID:    r.PathValue("id"),
		OperatorID: body.OperatorID,
	}

	if err := h.adminHandler.ForceClose(ctx, cmd); err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "batch not found"))
			return
		}
		var validationErr *pkgerrors.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// POST /batches/{id}/resubmit
func (h *Handler) handleResubmitBatch(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ResubmitBatch")
	defer span.End()

	var body struct {
		OperatorID string `json:"operator_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_BODY", "invalid request body"))
		return
	}

	cmd := port.ResubmitBatchCommand{
		BatchID:    r.PathValue("id"),
		OperatorID: body.OperatorID,
	}

	if err := h.adminHandler.ResubmitBatch(ctx, cmd); err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "batch not found"))
			return
		}
		var validationErr *pkgerrors.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "submitted"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"error": code, "message": msg}
}
