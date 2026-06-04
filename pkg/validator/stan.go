package validator

import "fmt"

// ─── STAN ─────────────────────────────────────────────────────────────────────

// ValidateSTAN verifica que el STAN esté en el rango 1–999999 (ISO 8583).
func ValidateSTAN(stan int) error {
	if stan < 1 || stan > 999999 {
		return fmt.Errorf("stan: value %d out of range [1, 999999]", stan)
	}
	return nil
}
