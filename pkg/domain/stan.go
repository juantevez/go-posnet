package domain

import "fmt"

// STAN es el System Trace Audit Number.
// Entero en el rango 1–999999 (6 dígitos máximo, estándar ISO 8583).
// Es único por terminal por día y se resetea a 1 al llegar a 999999.
type STAN struct{ value int }

// NewSTAN crea un STAN validando que esté en el rango [1, 999999].
func NewSTAN(v int) (STAN, error) {
	if v < 1 || v > 999999 {
		return STAN{}, fmt.Errorf("stan: value %d out of range [1, 999999]", v)
	}
	return STAN{value: v}, nil
}

// Value retorna el valor entero del STAN.
func (s STAN) Value() int { return s.value }

// String retorna el STAN formateado con ceros a la izquierda (6 dígitos).
func (s STAN) String() string { return fmt.Sprintf("%06d", s.value) }

// Next retorna el siguiente STAN en la secuencia.
// Al llegar a 999999 vuelve a 1 (ciclo diario del terminal).
func (s STAN) Next() STAN {
	next := s.value + 1
	if next > 999999 {
		next = 1
	}
	return STAN{value: next}
}

// Equals compara por valor.
func (s STAN) Equals(other STAN) bool { return s.value == other.value }
