package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// TransactionID es el identificador global de una transacción.
// Correlaciona todos los eventos de una transacción a través de todos los BCs.
// Generado en Terminal Gateway al recibir el mensaje del terminal.
type TransactionID struct{ value string }

// NewTransactionID genera un nuevo TransactionID con UUID v4.
func NewTransactionID() TransactionID {
	return TransactionID{value: uuid.NewString()}
}

// ParseTransactionID parsea y valida un UUID v4 existente.
func ParseTransactionID(s string) (TransactionID, error) {
	if _, err := uuid.Parse(s); err != nil {
		return TransactionID{}, fmt.Errorf("invalid transaction_id %q: %w", s, err)
	}
	return TransactionID{value: s}, nil
}

// String implementa fmt.Stringer.
func (t TransactionID) String() string { return t.value }

// IsZero indica si el TransactionID no fue inicializado.
func (t TransactionID) IsZero() bool { return t.value == "" }

// Equals compara por valor.
func (t TransactionID) Equals(other TransactionID) bool { return t.value == other.value }

// MarshalText implementa encoding.TextMarshaler (para JSON, slog, etc.).
func (t TransactionID) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (t *TransactionID) UnmarshalText(data []byte) error {
	parsed, err := ParseTransactionID(string(data))
	if err != nil {
		return err
	}
	t.value = parsed.value
	return nil
}
