// Package event contiene los Domain Events internos del BC Terminal Gateway.
package event

import (
	"time"

	"github.com/tu-org/posnet-backend/context/terminal-gateway/domain/valueobject"
	"github.com/tu-org/posnet-backend/pkg/domain"
)

// DomainEvent es la interfaz base de los eventos de dominio internos.
type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
}

// ─── SessionCreated ───────────────────────────────────────────────────────────

type SessionCreated struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Amount        domain.Money
	STAN          domain.STAN
	Channel       valueobject.PaymentChannel
	ExpiresAt     time.Time
	occurredAt    time.Time
}

func NewSessionCreated(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	amount domain.Money,
	stan domain.STAN,
	channel valueobject.PaymentChannel,
	expiresAt time.Time,
) SessionCreated {
	return SessionCreated{
		TransactionID: id, TerminalID: tid, MerchantID: mid,
		Amount: amount, STAN: stan, Channel: channel,
		ExpiresAt: expiresAt, occurredAt: time.Now().UTC(),
	}
}

func (e SessionCreated) EventType() string     { return "session.created" }
func (e SessionCreated) OccurredAt() time.Time { return e.occurredAt }

// ─── TransactionInitiated ─────────────────────────────────────────────────────
// Emitido cuando el cliente inicia el pago (escanea el QR o acerca la tarjeta).
// Este evento es el que dispara la publicación de TransactionReceived a NATS.

type TransactionInitiated struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Amount        domain.Money
	STAN          domain.STAN
	Channel       valueobject.PaymentChannel
	ISO8583Raw    []byte
	EMVDataBase64 string
	occurredAt    time.Time
}

func NewTransactionInitiated(
	id domain.TransactionID,
	tid domain.TerminalID,
	mid domain.MerchantID,
	amount domain.Money,
	stan domain.STAN,
	channel valueobject.PaymentChannel,
	iso8583Raw []byte,
	emvDataBase64 string,
) TransactionInitiated {
	return TransactionInitiated{
		TransactionID: id, TerminalID: tid, MerchantID: mid,
		Amount: amount, STAN: stan, Channel: channel,
		ISO8583Raw: iso8583Raw, EMVDataBase64: emvDataBase64,
		occurredAt: time.Now().UTC(),
	}
}

func (e TransactionInitiated) EventType() string     { return "transaction.initiated" }
func (e TransactionInitiated) OccurredAt() time.Time { return e.occurredAt }

// ─── SessionApproved ─────────────────────────────────────────────────────────

type SessionApproved struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	AuthCode      string
	occurredAt    time.Time
}

func NewSessionApproved(id domain.TransactionID, tid domain.TerminalID, authCode string) SessionApproved {
	return SessionApproved{TransactionID: id, TerminalID: tid, AuthCode: authCode, occurredAt: time.Now().UTC()}
}

func (e SessionApproved) EventType() string     { return "session.approved" }
func (e SessionApproved) OccurredAt() time.Time { return e.occurredAt }

// ─── SessionRejected ─────────────────────────────────────────────────────────

type SessionRejected struct {
	TransactionID   domain.TransactionID
	TerminalID      domain.TerminalID
	RejectionCode   string
	RejectionReason string
	occurredAt      time.Time
}

func NewSessionRejected(id domain.TransactionID, tid domain.TerminalID, code, reason string) SessionRejected {
	return SessionRejected{
		TransactionID: id, TerminalID: tid,
		RejectionCode: code, RejectionReason: reason,
		occurredAt: time.Now().UTC(),
	}
}

func (e SessionRejected) EventType() string     { return "session.rejected" }
func (e SessionRejected) OccurredAt() time.Time { return e.occurredAt }

// ─── SessionExpired ───────────────────────────────────────────────────────────

type SessionExpired struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	occurredAt    time.Time
}

func NewSessionExpired(id domain.TransactionID, tid domain.TerminalID) SessionExpired {
	return SessionExpired{TransactionID: id, TerminalID: tid, occurredAt: time.Now().UTC()}
}

func (e SessionExpired) EventType() string     { return "session.expired" }
func (e SessionExpired) OccurredAt() time.Time { return e.occurredAt }

// ─── SessionCancelled ─────────────────────────────────────────────────────────

type SessionCancelled struct {
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	occurredAt    time.Time
}

func NewSessionCancelled(id domain.TransactionID, tid domain.TerminalID) SessionCancelled {
	return SessionCancelled{TransactionID: id, TerminalID: tid, occurredAt: time.Now().UTC()}
}

func (e SessionCancelled) EventType() string     { return "session.cancelled" }
func (e SessionCancelled) OccurredAt() time.Time { return e.occurredAt }
