package domain

import (
	"fmt"
	"math"
)

// Money representa un monto monetario.
// INVARIANTE CRÍTICA: los montos se almacenan SIEMPRE en centavos (int64).
// Nunca se usa float para dinero — los errores de punto flotante tienen
// consecuencias financieras reales.
type Money struct {
	cents    int64
	currency Currency
}

// NewMoney crea un Money validando las invariantes.
// cents puede ser negativo solo en contextos de reversión/devolución.
func NewMoney(cents int64, currency Currency) (Money, error) {
	if !currency.IsValid() {
		return Money{}, fmt.Errorf("money: invalid currency %q", currency)
	}
	if cents == 0 {
		return Money{}, fmt.Errorf("money: amount cannot be zero")
	}
	// Prevenir overflow: int64 max ~ 9.2 * 10^18 centavos
	if cents > math.MaxInt64/100 {
		return Money{}, fmt.Errorf("money: amount %d exceeds maximum allowed value", cents)
	}
	return Money{cents: cents, currency: currency}, nil
}

// NewMoneyFromFloat convierte un float (ej: 150.50) a Money en centavos.
// Uso exclusivo en deserialización de APIs externas. Internamente siempre usar centavos.
func NewMoneyFromFloat(amount float64, currency Currency) (Money, error) {
	cents := int64(math.Round(amount * 100))
	return NewMoney(cents, currency)
}

// Cents devuelve el valor en centavos (solo lectura).
func (m Money) Cents() int64 { return m.cents }

// Currency devuelve la moneda.
func (m Money) Currency() Currency { return m.currency }

// IsPositive indica si el monto es mayor a cero.
func (m Money) IsPositive() bool { return m.cents > 0 }

// IsNegative indica si el monto es menor a cero (reversión/devolución).
func (m Money) IsNegative() bool { return m.cents < 0 }

// Add suma dos Money. Devuelve error si las monedas difieren.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("money: cannot add %s and %s", m.currency, other.currency)
	}
	return Money{cents: m.cents + other.cents, currency: m.currency}, nil
}

// Negate devuelve el Money con el signo invertido (para reversiones).
func (m Money) Negate() Money {
	return Money{cents: -m.cents, currency: m.currency}
}

// Equals compara dos Money por valor.
func (m Money) Equals(other Money) bool {
	return m.cents == other.cents && m.currency == other.currency
}

// String devuelve representación legible: "ARS 1500.50"
func (m Money) String() string {
	abs := m.cents
	if abs < 0 {
		abs = -abs
	}
	sign := ""
	if m.cents < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s %s%d.%02d", m.currency, sign, abs/100, abs%100)
}
