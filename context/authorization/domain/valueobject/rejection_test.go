package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
)

func TestNewRejectionFromISO(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rc, err := valueobject.NewRejectionFromISO(valueobject.ISO_DO_NOT_HONOR)
		if err != nil {
			t.Fatalf("NewRejectionFromISO() error = %v", err)
		}
		if rc.Code() != valueobject.ISO_DO_NOT_HONOR {
			t.Errorf("Code() = %q, want %q", rc.Code(), valueobject.ISO_DO_NOT_HONOR)
		}
		if rc.Source() != valueobject.SourceAcquirer {
			t.Errorf("Source() = %v, want %v", rc.Source(), valueobject.SourceAcquirer)
		}
	})

	t.Run("empty code returns error", func(t *testing.T) {
		rc, err := valueobject.NewRejectionFromISO("")
		if err == nil {
			t.Fatal("NewRejectionFromISO(\"\") error = nil, want error")
		}
		if !strings.Contains(err.Error(), "iso code cannot be empty") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "iso code cannot be empty")
		}
		if rc.Code() != "" || rc.Source() != "" {
			t.Errorf("RejectionCode = %v, want zero value", rc)
		}
	})
}

func TestNewRejectionFromFraud(t *testing.T) {
	rc := valueobject.NewRejectionFromFraud()
	if rc.Code() != "FRAUD_REJECTED" {
		t.Errorf("Code() = %q, want %q", rc.Code(), "FRAUD_REJECTED")
	}
	if rc.Source() != valueobject.SourceFraud {
		t.Errorf("Source() = %v, want %v", rc.Source(), valueobject.SourceFraud)
	}
}

func TestNewRejectionFromTimeout(t *testing.T) {
	rc := valueobject.NewRejectionFromTimeout()
	if rc.Code() != "TIMEOUT" {
		t.Errorf("Code() = %q, want %q", rc.Code(), "TIMEOUT")
	}
	if rc.Source() != valueobject.SourceTimeout {
		t.Errorf("Source() = %v, want %v", rc.Source(), valueobject.SourceTimeout)
	}
}

func TestNewRejectionFromValidation(t *testing.T) {
	rc := valueobject.NewRejectionFromValidation("invalid pan")
	if rc.Code() != "invalid pan" {
		t.Errorf("Code() = %q, want %q", rc.Code(), "invalid pan")
	}
	if rc.Source() != valueobject.SourceValidation {
		t.Errorf("Source() = %v, want %v", rc.Source(), valueobject.SourceValidation)
	}
}

func TestRejectionCode_Description(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{valueobject.ISO_DO_NOT_HONOR, "Do Not Honor"},
		{valueobject.ISO_INVALID_TRANSACTION, "Invalid Transaction"},
		{valueobject.ISO_INVALID_AMOUNT, "Invalid Amount"},
		{valueobject.ISO_CARD_NOT_FOUND, "Card Not Found"},
		{valueobject.ISO_FORMAT_ERROR, "Format Error"},
		{valueobject.ISO_INSUFFICIENT_FUNDS, "Insufficient Funds"},
		{valueobject.ISO_EXPIRED_CARD, "Expired Card"},
		{valueobject.ISO_INCORRECT_PIN, "Incorrect PIN"},
		{valueobject.ISO_NOT_PERMITTED, "Transaction Not Permitted"},
		{valueobject.ISO_SUSPECTED_FRAUD, "Suspected Fraud"},
		{valueobject.ISO_EXCEEDS_LIMIT, "Exceeds Withdrawal Limit"},
		{valueobject.ISO_RESTRICTED_CARD, "Restricted Card"},
		{valueobject.ISO_SECURITY_VIOLATION, "Security Violation"},
		{valueobject.ISO_ISSUER_UNAVAILABLE, "Issuer Unavailable"},
		{valueobject.ISO_SYSTEM_MALFUNCTION, "System Malfunction"},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			rc, err := valueobject.NewRejectionFromISO(tc.code)
			if err != nil {
				t.Fatalf("NewRejectionFromISO(%q) error = %v", tc.code, err)
			}
			if got := rc.Description(); got != tc.want {
				t.Errorf("Description() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("unmapped code falls back to raw code", func(t *testing.T) {
		rc, err := valueobject.NewRejectionFromISO(valueobject.ISO_APPROVED)
		if err != nil {
			t.Fatalf("NewRejectionFromISO() error = %v", err)
		}
		want := "Rejection code: " + valueobject.ISO_APPROVED
		if got := rc.Description(); got != want {
			t.Errorf("Description() = %q, want %q", got, want)
		}
	})
}

