package validator

import (
	"strings"
	"testing"
)

func TestValidateSTAN(t *testing.T) {
	tests := []struct {
		name    string
		stan    int
		wantErr bool
	}{
		{"zero is invalid", 0, true},
		{"negative is invalid", -1, true},
		{"one is valid (min boundary)", 1, false},
		{"typical value is valid", 123456, false},
		{"999999 is valid (max boundary)", 999999, false},
		{"1000000 is invalid (one over max)", 1000000, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSTAN(tc.stan)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("ValidateSTAN(%d) error = %v, want nil", tc.stan, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "out of range") {
				t.Fatalf("ValidateSTAN(%d) error = %v, want it to contain %q", tc.stan, err, "out of range")
			}
		})
	}
}
