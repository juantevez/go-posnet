// Package http contiene el adaptador HTTP del BC Fraud Detection.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/query"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// Handler contiene todos los handlers HTTP del BC Fraud Detection.
type Handler struct {
	queryHandler *query.FraudQueryHandler
	adminHandler *command.AdminHandler
	pool         *pgxpool.Pool
}

func NewHandler(
	queryHandler *query.FraudQueryHandler,
	adminHandler *command.AdminHandler,
	pool *pgxpool.Pool,
) *Handler {
	return &Handler{
		queryHandler: queryHandler,
		adminHandler: adminHandler,
		pool:         pool,
	}
}

// RegisterRoutes registra las rutas HTTP del BC.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET  /healthz", h.handleHealth)
	mux.HandleFunc("GET  /readyz", h.handleReady)
	mux.HandleFunc("GET  /fraud-cases/{transaction_id}", h.handleGetFraudCase)
	mux.HandleFunc("GET  /rules", h.handleListRules)
	mux.HandleFunc("PUT  /rules/{rule_id}/threshold", h.handleUpdateRuleThreshold)
}

// GET /healthz
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /readyz
func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := pgutil.HealthCheck(r.Context(), h.pool); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready", "reason": "database unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// GET /fraud-cases/{transaction_id} — análisis de fraude de una transacción
func (h *Handler) handleGetFraudCase(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.GetFraudCase")
	defer span.End()

	txID := r.PathValue("transaction_id")
	result, err := h.queryHandler.GetFraudCase(ctx, txID)
	if err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "fraud case not found"))
			return
		}
		var validationErr *pkgerrors.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errResp("INVALID_ID", err.Error()))
			return
		}

		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GET /rules — lista todas las reglas activas con sus parámetros
func (h *Handler) handleListRules(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.ListRules")
	defer span.End()

	rules, err := h.queryHandler.ListActiveRules(ctx)
	if err != nil {
		observability.RecordError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, errResp("INTERNAL", "internal error"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "count": len(rules)})
}

// PUT /rules/{rule_id}/threshold — actualiza el umbral de una regla
func (h *Handler) handleUpdateRuleThreshold(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.StartSpan(r.Context(), "http.UpdateRuleThreshold")
	defer span.End()

	ruleID := r.PathValue("rule_id")

	var body struct {
		NewThreshold   float64 `json:"new_threshold"`
		NewScoreWeight int     `json:"new_score_weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("INVALID_BODY", "invalid request body"))
		return
	}

	cmd := port.UpdateRuleThresholdCommand{
		RuleID:         ruleID,
		NewThreshold:   body.NewThreshold,
		NewScoreWeight: body.NewScoreWeight,
	}

	if err := h.adminHandler.UpdateRuleThreshold(ctx, cmd); err != nil {
		var notFound *pkgerrors.NotFoundError
		if errors.As(err, &notFound) {
			writeJSON(w, http.StatusNotFound, errResp("NOT_FOUND", "rule not found"))
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

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"error": code, "message": msg}
}
