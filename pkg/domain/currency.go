// Package domain contiene los Value Objects compartidos del dominio financiero.
// Son el vocabulario común del sistema: todos los Bounded Contexts los usan.
package domain

import "fmt"

// Currency representa un código de moneda ISO 4217.
// Es un string tipado — no acepta cualquier string arbitrario.
type Currency string

const (
	ARS Currency = "ARS" // Peso Argentino
	USD Currency = "USD" // Dólar Estadounidense
	EUR Currency = "EUR" // Euro
	BRL Currency = "BRL" // Real Brasileño
	UYU Currency = "UYU" // Peso Uruguayo
	CLP Currency = "CLP" // Peso Chileno
)

var validCurrencies = map[Currency]struct{}{
	ARS: {}, USD: {}, EUR: {}, BRL: {}, UYU: {}, CLP: {},
}

// IsValid verifica que sea un código ISO 4217 conocido por el sistema.
func (c Currency) IsValid() bool {
	_, ok := validCurrencies[c]
	return ok
}

// String implementa fmt.Stringer.
func (c Currency) String() string { return string(c) }

// ParseCurrency parsea un string y devuelve error si no es una Currency válida.
func ParseCurrency(s string) (Currency, error) {
	c := Currency(s)
	if !c.IsValid() {
		return "", fmt.Errorf("invalid currency %q: must be a valid ISO 4217 code", s)
	}
	return c, nil
}
