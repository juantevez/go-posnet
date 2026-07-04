package validator

import (
	"strings"
	"testing"
)

func TestValidateAmount(t *testing.T) {
	tests := []struct {
		name       string
		cents      int64
		wantErr    bool
		wantSubstr string
	}{
		{"zero is invalid", 0, true, "must be positive"},
		{"negative is invalid", -1, true, "must be positive"},
		{"one cent is valid", 1, false, ""},
		{"typical amount is valid", 500000, false, ""},
		{"at max boundary is valid", maxAmountCents, false, ""},
		{"one over max is invalid", maxAmountCents + 1, true, "exceeds maximum"},
		{"far over max is invalid", maxAmountCents * 100, true, "exceeds maximum"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAmount(tc.cents)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("ValidateAmount(%d) error = %v, want nil", tc.cents, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("ValidateAmount(%d) error = %v, want it to contain %q", tc.cents, err, tc.wantSubstr)
			}
		})
	}
}
