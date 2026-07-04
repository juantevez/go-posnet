package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/settlement/application/command"
	"github.com/juantevez/go-posnet/context/settlement/application/query"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
)

func newRouterFor(repo repository.SettlementBatchRepository, pool *fakePool) http.Handler {
	queryHandler := query.NewBatchQueryHandler(repo)
	adminHandler := command.NewAdminHandler(repo)
	return NewRouter(queryHandler, adminHandler, pool)
}

func TestNewRouter_Routes(t *testing.T) {
	b := newBatch(t)
	repo := &fakeBatchRepo{
		findByIDResult: b,
		listResult:     nil,
	}
	router := newRouterFor(repo, &fakePool{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"healthz", http.MethodGet, "/healthz", "", http.StatusOK},
		{"readyz", http.MethodGet, "/readyz", "", http.StatusOK},
		{"metrics", http.MethodGet, "/metrics", "", http.StatusOK},
		{"get batch", http.MethodGet, "/batches/" + b.ID(), "", http.StatusOK},
		{"list batches", http.MethodGet, "/merchants/" + b.MerchantID().String() + "/batches?date=2026-01-15", "", http.StatusOK},
		{"force close", http.MethodPost, "/batches/" + b.ID() + "/force-close", `{"operator_id":"op-1"}`, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestNewRouter_ReadyzReflectsPoolFailure(t *testing.T) {
	pool := &fakePool{pingErr: errors.New("connection refused")}
	router := newRouterFor(&fakeBatchRepo{}, pool)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestNewRouter_RecoversFromPanicInHandler(t *testing.T) {
	// panickyBatchRepo simula un bug en la capa de aplicación en vez de
	// devolver el error normalmente — sirve para probar que recoverMiddleware
	// protege incluso detrás de la composición completa de middlewares.
	router := newRouterFor(&panickyBatchRepo{}, &fakePool{})

	req := httptest.NewRequest(http.MethodGet, "/batches/batch-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestNewRouter_UnknownRouteReturns404(t *testing.T) {
	router := newRouterFor(&fakeBatchRepo{}, &fakePool{})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMetricsHandler(t *testing.T) {
	handler := metricsHandler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Prometheus metrics endpoint") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "Prometheus metrics endpoint")
	}
}
