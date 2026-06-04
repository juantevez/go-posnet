package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// TerminalID identifica de forma única un terminal POSNET físico registrado
// en el sistema. Es inmutable una vez asignado al terminal.
type TerminalID struct{ value string }

// NewTerminalID genera un nuevo TerminalID con UUID v4.
func NewTerminalID() TerminalID {
	return TerminalID{value: uuid.NewString()}
}

// ParseTerminalID parsea y valida un UUID v4 existente.
func ParseTerminalID(s string) (TerminalID, error) {
	if _, err := uuid.Parse(s); err != nil {
		return TerminalID{}, fmt.Errorf("invalid terminal_id %q: %w", s, err)
	}
	return TerminalID{value: s}, nil
}

// String implementa fmt.Stringer.
func (t TerminalID) String() string { return t.value }

// IsZero indica si el TerminalID no fue inicializado.
func (t TerminalID) IsZero() bool { return t.value == "" }

// Equals compara por valor.
func (t TerminalID) Equals(other TerminalID) bool { return t.value == other.value }

// MarshalText implementa encoding.TextMarshaler.
func (t TerminalID) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (t *TerminalID) UnmarshalText(data []byte) error {
	parsed, err := ParseTerminalID(string(data))
	if err != nil {
		return err
	}
	t.value = parsed.value
	return nil
}
