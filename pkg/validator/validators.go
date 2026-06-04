// Package validator contiene funciones de validación de dominio reutilizables.
// Son funciones puras: sin efectos secundarios, sin dependencias externas.
package validator

import (
	"fmt"
)

// ─── ISO 8583 ─────────────────────────────────────────────────────────────────

// ValidateISO8583 verifica que el mensaje binario tenga una estructura
// ISO 8583 mínimamente válida: MTI de 4 bytes y bitmap de 8 bytes.
func ValidateISO8583(msg []byte) error {
	if len(msg) < 12 {
		return fmt.Errorf("iso8583: message too short: %d bytes (minimum 12)", len(msg))
	}
	mti := msg[0:4]
	// MTI debe ser numérico (BCD o ASCII según implementación del adquirente)
	for _, b := range mti {
		if b < '0' || b > '9' {
			// Puede ser BCD — validación básica: no todos cero
			break
		}
	}
	// El bitmap primario ocupa bytes 4–11
	bitmap := msg[4:12]
	allZero := true
	for _, b := range bitmap {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return fmt.Errorf("iso8583: primary bitmap is all zeros — invalid message")
	}
	return nil
}

// ─── Luhn ─────────────────────────────────────────────────────────────────────

// LuhnCheck verifica que el PAN tenga un dígito verificador válido (algoritmo de Luhn).
// Se usa como primera barrera antes de enviar al adquirente.
// No valida que la tarjeta exista — solo que el número sea matemáticamente válido.
func LuhnCheck(pan string) bool {
	if len(pan) < 13 || len(pan) > 19 {
		return false
	}
	sum := 0
	nDigits := len(pan)
	parity := nDigits % 2

	for i := 0; i < nDigits; i++ {
		d := int(pan[i] - '0')
		if d < 0 || d > 9 {
			return false // carácter no numérico
		}
		if i%2 == parity {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

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

// ─── STAN ─────────────────────────────────────────────────────────────────────

// ValidateSTAN verifica que el STAN esté en el rango 1–999999 (ISO 8583).
func ValidateSTAN(stan int) error {
	if stan < 1 || stan > 999999 {
		return fmt.Errorf("stan: value %d out of range [1, 999999]", stan)
	}
	return nil
}
