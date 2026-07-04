package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
)

func TestNotificationState_IsTerminal(t *testing.T) {
	tests := []struct {
		state valueobject.NotificationState
		want  bool
	}{
		{valueobject.StatePending, false},
		{valueobject.StateSent, true},
		{valueobject.StateFailed, false},
		{valueobject.StateRetrying, false},
		{valueobject.StateDead, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := tc.state.IsTerminal(); got != tc.want {
				t.Errorf("%s.IsTerminal() = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

func TestNotificationState_CanTransitionTo(t *testing.T) {
	allStates := []valueobject.NotificationState{
		valueobject.StatePending,
		valueobject.StateSent,
		valueobject.StateFailed,
		valueobject.StateRetrying,
		valueobject.StateDead,
	}

	// wantAllowed codifica la máquina de estados esperada del BC Notification.
	wantAllowed := map[valueobject.NotificationState]map[valueobject.NotificationState]bool{
		valueobject.StatePending: {
			valueobject.StateSent:   true,
			valueobject.StateFailed: true,
		},
		valueobject.StateFailed: {
			valueobject.StateRetrying: true,
			valueobject.StateDead:     true,
		},
		valueobject.StateRetrying: {
			valueobject.StateSent:   true,
			valueobject.StateFailed: true,
		},
		valueobject.StateSent: {},
		valueobject.StateDead: {},
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

func TestNotificationState_CanTransitionTo_UnknownState(t *testing.T) {
	unknown := valueobject.NotificationState("BOGUS")
	if got := unknown.CanTransitionTo(valueobject.StatePending); got != false {
		t.Errorf("BOGUS.CanTransitionTo(PENDING) = %v, want false", got)
	}
	if got := valueobject.StatePending.CanTransitionTo(unknown); got != false {
		t.Errorf("PENDING.CanTransitionTo(BOGUS) = %v, want false", got)
	}
}

func TestNotificationState_String(t *testing.T) {
	if got := valueobject.StateRetrying.String(); got != "RETRYING" {
		t.Errorf("String() = %q, want %q", got, "RETRYING")
	}
}

func TestParseNotificationState(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    valueobject.NotificationState
		wantErr bool
	}{
		{"pending", "PENDING", valueobject.StatePending, false},
		{"sent", "SENT", valueobject.StateSent, false},
		{"failed", "FAILED", valueobject.StateFailed, false},
		{"retrying", "RETRYING", valueobject.StateRetrying, false},
		{"dead", "DEAD", valueobject.StateDead, false},
		{"unknown", "BOGUS", "", true},
		{"empty", "", "", true},
		{"lowercase not accepted", "pending", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := valueobject.ParseNotificationState(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseNotificationState(%q) error = nil, want error", tc.input)
				}
				if !strings.Contains(err.Error(), "unknown notification state") {
					t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown notification state")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNotificationState(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseNotificationState(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseNotificationChannel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    valueobject.NotificationChannel
		wantErr bool
	}{
		{"terminal websocket", "TERMINAL_WEBSOCKET", valueobject.ChannelTerminalWebSocket, false},
		{"webhook", "WEBHOOK", valueobject.ChannelWebhook, false},
		{"email", "EMAIL", valueobject.ChannelEmail, false},
		{"sms", "SMS", valueobject.ChannelSMS, false},
		{"unknown", "BOGUS", "", true},
		{"empty", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := valueobject.ParseNotificationChannel(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseNotificationChannel(%q) error = nil, want error", tc.input)
				}
				if !strings.Contains(err.Error(), "unknown notification channel") {
					t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown notification channel")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNotificationChannel(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseNotificationChannel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestNotificationChannel_String(t *testing.T) {
	if got := valueobject.ChannelWebhook.String(); got != "WEBHOOK" {
		t.Errorf("String() = %q, want %q", got, "WEBHOOK")
	}
}