func TestRejectionCode_IsRetryable(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{valueobject.ISO_ISSUER_UNAVAILABLE, true},
		{valueobject.ISO_SYSTEM_MALFUNCTION, true},
		{valueobject.ISO_DO_NOT_HONOR, false},
		{valueobject.ISO_INSUFFICIENT_FUNDS, false},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			rc, err := valueobject.NewRejectionFromISO(tc.code)
			if err != nil {
				t.Fatalf("NewRejectionFromISO(%q) error = %v", tc.code, err)
			}
			if got := rc.IsRetryable(); got != tc.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRejectionCode_String(t *testing.T) {
	rc, err := valueobject.NewRejectionFromISO(valueobject.ISO_DO_NOT_HONOR)
	if err != nil {
		t.Fatalf("NewRejectionFromISO() error = %v", err)
	}
	want := "ACQUIRER(" + valueobject.ISO_DO_NOT_HONOR + ")"
	if got := rc.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestNewFraudDecision(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fd, err := valueobject.NewFraudDecision(75, valueobject.FraudDecisionReview, []string{"R1", "R2"})
		if err != nil {
			t.Fatalf("NewFraudDecision() error = %v", err)
		}
		if fd.Score != 75 {
			t.Errorf("Score = %d, want 75", fd.Score)
		}
		if fd.Decision != valueobject.FraudDecisionReview {
			t.Errorf("Decision = %q, want %q", fd.Decision, valueobject.FraudDecisionReview)
		}
		if len(fd.RulesHit) != 2 {
			t.Errorf("RulesHit = %v, want 2 items", fd.RulesHit)
		}
	})

	t.Run("score below range", func(t *testing.T) {
		_, err := valueobject.NewFraudDecision(-1, valueobject.FraudDecisionApprove, nil)
		if err == nil {
			t.Fatal("NewFraudDecision(-1, ...) error = nil, want error")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "out of range")
		}
	})

	t.Run("score above range", func(t *testing.T) {
		_, err := valueobject.NewFraudDecision(101, valueobject.FraudDecisionApprove, nil)
		if err == nil {
			t.Fatal("NewFraudDecision(101, ...) error = nil, want error")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "out of range")
		}
	})

	t.Run("unknown decision", func(t *testing.T) {
		_, err := valueobject.NewFraudDecision(50, "MAYBE", nil)
		if err == nil {
			t.Fatal("NewFraudDecision(50, \"MAYBE\", nil) error = nil, want error")
		}
		if !strings.Contains(err.Error(), "unknown decision") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown decision")
		}
	})

	t.Run("boundary scores are valid", func(t *testing.T) {
		if _, err := valueobject.NewFraudDecision(0, valueobject.FraudDecisionApprove, nil); err != nil {
			t.Errorf("NewFraudDecision(0, ...) error = %v, want nil", err)
		}
		if _, err := valueobject.NewFraudDecision(100, valueobject.FraudDecisionReject, nil); err != nil {
			t.Errorf("NewFraudDecision(100, ...) error = %v, want nil", err)
		}
	})
}

func TestFraudDecision_ShouldReject(t *testing.T) {
	tests := []struct {
		decision string
		want     bool
	}{
		{valueobject.FraudDecisionReject, true},
		{valueobject.FraudDecisionApprove, false},
		{valueobject.FraudDecisionReview, false},
	}

	for _, tc := range tests {
		t.Run(tc.decision, func(t *testing.T) {
			fd, err := valueobject.NewFraudDecision(50, tc.decision, nil)
			if err != nil {
				t.Fatalf("NewFraudDecision() error = %v", err)
			}
			if got := fd.ShouldReject(); got != tc.want {
				t.Errorf("ShouldReject() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFraudDecision_IsZero(t *testing.T) {
	var zero valueobject.FraudDecision
	if !zero.IsZero() {
		t.Error("zero-value FraudDecision.IsZero() = false, want true")
	}

	fd, err := valueobject.NewFraudDecision(10, valueobject.FraudDecisionApprove, nil)
	if err != nil {
		t.Fatalf("NewFraudDecision() error = %v", err)
	}
	if fd.IsZero() {
		t.Error("initialized FraudDecision.IsZero() = true, want false")
	}
}
