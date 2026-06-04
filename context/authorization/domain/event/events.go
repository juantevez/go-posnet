// Package event contiene los Domain Events internos del BC Authorization.
// Son distintos de los eventos de integración (pkg/events): estos son
// eventos de dominio locales que el aggregate emite para notificar cambios.
// El adaptador NATS los transforma en eventos de integración al publicar.
package event

import (
	"time"

	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// DomainEvent es la interfaz base de los eventos de dominio internos.
type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
}

// ─── TransactionCreated ───────────────────────────────────────────────────────

type TransactionCreated struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Amount        domain.Money
	STAN          domain.STAN
	occurredAt    time.Time
}

func NewTransactionCreated(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	amount domain.Money,
	stan domain.STAN,
) TransactionCreated {
	return TransactionCreated{
		TransactionID: id,
		TerminalID:    tid,
		MerchantID:    mid,
		Amount:        amount,
		STAN:          stan,
		occurredAt:    time.Now().UTC(),
	}
}

func (e TransactionCreated) EventType() string     { return "transaction.created" }
func (e TransactionCreated) OccurredAt() time.Time { return e.occurredAt }

// ─── FraudCheckStarted ────────────────────────────────────────────────────────

type FraudCheckStarted struct {
	TransactionID domain.TransactionID
	occurredAt    time.Time
}

func NewFraudCheckStarted(id domain.TransactionID) FraudCheckStarted {
	return FraudCheckStarted{TransactionID: id, occurredAt: time.Now().UTC()}
}

func (e FraudCheckStarted) EventType() string     { return "fraud.check.started" }
func (e FraudCheckStarted) OccurredAt() time.Time { return e.occurredAt }

// ─── TransactionApproved ─────────────────────────────────────────────────────

type TransactionApproved struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Amount        domain.Money
	PAN           domain.PAN
	AuthCode      domain.AuthCode
	FraudScore    int
	occurredAt    time.Time
}

func NewTransactionApproved(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	amount domain.Money,
	pan domain.PAN,
	authCode domain.AuthCode,
	fraudScore int,
) TransactionApproved {
	return TransactionApproved{
		TransactionID: id,
		TerminalID:    tid,
		MerchantID:    mid,
		Amount:        amount,
		PAN:           pan,
		AuthCode:      authCode,
		FraudScore:    fraudScore,
		occurredAt:    time.Now().UTC(),
	}
}

func (e TransactionApproved) EventType() string     { return "transaction.approved" }
func (e TransactionApproved) OccurredAt() time.Time { return e.occurredAt }

// ─── TransactionRejected ─────────────────────────────────────────────────────

type TransactionRejected struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	RejectionCode valueobject.RejectionCode
	occurredAt    time.Time
}

func NewTransactionRejected(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	rc valueobject.RejectionCode,
) TransactionRejected {
	return TransactionRejected{
		TransactionID: id,
		TerminalID:    tid,
		MerchantID:    mid,
		RejectionCode: rc,
		occurredAt:    time.Now().UTC(),
	}
}

func (e TransactionRejected) EventType() string     { return "transaction.rejected" }
func (e TransactionRejected) OccurredAt() time.Time { return e.occurredAt }

// ─── TransactionIndeterminate ────────────────────────────────────────────────

type TransactionIndeterminate struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	occurredAt    time.Time
}

func NewTransactionIndeterminate(id domain.TransactionID, tid domain.TerminalID) TransactionIndeterminate {
	return TransactionIndeterminate{TransactionID: id, TerminalID: tid, occurredAt: time.Now().UTC()}
}

func (e TransactionIndeterminate) EventType() string     { return "transaction.indeterminate" }
func (e TransactionIndeterminate) OccurredAt() time.Time { return e.occurredAt }

// ─── TransactionReversed ─────────────────────────────────────────────────────

type TransactionReversed struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Amount        domain.Money
	occurredAt    time.Time
}

func NewTransactionReversed(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	amount domain.Money,
) TransactionReversed {
	return TransactionReversed{
		TransactionID: id,
		TerminalID:    tid,
		MerchantID:    mid,
		Amount:        amount,
		occurredAt:    time.Now().UTC(),
	}
}

func (e TransactionReversed) EventType() string     { return "transaction.reversed" }
func (e TransactionReversed) OccurredAt() time.Time { return e.occurredAt }
