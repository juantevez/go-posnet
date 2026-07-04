package entity_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/entity"
)

func TestNewDeliveryAttempt_Success(t *testing.T) {
	before := time.Now().UTC()
	a, err := entity.NewDeliveryAttempt("notif-1", 2, true, 200, "")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewDeliveryAttempt() error = %v", err)
	}

	wantID := fmt.Sprintf("%s-%d", "notif-1", 2)
	if a.ID() != wantID {
		t.Errorf("ID() = %q, want %q", a.ID(), wantID)
	}
	if a.NotificationID() != "notif-1" {
		t.Errorf("NotificationID() = %q, want %q", a.NotificationID(), "notif-1")
	}
	if a.AttemptNumber() != 2 {
		t.Errorf("AttemptNumber() = %d, want 2", a.AttemptNumber())
	}
	if !a.Success() {
		t.Error("Success() = false, want true")
	}
	if a.HTTPStatus() != 200 {
		t.Errorf("HTTPStatus() = %d, want 200", a.HTTPStatus())
	}
	if a.ErrorMessage() != "" {
		t.Errorf("ErrorMessage() = %q, want empty", a.ErrorMessage())
	}
	if a.AttemptedAt().Before(before) || a.AttemptedAt().After(after) {
		t.Errorf("AttemptedAt() = %v, want between %v and %v", a.AttemptedAt(), before, after)
	}
}

func TestNewDeliveryAttempt_FailedAttempt(t *testing.T) {
	a, err := entity.NewDeliveryAttempt("notif-1", 1, false, 500, "connection timeout")
	if err != nil {
		t.Fatalf("NewDeliveryAttempt() error = %v", err)
	}
	if a.Success() {
		t.Error("Success() = true, want false")
	}
	if a.HTTPStatus() != 500 {
		t.Errorf("HTTPStatus() = %d, want 500", a.HTTPStatus())
	}
	if a.ErrorMessage() != "connection timeout" {
		t.Errorf("ErrorMessage() = %q, want %q", a.ErrorMessage(), "connection timeout")
	}
}

func TestNewDeliveryAttempt_ValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		notificationID string
		attemptNumber  int
		wantErr        string
	}{
		{"empty notification id", "", 1, "notification_id cannot be empty"},
		{"zero attempt number", "notif-1", 0, "attempt_number must be >= 1"},
		{"negative attempt number", "notif-1", -1, "attempt_number must be >= 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := entity.NewDeliveryAttempt(tc.notificationID, tc.attemptNumber, true, 200, "")
			if err == nil {
				t.Fatalf("NewDeliveryAttempt() error = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
			if a != nil {
				t.Errorf("NewDeliveryAttempt() = %v, want nil", a)
			}
		})
	}
}

func TestNewDeliveryAttempt_BoundaryAttemptNumber(t *testing.T) {
	a, err := entity.NewDeliveryAttempt("notif-1", 1, true, 200, "")
	if err != nil {
		t.Fatalf("NewDeliveryAttempt(..., 1, ...) error = %v, want nil", err)
	}
	if a.AttemptNumber() != 1 {
		t.Errorf("AttemptNumber() = %d, want 1", a.AttemptNumber())
	}
}

func TestReconstituteDeliveryAttempt(t *testing.T) {
	attemptedAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a := entity.ReconstituteDeliveryAttempt("custom-id", "notif-1", 3, false, 503, "service unavailable", attemptedAt)

	if a.ID() != "custom-id" {
		t.Errorf("ID() = %q, want %q", a.ID(), "custom-id")
	}
	if a.NotificationID() != "notif-1" {
		t.Errorf("NotificationID() = %q, want %q", a.NotificationID(), "notif-1")
	}
	if a.AttemptNumber() != 3 {
		t.Errorf("AttemptNumber() = %d, want 3", a.AttemptNumber())
	}
	if a.Success() {
		t.Error("Success() = true, want false")
	}
	if a.HTTPStatus() != 503 {
		t.Errorf("HTTPStatus() = %d, want 503", a.HTTPStatus())
	}
	if a.ErrorMessage() != "service unavailable" {
		t.Errorf("ErrorMessage() = %q, want %q", a.ErrorMessage(), "service unavailable")
	}
	if !a.AttemptedAt().Equal(attemptedAt) {
		t.Errorf("AttemptedAt() = %v, want %v", a.AttemptedAt(), attemptedAt)
	}
}
