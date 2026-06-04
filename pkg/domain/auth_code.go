package domain

import (
	"fmt"
	"regexp"
)

var reAuthCode = regexp.MustCompile(`^[A-Z0-9]{6}$`)

// AuthCode es el código de autorización de 6 caracteres alfanuméricos
// generado por el banco emisor al aprobar una transacción.
// Es inmutable una vez recibido del adquirente.
type AuthCode struct{ value string }

// NewAuthCode crea un AuthCode validando el formato: exactamente 6 caracteres
// alfanuméricos en mayúsculas (A-Z, 0-9).
func NewAuthCode(s string) (AuthCode, error) {
	if !reAuthCode.MatchString(s) {
		return AuthCode{}, fmt.Errorf("auth_code: must be exactly 6 alphanumeric uppercase chars, got %q", s)
	}
	return AuthCode{value: s}, nil
}

// String implementa fmt.Stringer.
func (a AuthCode) String() string { return a.value }

// IsZero indica si el AuthCode no fue inicializado.
func (a AuthCode) IsZero() bool { return a.value == "" }

// Equals compara por valor.
func (a AuthCode) Equals(other AuthCode) bool { return a.value == other.value }

// MarshalText implementa encoding.TextMarshaler (JSON, slog, etc.).
func (a AuthCode) MarshalText() ([]byte, error) { return []byte(a.value), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (a *AuthCode) UnmarshalText(data []byte) error {
	parsed, err := NewAuthCode(string(data))
	if err != nil {
		return err
	}
	a.value = parsed.value
	return nil
}
