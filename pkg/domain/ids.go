package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// TerminalID identifica de forma única un terminal POSNET físico.
type TerminalID struct{ value string }

func NewTerminalID() TerminalID                        { return TerminalID{value: uuid.NewString()} }
func ParseTerminalID(s string) (TerminalID, error) {
	if _, err := uuid.Parse(s); err != nil {
		return TerminalID{}, fmt.Errorf("invalid terminal_id %q: %w", s, err)
	}
	return TerminalID{value: s}, nil
}
func (t TerminalID) String() string               { return t.value }
func (t TerminalID) IsZero() bool                 { return t.value == "" }
func (t TerminalID) Equals(other TerminalID) bool { return t.value == other.value }
func (t TerminalID) MarshalText() ([]byte, error) { return []byte(t.value), nil }
func (t *TerminalID) UnmarshalText(data []byte) error {
	parsed, err := ParseTerminalID(string(data))
	if err != nil {
		return err
	}
	t.value = parsed.value
	return nil
}

// MerchantID identifica de forma única un comercio registrado en el adquirente.
type MerchantID struct{ value string }

func NewMerchantID() MerchantID                        { return MerchantID{value: uuid.NewString()} }
func ParseMerchantID(s string) (MerchantID, error) {
	if _, err := uuid.Parse(s); err != nil {
		return MerchantID{}, fmt.Errorf("invalid merchant_id %q: %w", s, err)
	}
	return MerchantID{value: s}, nil
}
func (m MerchantID) String() string               { return m.value }
func (m MerchantID) IsZero() bool                 { return m.value == "" }
func (m MerchantID) Equals(other MerchantID) bool { return m.value == other.value }
func (m MerchantID) MarshalText() ([]byte, error) { return []byte(m.value), nil }
func (m *MerchantID) UnmarshalText(data []byte) error {
	parsed, err := ParseMerchantID(string(data))
	if err != nil {
		return err
	}
	m.value = parsed.value
	return nil
}
