package errors

import "fmt"

// FraudError indica que el motor antifraude rechazó la transacción.
// No es retryable: el rechazo es una decisión de negocio, no un error técnico.
type FraudError struct {
	Score    int      // Score calculado (0–100)
	Decision string   // "REJECT"
	Rules    []string // IDs de las reglas que activaron el rechazo
}

func NewFraudError(score int, decision string, rules []string) *FraudError {
	return &FraudError{Score: score, Decision: decision, Rules: rules}
}

func (e *FraudError) Error() string {
	return fmt.Sprintf("transaction rejected by fraud engine: score=%d decision=%s", e.Score, e.Decision)
}
func (e *FraudError) Code() string      { return "FRAUD_REJECTED" }
func (e *FraudError) HTTPStatus() int   { return 422 }
func (e *FraudError) IsRetryable() bool { return false }
