// Package valueobject contiene los Value Objects del BC Authorization.
// Son inmutables y se distinguen por su valor, no por identidad.
package valueobject

import "fmt"

// ─── TransactionState ────────────────────────────────────────────────────────

// TransactionState representa el estado del proceso de autorización.
// Es la máquina de estados central del aggregate Transaction.
type TransactionState string

const (
	StateReceived      TransactionState = "RECEIVED"
	StateFraudChecking TransactionState = "FRAUD_CHECKING"
	StateProcessing    TransactionState = "PROCESSING"
	StateApproved      TransactionState = "APPROVED"
	StateRejected      TransactionState = "REJECTED"
	StateIndeterminate TransactionState = "INDETERMINATE" // Timeout sin respuesta del adquirente
	StateReversed      TransactionState = "REVERSED"
)

// IsTerminal indica si el estado es final (no hay transiciones posibles).
func (s TransactionState) IsTerminal() bool {
	switch s {
	case StateApproved, StateRejected, StateReversed:
		return true
	}
	return false
}

// CanTransitionTo valida si la transición al nuevo estado es válida.
func (s TransactionState) CanTransitionTo(next TransactionState) bool {
	allowed := map[TransactionState][]TransactionState{
		StateReceived:      {StateFraudChecking, StateRejected},
		StateFraudChecking: {StateProcessing, StateRejected},
		StateProcessing:    {StateApproved, StateRejected, StateIndeterminate},
		StateIndeterminate: {StateReversed, StateApproved, StateRejected},
		StateApproved:      {StateReversed},
	}
	for _, a := range allowed[s] {
		if a == next {
			return true
		}
	}
	return false
}

func (s TransactionState) String() string { return string(s) }

// ─── EntryMode ───────────────────────────────────────────────────────────────

// EntryMode indica cómo fue leído el medio de pago en el terminal.
type EntryMode string

const (
	EntryModeChip        EntryMode = "CHIP"
	EntryModeContactless EntryMode = "CONTACTLESS"
	EntryModeMagstripe   EntryMode = "MAGSTRIPE"
	EntryModeManual      EntryMode = "MANUAL"
)

func ParseEntryMode(s string) (EntryMode, error) {
	switch EntryMode(s) {
	case EntryModeChip, EntryModeContactless, EntryModeMagstripe, EntryModeManual:
		return EntryMode(s), nil
	}
	return "", fmt.Errorf("unknown entry mode %q", s)
}

func (e EntryMode) String() string { return string(e) }
