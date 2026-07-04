package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/application/query"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func newTestHandler(caseRepo *fakeNotificationRepo, pool *fakePool) *Handler {
	queryHandler := query.NewNotificationQueryHandler(caseRepo)
	notifyHandler := command.NewNotifyHandler(
		caseRepo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, "notification"), nil,
	)
	adminHandler := command.NewAdminHandler(caseRepo, notifyHandler)
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
	mux := newTestMux(newTestHandler(&fakeNotificationRepo{}, &fakePool{}))

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
	mux := newTestMux(newTestHandler(&fakeNotificationRepo{}, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleReady_DatabaseUnavailable(t *testing.T) {
	pool := &fakePool{pingErr: errors.New("connection refused")}
	mux := newTestMux(newTestHandler(&fakeNotificationRepo{}, pool))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// ─── GET /notifications/{id} ──────────────────────────────────────────────────

func TestHandleGetNotification_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	repo := &fakeNotificationRepo{findByIDResult: newNotification(t, txID, valueobject.StatePending)}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/notif-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["ID"] != "notif-1" {
		t.Errorf("ID = %v, want %q", body["ID"], "notif-1")
	}
}

func TestHandleGetNotification_NotFound(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDResult: nil, findByIDErr: nil}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/notif-999", nil)
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

func TestHandleGetNotification_InternalError(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/notif-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "db unreachable") {
		t.Errorf("internal error message leaked to client: %s", rec.Body.String())
	}
}

// ─── GET /transactions/{tx_id}/notifications ─────────────────────────────────

func TestHandleGetByTransaction_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	repo := &fakeNotificationRepo{findByTxResult: []*aggregate.Notification{newNotification(t, txID, valueobject.StatePending)}}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/transactions/"+txID.String()+"/notifications", nil)
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

func TestHandleGetByTransaction_ValidationError(t *testing.T) {
	mux := newTestMux(newTestHandler(&fakeNotificationRepo{}, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/transactions/not-a-uuid/notifications", nil)
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

func TestHandleGetByTransaction_InternalError(t *testing.T) {
	repo := &fakeNotificationRepo{findByTxErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/transactions/"+domain.NewTransactionID().String()+"/notifications", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── GET /notifications/dead ──────────────────────────────────────────────────

func TestHandleListDead_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	repo := &fakeNotificationRepo{findDeadResult: []*aggregate.Notification{newNotification(t, txID, valueobject.StateDead)}}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/dead", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if count, ok := body["count"].(float64); !ok || count != 1 {
		t.Errorf("body[count] = %v, want 1", body["count"])
	}
	if repo.lastFindDeadLimit != 50 {
		t.Errorf("lastFindDeadLimit = %d, want 50 (default)", repo.lastFindDeadLimit)
	}
}

func TestHandleListDead_CustomLimit(t *testing.T) {
	repo := &fakeNotificationRepo{}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/dead?limit=10", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if repo.lastFindDeadLimit != 10 {
		t.Errorf("lastFindDeadLimit = %d, want 10", repo.lastFindDeadLimit)
	}
}

func TestHandleListDead_InvalidLimitFallsBackToDefault(t *testing.T) {
	tests := []string{"abc", "-5", "0"}
	for _, l := range tests {
		t.Run(l, func(t *testing.T) {
			repo := &fakeNotificationRepo{}
			mux := newTestMux(newTestHandler(repo, &fakePool{}))

			req := httptest.NewRequest(http.MethodGet, "/notifications/dead?limit="+l, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if repo.lastFindDeadLimit != 50 {
				t.Errorf("lastFindDeadLimit = %d, want 50 (default fallback)", repo.lastFindDeadLimit)
			}
		})
	}
}

func TestHandleListDead_Error(t *testing.T) {
	repo := &fakeNotificationRepo{findDeadErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodGet, "/notifications/dead", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── POST /notifications/{id}/force-retry ────────────────────────────────────

func TestHandleForceRetry_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	repo := &fakeNotificationRepo{findByIDResult: newNotification(t, txID, valueobject.StateRetrying)}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/notifications/notif-1/force-retry", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	body := decodeJSON(t, rec.Body.Bytes())
	if body["status"] != "retry_dispatched" {
		t.Errorf("body[status] = %v, want %q", body["status"], "retry_dispatched")
	}
}

func TestHandleForceRetry_NotFound(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDResult: nil}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/notifications/notif-999/force-retry", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleForceRetry_InternalError(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDErr: errors.New("db unreachable")}
	mux := newTestMux(newTestHandler(repo, &fakePool{}))

	req := httptest.NewRequest(http.MethodPost, "/notifications/notif-1/force-retry", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
