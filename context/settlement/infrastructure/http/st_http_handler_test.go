package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/settlement/application/command"
	"github.com/juantevez/go-posnet/context/settlement/application/query"
	"github.com/juantevez/go-posnet/context/settlement/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func newTestHandler(batchRepo *fakeBatchRepo, pool *fakePool) *Handler {
	queryHandler := query.NewBatchQueryHandler(batchRepo)
	adminHandler := command.NewAdminHandler(batchRepo)
	return NewHandler(queryHandler, adminHandler, pool)
}

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
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, &fakePool{}))

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
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleReady_DatabaseUnavailable(t *testing.T) {
	pool := &fakePool{pingErr: errors.New("connection refused")}
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, pool))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// ─── GET /batches/{id} ────────────────────────────────────────────────────────

func TestHandleGetBatch_Success(t *testing.T) {
	b := newBatch(t)
	repo := &fakeBatchRepo{findByIDResult: b}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/batches/"+b.ID(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["ID"] != b.ID() {
		t.Errorf("ID = %v, want %q", body["ID"], b.ID())
	}
}

func TestHandleGetBatch_NotFound(t *testing.T) {
	repo := &fakeBatchRepo{findByIDResult: nil}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/batches/batch-999", nil)
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

func TestHandleGetBatch_InternalError(t *testing.T) {
	repo := &fakeBatchRepo{findByIDErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/batches/batch-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "db unreachable") {
		t.Errorf("internal error message leaked to client: %s", rec.Body.String())
	}
}

// ─── GET /merchants/{merchant_id}/batches ─────────────────────────────────────

func TestHandleListBatches_Success(t *testing.T) {
	b := newBatch(t)
	repo := &fakeBatchRepo{listResult: []*aggregate.SettlementBatch{b}}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	merchantID := domain.NewMerchantID()
	req := httptest.NewRequest(http.MethodGet, "/merchants/"+merchantID.String()+"/batches?date=2026-01-15", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if count, ok := body["count"].(float64); !ok || count != 1 {
		t.Errorf("body[count] = %v, want 1", body["count"])
	}
}

func TestHandleListBatches_ValidationError(t *testing.T) {
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/merchants/not-a-uuid/batches?date=2026-01-15", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "VALIDATION_ERROR" {
		t.Errorf("body[error] = %v, want %q", body["error"], "VALIDATION_ERROR")
	}
}

func TestHandleListBatches_InternalError(t *testing.T) {
	repo := &fakeBatchRepo{listErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/merchants/"+domain.NewMerchantID().String()+"/batches?date=2026-01-15", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── POST /batches/{id}/force-close ───────────────────────────────────────────

func TestHandleForceClose_Success(t *testing.T) {
	b := newBatch(t)
	repo := &fakeBatchRepo{findByIDResult: b}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/batches/"+b.ID()+"/force-close",
		strings.NewReader(`{"operator_id":"op-1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "closed" {
		t.Errorf("body[status] = %v, want %q", body["status"], "closed")
	}
}

func TestHandleForceClose_InvalidBody(t *testing.T) {
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/batches/batch-1/force-close", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "INVALID_BODY" {
		t.Errorf("body[error] = %v, want %q", body["error"], "INVALID_BODY")
	}
}

func TestHandleForceClose_NotFound(t *testing.T) {
	repo := &fakeBatchRepo{findByIDResult: nil}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/batches/batch-999/force-close",
		strings.NewReader(`{"operator_id":"op-1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleForceClose_ValidationError(t *testing.T) {
	mux := newTestMux(newTestHandler(&fakeBatchRepo{}, &fakePool{}))

	// operator_id vacío — el path {id} sigue teniendo un valor no vacío.
	req := httptest.NewRequest(http.MethodPost, "/batches/batch-1/force-close", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["error"] != "VALIDATION_ERROR" {
		t.Errorf("body[error] = %v, want %q", body["error"], "VALIDATION_ERROR")
	}
}

func TestHandleForceClose_InternalError(t *testing.T) {
	repo := &fakeBatchRepo{findByIDErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/batches/batch-1/force-close",
		strings.NewReader(`{"operator_id":"op-1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
