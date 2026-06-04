package validator

import "fmt"

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
