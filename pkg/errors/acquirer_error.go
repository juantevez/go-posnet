package errors

import "fmt"

// AcquirerError encapsula el rechazo del host adquirente o banco emisor.
// ISOCode es el código de respuesta ISO 8583 original (DE-39).
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
func (e *AcquirerError) Code() string    { return "ACQUIRER_ERROR" }
func (e *AcquirerError) HTTPStatus() int { return 422 }

// IsRetryable indica si tiene sentido reintentar.
// Solo los códigos de error transitorio del emisor son retryables.
func (e *AcquirerError) IsRetryable() bool {
	retryable := map[string]bool{
		"91": true, // Issuer Unavailable
		"96": true, // System Malfunction
	}
	return retryable[e.ISOCode]
}
