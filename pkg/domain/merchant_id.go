package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// MerchantID identifica de forma única un comercio registrado en el adquirente.
// Un comercio puede tener múltiples terminales asociados.
type MerchantID struct{ value string }

// NewMerchantID genera un nuevo MerchantID con UUID v4.
func NewMerchantID() MerchantID {
	return MerchantID{value: uuid.NewString()}
}

// ParseMerchantID parsea y valida un UUID v4 existente.
func ParseMerchantID(s string) (MerchantID, error) {
	if _, err := uuid.Parse(s); err != nil {
		return MerchantID{}, fmt.Errorf("invalid merchant_id %q: %w", s, err)
	}
	return MerchantID{value: s}, nil
}

// String implementa fmt.Stringer.
func (m MerchantID) String() string { return m.value }

// IsZero indica si el MerchantID no fue inicializado.
func (m MerchantID) IsZero() bool { return m.value == "" }

// Equals compara por valor.
func (m MerchantID) Equals(other MerchantID) bool { return m.value == other.value }

// MarshalText implementa encoding.TextMarshaler.
func (m MerchantID) MarshalText() ([]byte, error) { return []byte(m.value), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (m *MerchantID) UnmarshalText(data []byte) error {
	parsed, err := ParseMerchantID(string(data))
	if err != nil {
		return err
	}
	m.value = parsed.value
	return nil
}
