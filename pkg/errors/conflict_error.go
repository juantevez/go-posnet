package errors

import "fmt"

// ConflictError indica una operación duplicada detectada por la clave de idempotencia.
// Ocurre cuando un event_id ya fue procesado previamente.
type ConflictError struct {
	EventID string
	Message string
}

func NewConflictError(eventID string) *ConflictError {
	return &ConflictError{
		EventID: eventID,
		Message: fmt.Sprintf("event %q already processed", eventID),
	}
}

func (e *ConflictError) Error() string     { return e.Message }
func (e *ConflictError) Code() string      { return "CONFLICT" }
func (e *ConflictError) HTTPStatus() int   { return 409 }
func (e *ConflictError) IsRetryable() bool { return false }
