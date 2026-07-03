package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/query"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func newTestMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func decodeJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", body, err)
	}
	return m
}

// ─── healthz / readyz ─────────────────────────────────────────────────────────

func TestHandleHealth(t *testing.T) {
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "ok" {
		t.Errorf("body[status] = %v, want %q", body["status"], "ok")
	}
}

func TestHandleReady_Success(t *testing.T) {
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "ready" {
		t.Errorf("body[status] = %v, want %q", body["status"], "ready")
	}
}

func TestHandleReady_DatabaseUnavailable(t *testing.T) {
	pool := &fakePool{pingErr: errors.New("connection refused")}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, &fakeFraudCaseRepo{}), pool)
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "not ready" || body["reason"] != "database unavailable" {
		t.Errorf("body = %+v, want status=not ready reason=database unavailable", body)
	}
}

// ─── GET /fraud-cases/{transaction_id} ────────────────────────────────────────

func TestHandleGetFraudCase_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	caseRepo := &fakeFraudCaseRepo{findResult: evaluatedFraudCase(t, txID)}
	h := NewHandler(query.NewFraudQueryHandler(caseRepo, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, caseRepo), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/fraud-cases/"+txID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result port.FraudCaseResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if result.TransactionID != txID.String() {
		t.Errorf("TransactionID = %q, want %q", result.TransactionID, txID.String())
	}
	if result.Score != 30 {
		t.Errorf("Score = %d, want 30", result.Score)
	}
	if len(result.Evaluations) != 1 || result.Evaluations[0].RuleID != "RULE-001" {
		t.Errorf("Evaluations = %v, want 1 item with RuleID=RULE-001", result.Evaluations)
	}
}

func TestHandleGetFraudCase_InvalidTransactionID(t *testing.T) {
	caseRepo := &fakeFraudCaseRepo{}
	h := NewHandler(query.NewFraudQueryHandler(caseRepo, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, caseRepo), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/fraud-cases/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "INVALID_ID" {
		t.Errorf("body[error] = %v, want %q", body["error"], "INVALID_ID")
	}
}

func TestHandleGetFraudCase_NotFound(t *testing.T) {
	caseRepo := &fakeFraudCaseRepo{findResult: nil, findErr: nil}
	h := NewHandler(query.NewFraudQueryHandler(caseRepo, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, caseRepo), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/fraud-cases/"+domain.NewTransactionID().String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "NOT_FOUND" {
		t.Errorf("body[error] = %v, want %q", body["error"], "NOT_FOUND")
	}
}

func TestHandleGetFraudCase_InternalError(t *testing.T) {
	caseRepo := &fakeFraudCaseRepo{findErr: errors.New("db unreachable")}
	h := NewHandler(query.NewFraudQueryHandler(caseRepo, &fakeRuleRepo{}), command.NewAdminHandler(&fakeRuleRepo{}, caseRepo), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/fraud-cases/"+domain.NewTransactionID().String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "INTERNAL" {
		t.Errorf("body[error] = %v, want %q", body["error"], "INTERNAL")
	}
	if strings.Contains(rec.Body.String(), "db unreachable") {
		t.Errorf("internal error message leaked to client: %s", rec.Body.String())
	}
}

// ─── GET /rules ────────────────────────────────────────────────────────────────

func TestHandleListRules_Success(t *testing.T) {
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, 10, "RULE-001")}}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/rules", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if count, ok := body["count"].(float64); !ok || count != 1 {
		t.Errorf("body[count] = %v, want 1", body["count"])
	}
}

func TestHandleListRules_Error(t *testing.T) {
	ruleRepo := &fakeRuleRepo{findErr: errors.New("db unreachable")}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	req := httptest.NewRequest(http.MethodGet, "/rules", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── PUT /rules/{rule_id}/threshold ───────────────────────────────────────────

func putThresholdRequest(t *testing.T, ruleID string, body string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPut, "/rules/"+ruleID+"/threshold", bytes.NewBufferString(body))
}

func TestHandleUpdateRuleThreshold_InvalidBody(t *testing.T) {
	ruleRepo := &fakeRuleRepo{}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putThresholdRequest(t, "RULE-001", "not-json"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "INVALID_BODY" {
		t.Errorf("body[error] = %v, want %q", body["error"], "INVALID_BODY")
	}
}

func TestHandleUpdateRuleThreshold_NotFound(t *testing.T) {
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, 10, "RULE-001")}}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putThresholdRequest(t, "RULE-999", `{"new_threshold":1.5,"new_score_weight":50}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "NOT_FOUND" {
		t.Errorf("body[error] = %v, want %q", body["error"], "NOT_FOUND")
	}
}

func TestHandleUpdateRuleThreshold_ValidationError(t *testing.T) {
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, 10, "RULE-001")}}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	// new_score_weight = 0 está fuera de rango [1, 100] → ValidationError.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putThresholdRequest(t, "RULE-001", `{"new_threshold":1.5,"new_score_weight":0}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "VALIDATION_ERROR" {
		t.Errorf("body[error] = %v, want %q", body["error"], "VALIDATION_ERROR")
	}
}

func TestHandleUpdateRuleThreshold_Success(t *testing.T) {
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, 10, "RULE-001")}}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putThresholdRequest(t, "RULE-001", `{"new_threshold":1.5,"new_score_weight":50}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "updated" {
		t.Errorf("body[status] = %v, want %q", body["status"], "updated")
	}
	if len(ruleRepo.savedRules) != 1 {
		t.Errorf("saved rules = %d, want 1", len(ruleRepo.savedRules))
	}
}

func TestHandleUpdateRuleThreshold_InternalError(t *testing.T) {
	ruleRepo := &fakeRuleRepo{findErr: errors.New("db unreachable")}
	h := NewHandler(query.NewFraudQueryHandler(&fakeFraudCaseRepo{}, ruleRepo), command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{}), &fakePool{})
	mux := newTestMux(h)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, putThresholdRequest(t, "RULE-001", `{"new_threshold":1.5,"new_score_weight":50}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
