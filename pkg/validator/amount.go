package validator

import "fmt"

// ─── Monto ───────────────────────────────────────────────────────────────────

const maxAmountCents int64 = 10_000_000_00 // ARS 10.000.000 en centavos

// ValidateAmount verifica que el monto sea positivo y no supere el límite máximo.
func ValidateAmount(cents int64) error {
	if cents <= 0 {
		return fmt.Errorf("amount: must be positive, got %d", cents)
	}
	if cents > maxAmountCents {
		return fmt.Errorf("amount: %d cents exceeds maximum allowed %d", cents, maxAmountCents)
	}
	return nil
}
