package domain

import (
	"fmt"
	"regexp"
)

// STAN es el System Trace Audit Number.
// Entero 1–999999 (6 dígitos máximo). Único por terminal por día.
type STAN struct{ value int }

func NewSTAN(v int) (STAN, error) {
	if v < 1 || v > 999999 {
		return STAN{}, fmt.Errorf("stan: value %d out of range [1, 999999]", v)
	}
	return STAN{value: v}, nil
}

func (s STAN) Value() int    { return s.value }
func (s STAN) String() string { return fmt.Sprintf("%06d", s.value) }

// Next devuelve el siguiente STAN, volviendo a 1 al llegar a 999999.
func (s STAN) Next() STAN {
	next := s.value + 1
	if next > 999999 {
		next = 1
	}
	return STAN{value: next}
}

// ─────────────────────────────────────────────────────────────────────────────

var reAuthCode = regexp.MustCompile(`^[A-Z0-9]{6}$`)

// AuthCode es el código de autorización de 6 caracteres alfanuméricos
// generado por el banco emisor al aprobar una transacción.
// Es inmutable una vez recibido.
type AuthCode struct{ value string }

func NewAuthCode(s string) (AuthCode, error) {
	if !reAuthCode.MatchString(s) {
		return AuthCode{}, fmt.Errorf("auth_code: must be exactly 6 alphanumeric uppercase chars, got %q", s)
	}
	return AuthCode{value: s}, nil
}

func (a AuthCode) String() string            { return a.value }
func (a AuthCode) IsZero() bool              { return a.value == "" }
func (a AuthCode) Equals(other AuthCode) bool { return a.value == other.value }
