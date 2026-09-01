package domain

import (
	"fmt"
	"regexp"
)

var reCardToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CardToken es un identificador estable y no reversible de una tarjeta.
//
// Se deriva en el borde (terminal/HSM) como HMAC-SHA256(PAN, pepper) y viaja
// en hexadecimal minúscula. El backend Go nunca ve el PAN — ver la invariante
// documentada en PAN — así que este token es el único identificador con el
// que se puede reconocer la misma tarjeta entre transacciones distintas.
//
// Propiedades que lo hacen apto para una blocklist:
//   - Estable: la misma tarjeta produce siempre el mismo token.
//   - No reversible: sin el pepper no se puede volver al PAN.
//   - Sin colisiones prácticas, a diferencia de last4 + red.
//
// El token es OPCIONAL: un terminal que todavía no lo emite manda cadena
// vacía y la transacción sigue el flujo normal sin poder ser bloqueada.
// Nunca se debe inferir un token a partir de last4.
type CardToken struct {
	value string
}

// NewCardToken valida y construye un CardToken a partir de su hexadecimal.
func NewCardToken(s string) (CardToken, error) {
	if !reCardToken.MatchString(s) {
		return CardToken{}, fmt.Errorf("card_token: must be 64 lowercase hex chars")
	}
	return CardToken{value: s}, nil
}

// ParseOptionalCardToken acepta la cadena vacía como "sin token" y devuelve un
// CardToken cero. Cualquier valor no vacío debe ser un token bien formado:
// un token malformado es un error de integración, no una ausencia.
func ParseOptionalCardToken(s string) (CardToken, error) {
	if s == "" {
		return CardToken{}, nil
	}
	return NewCardToken(s)
}

// IsZero indica que la transacción llegó sin token de tarjeta.
func (t CardToken) IsZero() bool { return t.value == "" }

// String devuelve el token completo. Es seguro loguearlo: no es reversible.
func (t CardToken) String() string { return t.value }
