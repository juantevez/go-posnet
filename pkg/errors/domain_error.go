// Package errors define los tipos de error de dominio del sistema POSNET.
// Usar errores tipados permite que cada capa tome decisiones precisas
// sin parsear strings de error.
package errors

// DomainError es la interfaz base de todos los errores de dominio.
// Cada error concreto la implementa con su propio Code, HTTPStatus e IsRetryable.
type DomainError interface {
	error
	Code() string      // Código único: "NOT_FOUND", "FRAUD_REJECTED", etc.
	HTTPStatus() int   // Código HTTP sugerido para la respuesta al cliente
	IsRetryable() bool // ¿Tiene sentido reintentar esta operación?
}
