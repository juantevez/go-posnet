package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
)

func TestTransactionState_IsTerminal(t *testing.T) {
	tests := []struct {
		state valueobject.TransactionState
		want  bool
	}{
		{valueobject.StateReceived, false},
		{valueobject.StateFraudChecking, false},
		{valueobject.StateProcessing, false},
		{valueobject.StateApproved, true},
		{valueobject.StateRejected, true},
		{valueobject.StateIndeterminate, false},
		{valueobject.StateReversed, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := tc.state.IsTerminal(); got != tc.want {
				t.Errorf("%s.IsTerminal() = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

func TestTransactionState_CanTransitionTo(t *testing.T) {
	allStates := []valueobject.TransactionState{
		valueobject.StateReceived,
		valueobject.StateFraudChecking,
		valueobject.StateProcessing,
		valueobject.StateApproved,
		valueobject.StateRejected,
		valueobject.StateIndeterminate,
		valueobject.StateReversed,
	}

	// wantAllowed codifica la máquina de estados esperada del BC Authorization.
	wantAllowed := map[valueobject.TransactionState]map[valueobject.TransactionState]bool{
		valueobject.StateReceived: {
			valueobject.StateFraudChecking: true,
			valueobject.StateRejected:      true,
		},
		valueobject.StateFraudChecking: {
			valueobject.StateProcessing: true,
			valueobject.StateRejected:   true,
		},
		valueobject.StateProcessing: {
			valueobject.StateApproved:      true,
			valueobject.StateRejected:      true,
			valueobject.StateIndeterminate: true,
		},
		valueobject.StateIndeterminate: {
			valueobject.StateReversed: true,
			valueobject.StateApproved: true,
			valueobject.StateRejected: true,
		},
		valueobject.StateApproved: {
			valueobject.StateReversed: true,
		},
		valueobject.StateRejected: {},
		valueobject.StateReversed: {},
	}

	for _, from := range allStates {
		for _, to := range allStates {
			want := wantAllowed[from][to]
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				if got := from.CanTransitionTo(to); got != want {
					t.Errorf("%s.CanTransitionTo(%s) = %v, want %v", from, to, got, want)
				}
			})
		}
	}
}

func TestTransactionState_CanTransitionTo_UnknownState(t *testing.T) {
	unknown := valueobject.TransactionState("BOGUS")
	if got := unknown.CanTransitionTo(valueobject.StateReceived); got != false {
		t.Errorf("BOGUS.CanTransitionTo(RECEIVED) = %v, want false", got)
	}
	if got := valueobject.StateReceived.CanTransitionTo(unknown); got != false {
		t.Errorf("RECEIVED.CanTransitionTo(BOGUS) = %v, want false", got)
	}
}

func TestTransactionState_String(t *testing.T) {
	if got := valueobject.StateApproved.String(); got != "APPROVED" {
		t.Errorf("StateApproved.String() = %q, want %q", got, "APPROVED")
	}
}

func TestParseEntryMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    valueobject.EntryMode
		wantErr bool
	}{
		{"chip", "CHIP", valueobject.EntryModeChip, false},
		{"contactless", "CONTACTLESS", valueobject.EntryModeContactless, false},
		{"magstripe", "MAGSTRIPE", valueobject.EntryModeMagstripe, false},
		{"manual", "MANUAL", valueobject.EntryModeManual, false},
		{"unknown value", "SWIPE", "", true},
		{"empty string", "", "", true},
		{"lowercase not accepted", "chip", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := valueobject.ParseEntryMode(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseEntryMode(%q) error = nil, want error", tc.input)
				}
				if !strings.Contains(err.Error(), "unknown entry mode") {
					t.Errorf("ParseEntryMode(%q) error = %q, want it to contain %q", tc.input, err.Error(), "unknown entry mode")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEntryMode(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseEntryMode(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEntryMode_String(t *testing.T) {
	if got := valueobject.EntryModeContactless.String(); got != "CONTACTLESS" {
		t.Errorf("EntryModeContactless.String() = %q, want %q", got, "CONTACTLESS")
	}
}
