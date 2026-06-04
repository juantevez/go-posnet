// Package errors define los tipos de error de dominio del sistema POSNET.
// Usar errores tipados permite que cada capa tome decisiones precisas
// sin parsear strings de error.
package errors

import "fmt"

// DomainError es la interfaz base de todos los errores de dominio.
type DomainError interface {
	error
	Code() string    // Código único: "NOT_FOUND", "FRAUD_REJECTED", etc.
	HTTPStatus() int // Código HTTP sugerido
	IsRetryable() bool
}

// ─── NotFoundError ────────────────────────────────────────────────────────────

// NotFoundError indica que una entidad no fue encontrada.
type NotFoundError struct {
	Entity string
	ID     string
}

func NewNotFoundError(entity, id string) *NotFoundError {
	return &NotFoundError{Entity: entity, ID: id}
}
func (e *NotFoundError) Error() string    { return fmt.Sprintf("%s with id %q not found", e.Entity, e.ID) }
func (e *NotFoundError) Code() string     { return "NOT_FOUND" }
func (e *NotFoundError) HTTPStatus() int  { return 404 }
func (e *NotFoundError) IsRetryable() bool { return false }

// ─── ValidationError ──────────────────────────────────────────────────────────

// ValidationError indica una invariante de dominio violada.
type ValidationError struct {
	Message string
	Field   string // opcional: campo específico que falló
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
func (e *ValidationError) Code() string     { return "VALIDATION_ERROR" }
func (e *ValidationError) HTTPStatus() int  { return 400 }
func (e *ValidationError) IsRetryable() bool { return false }

// ─── ConflictError ────────────────────────────────────────────────────────────

// ConflictError indica una operación duplicada (clave de idempotencia ya procesada).
type ConflictError struct {
	EventID string
	Message string
}

func NewConflictError(eventID string) *ConflictError {
	return &ConflictError{EventID: eventID, Message: fmt.Sprintf("event %q already processed", eventID)}
}
func (e *ConflictError) Error() string    { return e.Message }
func (e *ConflictError) Code() string     { return "CONFLICT" }
func (e *ConflictError) HTTPStatus() int  { return 409 }
func (e *ConflictError) IsRetryable() bool { return false }

// ─── TimeoutError ─────────────────────────────────────────────────────────────

// TimeoutError indica que un sistema externo no respondió a tiempo.
type TimeoutError struct {
	System  string // "acquirer", "issuer", "fraud_engine"
	Message string
}

func NewTimeoutError(system string) *TimeoutError {
	return &TimeoutError{System: system, Message: fmt.Sprintf("%s did not respond in time", system)}
}
func (e *TimeoutError) Error() string    { return e.Message }
func (e *TimeoutError) Code() string     { return "TIMEOUT" }
func (e *TimeoutError) HTTPStatus() int  { return 504 }
func (e *TimeoutError) IsRetryable() bool { return true }

// ─── FraudError ───────────────────────────────────────────────────────────────

// FraudError indica que el motor antifraude rechazó la transacción.
type FraudError struct {
	Score    int
	Decision string
	Rules    []string // reglas que activaron
}

func NewFraudError(score int, decision string, rules []string) *FraudError {
	return &FraudError{Score: score, Decision: decision, Rules: rules}
}
func (e *FraudError) Error() string {
	return fmt.Sprintf("transaction rejected by fraud engine: score=%d decision=%s", e.Score, e.Decision)
}
func (e *FraudError) Code() string     { return "FRAUD_REJECTED" }
func (e *FraudError) HTTPStatus() int  { return 422 }
func (e *FraudError) IsRetryable() bool { return false }

// ─── AcquirerError ────────────────────────────────────────────────────────────

// AcquirerError encapsula el rechazo del host adquirente o banco emisor.
// ISOCode es el código de respuesta ISO 8583 original.
type AcquirerError struct {
	ISOCode string // "51", "54", "05", "91", etc.
	Message string
}

func NewAcquirerError(isoCode, message string) *AcquirerError {
	return &AcquirerError{ISOCode: isoCode, Message: message}
}
func (e *AcquirerError) Error() string {
	return fmt.Sprintf("acquirer rejected transaction: ISO code %s — %s", e.ISOCode, e.Message)
}
func (e *AcquirerError) Code() string     { return "ACQUIRER_ERROR" }
func (e *AcquirerError) HTTPStatus() int  { return 422 }
func (e *AcquirerError) IsRetryable() bool {
	// Códigos que tienen sentido reintentar (problemas transitorios del emisor)
	retryable := map[string]bool{"91": true, "96": true}
	return retryable[e.ISOCode]
}

// ─── UnauthorizedError ────────────────────────────────────────────────────────

// UnauthorizedError indica terminal no autenticado o certificado inválido.
type UnauthorizedError struct{ Message string }

func NewUnauthorizedError(msg string) *UnauthorizedError { return &UnauthorizedError{Message: msg} }
func (e *UnauthorizedError) Error() string     { return "unauthorized: " + e.Message }
func (e *UnauthorizedError) Code() string      { return "UNAUTHORIZED" }
func (e *UnauthorizedError) HTTPStatus() int   { return 401 }
func (e *UnauthorizedError) IsRetryable() bool { return false }

// ─── InfrastructureError ──────────────────────────────────────────────────────

// InfrastructureError indica un fallo técnico de infraestructura (BD, NATS, red).
type InfrastructureError struct {
	Component string // "postgres", "nats", "grpc"
	Cause     error
}

func NewInfrastructureError(component string, cause error) *InfrastructureError {
	return &InfrastructureError{Component: component, Cause: cause}
}
func (e *InfrastructureError) Error() string {
	return fmt.Sprintf("infrastructure error in %s: %v", e.Component, e.Cause)
}
func (e *InfrastructureError) Unwrap() error   { return e.Cause }
func (e *InfrastructureError) Code() string    { return "INFRASTRUCTURE_ERROR" }
func (e *InfrastructureError) HTTPStatus() int { return 500 }
func (e *InfrastructureError) IsRetryable() bool { return true }
