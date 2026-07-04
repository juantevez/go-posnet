package validator

import "testing"

func TestLuhnCheck(t *testing.T) {
	tests := []struct {
		name string
		pan  string
		want bool
	}{
		{"valid 16-digit Visa test PAN", "4111111111111111", true},
		{"valid 16-digit Mastercard test PAN (doubled digit overflows past 9)", "5500005555555559", true},
		{"valid 13-digit Visa test PAN", "4222222222222", true},
		{"valid 19-digit PAN", "4000000000000000113", true},
		{"invalid checksum (last digit tampered)", "4111111111111112", false},
		{"invalid checksum (13-digit)", "4222222222221", false},
		{"12 digits is too short", "421111111111", false},
		{"20 digits is too long", "41111111111111111112", false},
		{"empty string is too short", "", false},
		{"non-numeric character", "411111111111111a", false},
		{"non-numeric character below '0'", "411111111111111/", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LuhnCheck(tc.pan); got != tc.want {
				t.Errorf("LuhnCheck(%q) = %v, want %v", tc.pan, got, tc.want)
			}
		})
	}
}
