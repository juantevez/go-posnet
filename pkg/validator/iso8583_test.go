package validator

import (
	"strings"
	"testing"
)

func TestValidateISO8583(t *testing.T) {
	nonZeroBitmap := []byte{0x80, 0, 0, 0, 0, 0, 0, 0} // bit 1 activado — bitmap típico no vacío
	zeroBitmap := make([]byte, 8)

	tests := []struct {
		name       string
		msg        []byte
		wantErr    bool
		wantSubstr string
	}{
		{
			name:       "empty message is too short",
			msg:        nil,
			wantErr:    true,
			wantSubstr: "too short",
		},
		{
			name:       "11 bytes is too short",
			msg:        append([]byte("0200"), zeroBitmap[:7]...),
			wantErr:    true,
			wantSubstr: "too short",
		},
		{
			name:       "exactly 12 bytes with all-zero bitmap is invalid",
			msg:        append([]byte("0200"), zeroBitmap...),
			wantErr:    true,
			wantSubstr: "primary bitmap is all zeros",
		},
		{
			name:    "exactly 12 bytes with non-zero bitmap is valid",
			msg:     append([]byte("0200"), nonZeroBitmap...),
			wantErr: false,
		},
		{
			name:    "message longer than 12 bytes with non-zero bitmap is valid",
			msg:     append(append([]byte("0200"), nonZeroBitmap...), []byte("extra data elements")...),
			wantErr: false,
		},
		{
			name:       "message longer than 12 bytes with all-zero bitmap is still invalid",
			msg:        append(append([]byte("0200"), zeroBitmap...), []byte("extra data elements")...),
			wantErr:    true,
			wantSubstr: "primary bitmap is all zeros",
		},
		{
			name:    "non-numeric MTI is not validated — only length and bitmap matter",
			msg:     append([]byte("ABCD"), nonZeroBitmap...),
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateISO8583(tc.msg)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("ValidateISO8583(%v) error = %v, want nil", tc.msg, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("ValidateISO8583(%v) error = %v, want it to contain %q", tc.msg, err, tc.wantSubstr)
			}
		})
	}
}
