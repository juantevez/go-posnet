package valueobject_test

import (
	"testing"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
)

// ─── SessionState.IsTerminal ──────────────────────────────────────────────────

func TestSessionState_IsTerminal(t *testing.T) {
	tests := []struct {
		state valueobject.SessionState
		want  bool
	}{
		{valueobject.StateIdle, false},
		{valueobject.StateAwaitingPayment, false},
		{valueobject.StateProcessing, false},
		{valueobject.StateApproved, true},
		{valueobject.StateRejected, true},
		{valueobject.StateExpired, true},
		{valueobject.StateCancelled, true},
		{valueobject.StateReconnecting, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := tc.state.IsTerminal(); got != tc.want {
				t.Errorf("IsTerminal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ─── SessionState.CanTransitionTo ─────────────────────────────────────────────

// allowedTransitions refleja la matriz esperada de transiciones válidas.
var allowedTransitions = map[valueobject.SessionState]map[valueobject.SessionState]bool{
	valueobject.StateIdle: {
		valueobject.StateAwaitingPayment: true,
	},
	valueobject.StateAwaitingPayment: {
		valueobject.StateProcessing: true,
		valueobject.StateExpired:    true,
		valueobject.StateCancelled:  true,
	},
	valueobject.StateProcessing: {
		valueobject.StateApproved:     true,
		valueobject.StateRejected:     true,
		valueobject.StateReconnecting: true,
	},
	valueobject.StateReconnecting: {
		valueobject.StateProcessing: true,
		valueobject.StateApproved:   true,
		valueobject.StateRejected:   true,
	},
	valueobject.StateApproved:  {},
	valueobject.StateRejected:  {},
	valueobject.StateExpired:   {},
	valueobject.StateCancelled: {},
}

func TestSessionState_CanTransitionTo_Matrix(t *testing.T) {
	allStates := []valueobject.SessionState{
		valueobject.StateIdle,
		valueobject.StateAwaitingPayment,
		valueobject.StateProcessing,
		valueobject.StateApproved,
		valueobject.StateRejected,
		valueobject.StateExpired,
		valueobject.StateCancelled,
		valueobject.StateReconnecting,
	}

	for _, from := range allStates {
		for _, to := range allStates {
			want := allowedTransitions[from][to]
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				if got := from.CanTransitionTo(to); got != want {
					t.Errorf("CanTransitionTo() = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestSessionState_CanTransitionTo_UnknownState(t *testing.T) {
	unknown := valueobject.SessionState("UNKNOWN")
	if unknown.CanTransitionTo(valueobject.StateIdle) {
		t.Error("CanTransitionTo() = true for an unknown source state, want false")
	}
	if valueobject.StateIdle.CanTransitionTo(unknown) {
		t.Error("CanTransitionTo() = true for an unknown target state, want false")
	}
}

// ─── SessionState.String ──────────────────────────────────────────────────────

func TestSessionState_String(t *testing.T) {
	if got := valueobject.StateProcessing.String(); got != "PROCESSING" {
		t.Errorf("String() = %q, want %q", got, "PROCESSING")
	}
}

// ─── PaymentChannel ────────────────────────────────────────────────────────────

func TestPaymentChannel_String(t *testing.T) {
	if got := valueobject.ChannelNFC.String(); got != "NFC" {
		t.Errorf("String() = %q, want %q", got, "NFC")
	}
}

func TestParsePaymentChannel(t *testing.T) {
	tests := []struct {
		input   string
		want    valueobject.PaymentChannel
		wantErr bool
	}{
		{"QR", valueobject.ChannelQR, false},
		{"NFC", valueobject.ChannelNFC, false},
		{"APPLE_PAY", valueobject.ChannelApplePay, false},
		{"GOOGLE_PAY", valueobject.ChannelGooglePay, false},
		{"MAGSTRIPE", valueobject.ChannelMagstripe, false},
		{"qr", "", true},
		{"", "", true},
		{"UNKNOWN", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := valueobject.ParsePaymentChannel(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePaymentChannel(%q) error = nil, want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePaymentChannel(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParsePaymentChannel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPaymentChannel_ToEntryMode(t *testing.T) {
	tests := []struct {
		channel valueobject.PaymentChannel
		want    string
	}{
		{valueobject.ChannelQR, "CONTACTLESS"},
		{valueobject.ChannelNFC, "CONTACTLESS"},
		{valueobject.ChannelApplePay, "CONTACTLESS"},
		{valueobject.ChannelGooglePay, "CONTACTLESS"},
		{valueobject.ChannelMagstripe, "MAGSTRIPE"},
		{valueobject.PaymentChannel("UNKNOWN"), "CHIP"},
	}

	for _, tc := range tests {
		t.Run(string(tc.channel), func(t *testing.T) {
			if got := tc.channel.ToEntryMode(); got != tc.want {
				t.Errorf("ToEntryMode() = %q, want %q", got, tc.want)
			}
		})
	}
}
