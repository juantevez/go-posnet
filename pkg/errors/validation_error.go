package errors

import "fmt"

// ValidationError indica que una invariante de dominio fue violada.
// Ejemplos: monto negativo, PAN con formato inválido, currency desconocida.
type ValidationError struct {
	Field   string // campo específico que falló (opcional)
	Message string
}

func NewValidationError(message string) *ValidationError {
	return &ValidationError{Message: message}
}

func NewValidationErrorf(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
	}
	return "validation error: " + e.Message
}
func (e *ValidationError) Code() string      { return "VALIDATION_ERROR" }
func (e *ValidationError) HTTPStatus() int   { return 400 }
func (e *ValidationError) IsRetryable() bool { return false }
