// Package event contiene los Domain Events internos del BC Settlement.
package event

import (
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// DomainEvent es la interfaz base.
type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
}

// ─── BatchOpened ──────────────────────────────────────────────────────────────

type BatchOpened struct {
	BatchID    string
	TerminalID domain.TerminalID
	MerchantID domain.MerchantID
	BatchDate  time.Time
	occurredAt time.Time
}

func NewBatchOpened(batchID string, tid domain.TerminalID, mid domain.MerchantID, date time.Time) BatchOpened {
	return BatchOpened{BatchID: batchID, TerminalID: tid, MerchantID: mid, BatchDate: date, occurredAt: time.Now().UTC()}
}
func (e BatchOpened) EventType() string     { return "batch.opened" }
func (e BatchOpened) OccurredAt() time.Time { return e.occurredAt }

// ─── BatchCloseRequested ──────────────────────────────────────────────────────

type BatchCloseRequested struct {
	BatchID    string
	TerminalID domain.TerminalID
	MerchantID domain.MerchantID
	occurredAt time.Time
}

func NewBatchCloseRequested(batchID string, tid domain.TerminalID, mid domain.MerchantID) BatchCloseRequested {
	return BatchCloseRequested{BatchID: batchID, TerminalID: tid, MerchantID: mid, occurredAt: time.Now().UTC()}
}
func (e BatchCloseRequested) EventType() string     { return "batch.close_requested" }
func (e BatchCloseRequested) OccurredAt() time.Time { return e.occurredAt }

// ─── BatchClosed ──────────────────────────────────────────────────────────────

type BatchClosed struct {
	BatchID       string
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	Summary       valueobject.BatchSummary
	Discrepancies int
	occurredAt    time.Time
}

func NewBatchClosed(batchID string, tid domain.TerminalID, mid domain.MerchantID, summary valueobject.BatchSummary, discrepancies int) BatchClosed {
	return BatchClosed{
		BatchID: batchID, TerminalID: tid, MerchantID: mid,
		Summary: summary, Discrepancies: discrepancies, occurredAt: time.Now().UTC(),
	}
}
func (e BatchClosed) EventType() string     { return "batch.closed" }
func (e BatchClosed) OccurredAt() time.Time { return e.occurredAt }

// ─── BatchSettled ─────────────────────────────────────────────────────────────

type BatchSettled struct {
	BatchID    string
	TerminalID domain.TerminalID
	MerchantID domain.MerchantID
	Summary    valueobject.BatchSummary
	occurredAt time.Time
}

func NewBatchSettled(batchID string, tid domain.TerminalID, mid domain.MerchantID, summary valueobject.BatchSummary) BatchSettled {
	return BatchSettled{BatchID: batchID, TerminalID: tid, MerchantID: mid, Summary: summary, occurredAt: time.Now().UTC()}
}
func (e BatchSettled) EventType() string     { return "batch.settled" }
func (e BatchSettled) OccurredAt() time.Time { return e.occurredAt }

// ─── BatchDisputed ────────────────────────────────────────────────────────────

type BatchDisputed struct {
	BatchID    string
	TerminalID domain.TerminalID
	Reason     string
	occurredAt time.Time
}

func NewBatchDisputed(batchID string, tid domain.TerminalID, reason string) BatchDisputed {
	return BatchDisputed{BatchID: batchID, TerminalID: tid, Reason: reason, occurredAt: time.Now().UTC()}
}
func (e BatchDisputed) EventType() string     { return "batch.disputed" }
func (e BatchDisputed) OccurredAt() time.Time { return e.occurredAt }
