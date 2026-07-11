package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/query"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/pkg/domain"
	"github.com/juantevez/go-posnet/pkg/observability"
)

func TestNewRouter_Routes(t *testing.T) {
	caseRepo := &fakeFraudCaseRepo{findResult: evaluatedFraudCase(t, domain.NewTransactionID())}
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, 10, "RULE-001")}}
	queryHandler := query.NewFraudQueryHandler(caseRepo, ruleRepo)
	adminHandler := command.NewAdminHandler(ruleRepo, caseRepo)

	router := NewRouter(queryHandler, adminHandler, &fakePool{})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
		{"readyz", http.MethodGet, "/readyz", http.StatusOK},
		{"metrics", http.MethodGet, "/metrics", http.StatusOK},
		{"fraud case", http.MethodGet, "/fraud-cases/" + domain.NewTransactionID().String(), http.StatusOK},
		{"rules", http.MethodGet, "/rules", http.StatusOK},
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
	caseRepo := &fakeFraudCaseRepo{}
	ruleRepo := &fakeRuleRepo{}
	pool := &fakePool{pingErr: errors.New("connection refused")}
	router := NewRouter(query.NewFraudQueryHandler(caseRepo, ruleRepo), command.NewAdminHandler(ruleRepo, caseRepo), pool)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestNewRouter_RecoversFromPanicInHandler(t *testing.T) {
	// panickyFraudCaseRepo panickea en FindByTransactionID en vez de devolver
	// el error normalmente — sirve para probar que recoverMiddleware protege
	// incluso detrás de la composición completa de middlewares.
	panicRepo := &panickyFraudCaseRepo{}
	ruleRepo := &fakeRuleRepo{}
	router := NewRouter(query.NewFraudQueryHandler(panicRepo, ruleRepo), command.NewAdminHandler(ruleRepo, panicRepo), &fakePool{})

	req := httptest.NewRequest(http.MethodGet, "/fraud-cases/"+domain.NewTransactionID().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestNewRouter_UnknownRouteReturns404(t *testing.T) {
	caseRepo := &fakeFraudCaseRepo{}
	ruleRepo := &fakeRuleRepo{}
	router := NewRouter(query.NewFraudQueryHandler(caseRepo, ruleRepo), command.NewAdminHandler(ruleRepo, caseRepo), &fakePool{})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMetricsHandler(t *testing.T) {
	handler := observability.MetricsHandler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Errorf("body should expose Go runtime metrics, got %q", rec.Body.String())
	}
}
