package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/application/query"
	"github.com/juantevez/go-posnet/context/notification/domain/repository"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func newRouterFor(repo repository.NotificationRepository, pool *fakePool) http.Handler {
	queryHandler := query.NewNotificationQueryHandler(repo)
	notifyHandler := command.NewNotifyHandler(
		repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, "notification"), nil,
	)
	adminHandler := command.NewAdminHandler(repo, notifyHandler)
	return NewRouter(queryHandler, adminHandler, pool)
}

func TestNewRouter_Routes(t *testing.T) {
	txID := domain.NewTransactionID()
	repo := &fakeNotificationRepo{
		findByIDResult: newNotification(t, txID, valueobject.StatePending),
		findByTxResult: nil,
		findDeadResult: nil,
	}
	router := newRouterFor(repo, &fakePool{})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
		{"readyz", http.MethodGet, "/readyz", http.StatusOK},
		{"metrics", http.MethodGet, "/metrics", http.StatusOK},
		{"get notification", http.MethodGet, "/notifications/notif-1", http.StatusOK},
		{"get by transaction", http.MethodGet, "/transactions/" + txID.String() + "/notifications", http.StatusOK},
		{"list dead", http.MethodGet, "/notifications/dead", http.StatusOK},
		{"force retry", http.MethodPost, "/notifications/notif-1/force-retry", http.StatusAccepted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
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
	router := newRouterFor(&fakeNotificationRepo{}, pool)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestNewRouter_RecoversFromPanicInHandler(t *testing.T) {
	// panickyNotificationRepo panickea en FindByID en vez de devolver el error
	// normalmente — sirve para probar que recoverMiddleware protege incluso
	// detrás de la composición completa de middlewares.
	router := newRouterFor(&panickyNotificationRepo{}, &fakePool{})

	req := httptest.NewRequest(http.MethodGet, "/notifications/notif-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestNewRouter_UnknownRouteReturns404(t *testing.T) {
	router := newRouterFor(&fakeNotificationRepo{}, &fakePool{})

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
