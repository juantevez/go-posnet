package errors

import "fmt"

// TimeoutError indica que un sistema externo no respondió dentro del tiempo máximo.
// Es retryable: el problema es transitorio y tiene sentido reintentar.
type TimeoutError struct {
	System  string // "acquirer", "issuer", "fraud_engine"
	Message string
}

func NewTimeoutError(system string) *TimeoutError {
	return &TimeoutError{
		System:  system,
		Message: fmt.Sprintf("%s did not respond in time", system),
	}
}

func (e *TimeoutError) Error() string     { return e.Message }
func (e *TimeoutError) Code() string      { return "TIMEOUT" }
func (e *TimeoutError) HTTPStatus() int   { return 504 }
func (e *TimeoutError) IsRetryable() bool { return true }
