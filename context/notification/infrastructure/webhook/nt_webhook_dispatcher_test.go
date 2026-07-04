package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func mustReceipt(t *testing.T) valueobject.ReceiptPayload {
	t.Helper()
	r, err := valueobject.NewReceiptPayload(
		domain.NewTransactionID().String(), "Merchant Inc", "TERM-001",
		"APPROVED", 5000, "ARS", "1234", "VISA", "CHIP",
		"2026-01-01T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("NewReceiptPayload() error = %v", err)
	}
	return r
}

func newTestNotification(t *testing.T) *aggregate.Notification {
	t.Helper()
	n, err := aggregate.NewNotification(domain.NewTransactionID(), domain.NewMerchantID(), valueobject.ChannelWebhook, mustReceipt(t))
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}
	return n
}

func TestDispatch_Success(t *testing.T) {
	var gotMethod, gotContentType, gotNotifHeader, gotTxHeader string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotNotifHeader = r.Header.Get("X-Posnet-Notification-ID")
		gotTxHeader = r.Header.Get("X-Posnet-Transaction-ID")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(2*time.Second, srv.URL)
	n := newTestNotification(t)

	status, err := d.Dispatch(context.Background(), n)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
	if gotNotifHeader != n.ID() {
		t.Errorf("X-Posnet-Notification-ID = %q, want %q", gotNotifHeader, n.ID())
	}
	if gotTxHeader != n.TransactionID().String() {
		t.Errorf("X-Posnet-Transaction-ID = %q, want %q", gotTxHeader, n.TransactionID().String())
	}
	if gotBody["notification_id"] != n.ID() {
		t.Errorf("body[notification_id] = %v, want %q", gotBody["notification_id"], n.ID())
	}
	if gotBody["transaction_id"] != n.TransactionID().String() {
		t.Errorf("body[transaction_id] = %v, want %q", gotBody["transaction_id"], n.TransactionID().String())
	}
	if gotBody["channel"] != n.Channel().String() {
		t.Errorf("body[channel] = %v, want %q", gotBody["channel"], n.Channel().String())
	}
	if gotBody["receipt"] == nil {
		t.Error("body[receipt] is nil, want the receipt payload")
	}
}

func TestDispatch_RelaysNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDispatcher(2*time.Second, srv.URL)
	status, err := d.Dispatch(context.Background(), newTestNotification(t))
	if err != nil {
		t.Fatalf("Dispatch() error = %v, want nil (el status no-2xx no es un error de red)", err)
	}
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
}

func TestDispatch_NoEndpointConfigured(t *testing.T) {
	d := NewDispatcher(2*time.Second, "")
	n := newTestNotification(t)

	status, err := d.Dispatch(context.Background(), n)
	if err == nil || !strings.Contains(err.Error(), "no endpoint configured") {
		t.Fatalf("error = %v, want it to contain %q", err, "no endpoint configured")
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
}

func TestDispatch_RequestBuildError(t *testing.T) {
	d := NewDispatcher(2*time.Second, "://bad-url")
	_, err := d.Dispatch(context.Background(), newTestNotification(t))
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("error = %v, want it to contain %q", err, "build request")
	}
}

func TestDispatch_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := srv.URL
	srv.Close() // el puerto queda cerrado — cualquier request debe fallar por conexión rechazada

	d := NewDispatcher(2*time.Second, endpoint)
	_, err := d.Dispatch(context.Background(), newTestNotification(t))
	if err == nil || !strings.Contains(err.Error(), "send request to") {
		t.Fatalf("error = %v, want it to contain %q", err, "send request to")
	}
}

func TestDispatch_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(10*time.Millisecond, srv.URL)
	_, err := d.Dispatch(context.Background(), newTestNotification(t))
	if err == nil || !strings.Contains(err.Error(), "send request to") {
		t.Fatalf("error = %v, want it to contain %q", err, "send request to")
	}
}

func TestResolveEndpoint(t *testing.T) {
	d := NewDispatcher(time.Second, "https://merchant.example.com/webhook")
	got := d.resolveEndpoint(newTestNotification(t))
	if got != "https://merchant.example.com/webhook" {
		t.Errorf("resolveEndpoint() = %q, want %q", got, "https://merchant.example.com/webhook")
	}
}
