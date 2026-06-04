package errors

import "fmt"

// NotFoundError indica que una entidad no fue encontrada en el repositorio.
type NotFoundError struct {
	Entity string // ej: "Transaction", "Terminal"
	ID     string // ID buscado
}

func NewNotFoundError(entity, id string) *NotFoundError {
	return &NotFoundError{Entity: entity, ID: id}
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with id %q not found", e.Entity, e.ID)
}
func (e *NotFoundError) Code() string      { return "NOT_FOUND" }
func (e *NotFoundError) HTTPStatus() int   { return 404 }
func (e *NotFoundError) IsRetryable() bool { return false }
