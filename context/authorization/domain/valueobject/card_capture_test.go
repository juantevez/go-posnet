package valueobject_test

import (
	"testing"

	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
)

func TestRejectionCode_RequiresCardCapture(t *testing.T) {
	tests := []struct {
		name string
		rc   func(t *testing.T) valueobject.RejectionCode
		want bool
	}{
		{
			name: "acquirer stolen card",
			rc:   isoRejection(valueobject.ISO_STOLEN_CARD),
			want: true,
		},
		{
			name: "acquirer lost card",
			rc:   isoRejection(valueobject.ISO_LOST_CARD),
			want: true,
		},
		{
			name: "acquirer capture card",
			rc:   isoRejection(valueobject.ISO_CAPTURE_CARD),
			want: true,
		},
		{
			name: "acquirer insufficient funds does not capture",
			rc:   isoRejection(valueobject.ISO_INSUFFICIENT_FUNDS),
			want: false,
		},
		{
			name: "blocklist carries the original issuer capture order",
			rc: func(*testing.T) valueobject.RejectionCode {
				return valueobject.NewRejectionFromBlocklist()
			},
			want: true,
		},
		{
			name: "fraud engine cannot order a capture",
			rc: func(*testing.T) valueobject.RejectionCode {
				return valueobject.NewRejectionFromFraud()
			},
			want: false,
		},
		{
			// Un rechazo local que arrastre el string "43" no tiene autoridad
			// del emisor para retener el plástico.
			name: "local validation reusing an ISO capture code does not capture",
			rc: func(*testing.T) valueobject.RejectionCode {
				return valueobject.NewRejectionFromValidation(valueobject.ISO_STOLEN_CARD)
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rc(t).RequiresCardCapture(); got != tc.want {
				t.Errorf("RequiresCardCapture() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewRejectionFromBlocklist(t *testing.T) {
	rc := valueobject.NewRejectionFromBlocklist()

	if rc.Source() != valueobject.SourceBlocklist {
		t.Errorf("Source() = %v, want %v", rc.Source(), valueobject.SourceBlocklist)
	}
	if rc.IsRetryable() {
		t.Error("IsRetryable() = true, want false — una tarjeta bloqueada no se reintenta")
	}
	if got, want := rc.Description(), "Card Blocked - Pick Up"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestRejectionDescription_CaptureCodes(t *testing.T) {
	tests := map[string]string{
		valueobject.ISO_CAPTURE_CARD: "Capture Card",
		valueobject.ISO_LOST_CARD:    "Lost Card - Pick Up",
		valueobject.ISO_STOLEN_CARD:  "Stolen Card - Pick Up",
	}

	for code, want := range tests {
		rc, err := valueobject.NewRejectionFromISO(code)
		if err != nil {
			t.Fatalf("NewRejectionFromISO(%q) error = %v", code, err)
		}
		if got := rc.Description(); got != want {
			t.Errorf("Description() for %q = %q, want %q", code, got, want)
		}
	}
}

func isoRejection(code string) func(t *testing.T) valueobject.RejectionCode {
	return func(t *testing.T) valueobject.RejectionCode {
		t.Helper()
		rc, err := valueobject.NewRejectionFromISO(code)
		if err != nil {
			t.Fatalf("NewRejectionFromISO(%q) error = %v", code, err)
		}
		return rc
	}
}
