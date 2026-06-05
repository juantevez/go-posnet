// Package http contiene el adaptador HTTP del BC Notification.
package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/application/query"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// Handler contiene todos los handlers HTTP del BC Notification.
type Handler struct {
	queryHandler *query.NotificationQueryHandler
	adminHandler *command.AdminHandler
	pool         *pgxpool.Pool
}

func NewHandler(
	queryHandler *query.NotificationQueryHandler,
	adminHandler *command.AdminHandler,
	pool *pgxpool.Pool,
) *Handler {
	return &Handler{queryHandler: queryHandler, adminHandler: adminHandler, pool: pool}
}

// RegisterRoutes registra las rutas HTTP del BC.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET  /healthz", h.handleHealth)
	mux.HandleFunc("GET  /readyz", h.handleReady)
	mux.HandleFunc("GET  /notifications/{id}", h.handleGetNotification)
	mux.HandleFunc("GET  /transactions/{tx_id}/notifications", h.handleGetByTransaction)
	mux.HandleFunc("GET  /notifications/dead", h.handleListDead)
	mux.HandleFunc("POST /notifications/{id}/force-retry", h.handleForceRetry)
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

// GET /notifications/{id}
func (h *Handler) handleGetNotification(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetNotification")
	defer span.End()

	result, err := h.queryHandler.GetNotification(ctx, r.PathValue("id"))
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "notification not found"))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /transactions/{tx_id}/notifications
func (h *Handler) handleGetByTransaction(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetByTransaction")
	defer span.End()

	results, err := h.queryHandler.GetByTransactionID(ctx, r.PathValue("tx_id"))
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
	writeJSON(w, http.StatusOK, map[string]any{"notifications": results, "count": len(results)})
}

// GET /notifications/dead?limit=50
func (h *Handler) handleListDead(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ListDead")
	defer span.End()

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if i, err := strconv.Atoi(l); err == nil && i > 0 {
			limit = i
		}
	}

	results, err := h.queryHandler.ListDead(ctx, limit)
	if err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": results, "count": len(results)})
}

// POST /notifications/{id}/force-retry
func (h *Handler) handleForceRetry(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ForceRetry")
	defer span.End()

	if err := h.adminHandler.ForceRetry(ctx, r.PathValue("id")); err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "notification not found"))
			return
		}
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "retry_dispatched"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"error": code, "message": msg}
}
