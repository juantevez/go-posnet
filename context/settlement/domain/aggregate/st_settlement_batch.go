// Package aggregate contiene los Aggregates del BC Settlement.
package aggregate

import (
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/entity"
	"github.com/juantevez/go-posnet/context/settlement/domain/event"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// SettlementBatch es el Aggregate Root del BC Settlement.
// Representa el lote de transacciones de un terminal en un día.
// Agrupa las transacciones aprobadas y gestiona el ciclo de liquidación.
type SettlementBatch struct {
	id         string
	terminalID domain.TerminalID
	merchantID domain.MerchantID
	batchDate  time.Time // Solo la fecha — truncada a día
	state      valueobject.BatchState
	currency   string

	// Transacciones incluidas en el lote
	transactions []*entity.BatchTransaction

	// Resumen calculado al cierre (nil mientras OPEN)
	summary *valueobject.BatchSummary

	// Timestamps
	createdAt   time.Time
	closedAt    *time.Time
	submittedAt *time.Time
	settledAt   *time.Time

	// Discrepancias detectadas en la conciliación
	discrepancies int

	// Eventos pendientes
	domainEvents []event.DomainEvent
}

// NewSettlementBatch crea un lote nuevo en estado OPEN.
func NewSettlementBatch(
	terminalID domain.TerminalID,
	merchantID domain.MerchantID,
	batchDate time.Time,
	currency string,
) (*SettlementBatch, error) {
	if terminalID.IsZero() {
		return nil, fmt.Errorf("settlement_batch: terminal_id cannot be zero")
	}
	if merchantID.IsZero() {
		return nil, fmt.Errorf("settlement_batch: merchant_id cannot be zero")
	}
	if currency == "" {
		return nil, fmt.Errorf("settlement_batch: currency cannot be empty")
	}

	id := domain.NewTransactionID().String()
	b := &SettlementBatch{
		id:         id,
		terminalID: terminalID,
		merchantID: merchantID,
		batchDate:  batchDate.Truncate(24 * time.Hour),
		state:      valueobject.BatchStateOpen,
		currency:   currency,
		createdAt:  time.Now().UTC(),
	}

	b.record(event.NewBatchOpened(id, terminalID, merchantID, batchDate))
	return b, nil
}

// ─── Mutaciones ───────────────────────────────────────────────────────────────

// AddTransaction agrega una transacción aprobada al lote.
// Solo acepta transacciones mientras el lote está en estado OPEN.
func (b *SettlementBatch) AddTransaction(
	transactionID domain.TransactionID,
	amountCents int64,
	txType valueobject.BatchTransactionType,
) error {
	if b.state != valueobject.BatchStateOpen {
		return fmt.Errorf("settlement_batch %s: cannot add transaction in state %s", b.id, b.state)
	}

	tx, err := entity.NewBatchTransaction(b.id, transactionID, amountCents, b.currency, txType)
	if err != nil {
		return fmt.Errorf("settlement_batch: create batch transaction: %w", err)
	}

	b.transactions = append(b.transactions, tx)
	return nil
}

// RemoveTransaction descuenta una reversión del lote.
// Busca la transacción original y la marca como revertida.
func (b *SettlementBatch) RemoveTransaction(transactionID domain.TransactionID) error {
	if b.state != valueobject.BatchStateOpen {
		return fmt.Errorf("settlement_batch %s: cannot remove transaction in state %s", b.id, b.state)
	}

	// Agregar como REVERSAL negativo
	tx, err := entity.NewBatchTransaction(b.id, transactionID, 1, b.currency, valueobject.BatchTxReversal)
	if err != nil {
		return err
	}
	b.transactions = append(b.transactions, tx)
	return nil
}

// RequestClose inicia el proceso de cierre — transiciona a PENDING_CLOSE.
func (b *SettlementBatch) RequestClose() error {
	if err := b.transition(valueobject.BatchStatePendingClose); err != nil {
		return err
	}
	b.record(event.NewBatchCloseRequested(b.id, b.terminalID, b.merchantID))
	return nil
}

// Close cierra el lote calculando los totales y registrando discrepancias.
// terminalCount y terminalAmount son los totales reportados por el terminal
// para comparar contra los del backend.
func (b *SettlementBatch) Close(terminalCount int, terminalAmount int64) error {
	if err := b.transition(valueobject.BatchStateClosed); err != nil {
		return err
	}

	// Calcular totales del backend
	var backendPurchaseCount, backendReversalCount int
	var backendPurchaseAmount, backendReversalAmount int64

	for _, tx := range b.transactions {
		switch tx.Type() {
		case valueobject.BatchTxPurchase, valueobject.BatchTxOffline:
			backendPurchaseCount++
			backendPurchaseAmount += tx.AmountCents()
		case valueobject.BatchTxReversal:
			backendReversalCount++
			backendReversalAmount += tx.AmountCents()
		}
	}

	backendTotalCount := backendPurchaseCount + backendReversalCount
	backendTotalAmount := backendPurchaseAmount - backendReversalAmount

	// Detectar discrepancias. countDiff mide el desajuste de cantidad; un
	// desajuste de monto por sí solo también debe contar como discrepancia
	// aunque countDiff sea 0, o quedaría enmascarado para el caller que
	// solo chequea Discrepancies() > 0.
	countDiff := abs(terminalCount - backendTotalCount)
	b.discrepancies = 0
	if terminalCount != backendTotalCount || terminalAmount != backendTotalAmount {
		b.discrepancies = countDiff
		if b.discrepancies == 0 {
			b.discrepancies = 1
		}
	}

	// Construir el summary
	cur, _ := domain.ParseCurrency(b.currency)
	totalMoney, _ := domain.NewMoney(backendTotalAmount, cur)
	purchaseMoney, _ := domain.NewMoney(backendPurchaseAmount, cur)
	var reversalMoney domain.Money
	if backendReversalAmount > 0 {
		reversalMoney, _ = domain.NewMoney(backendReversalAmount, cur)
	}

	summary, err := valueobject.NewBatchSummary(
		backendTotalCount, totalMoney,
		backendPurchaseCount, purchaseMoney,
		backendReversalCount, reversalMoney,
	)
	if err != nil {
		return fmt.Errorf("settlement_batch: calculate summary: %w", err)
	}

	b.summary = &summary
	now := time.Now().UTC()
	b.closedAt = &now

	b.record(event.NewBatchClosed(b.id, b.terminalID, b.merchantID, summary, b.discrepancies))
	return nil
}

// Submit marca el lote como enviado al procesador externo.
func (b *SettlementBatch) Submit() error {
	if err := b.transition(valueobject.BatchStateSubmitted); err != nil {
		return err
	}
	now := time.Now().UTC()
	b.submittedAt = &now
	return nil
}

// MarkSettled marca el lote como liquidado exitosamente.
func (b *SettlementBatch) MarkSettled() error {
	if err := b.transition(valueobject.BatchStateSettled); err != nil {
		return err
	}
	now := time.Now().UTC()
	b.settledAt = &now
	b.record(event.NewBatchSettled(b.id, b.terminalID, b.merchantID, *b.summary))
	return nil
}

// MarkDisputed marca el lote con discrepancias que requieren intervención manual.
func (b *SettlementBatch) MarkDisputed(reason string) error {
	if err := b.transition(valueobject.BatchStateDisputed); err != nil {
		return err
	}
	b.record(event.NewBatchDisputed(b.id, b.terminalID, reason))
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (b *SettlementBatch) transition(next valueobject.BatchState) error {
	if !b.state.CanTransitionTo(next) {
		return fmt.Errorf("settlement_batch %s: invalid transition %s → %s", b.id, b.state, next)
	}
	b.state = next
	return nil
}

func (b *SettlementBatch) record(e event.DomainEvent) {
	b.domainEvents = append(b.domainEvents, e)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ─── Getters ──────────────────────────────────────────────────────────────────

func (b *SettlementBatch) ID() string                               { return b.id }
func (b *SettlementBatch) TerminalID() domain.TerminalID            { return b.terminalID }
func (b *SettlementBatch) MerchantID() domain.MerchantID            { return b.merchantID }
func (b *SettlementBatch) BatchDate() time.Time                     { return b.batchDate }
func (b *SettlementBatch) State() valueobject.BatchState            { return b.state }
func (b *SettlementBatch) Currency() string                         { return b.currency }
func (b *SettlementBatch) Transactions() []*entity.BatchTransaction { return b.transactions }
func (b *SettlementBatch) Summary() *valueobject.BatchSummary       { return b.summary }
func (b *SettlementBatch) Discrepancies() int                       { return b.discrepancies }
func (b *SettlementBatch) CreatedAt() time.Time                     { return b.createdAt }
func (b *SettlementBatch) ClosedAt() *time.Time                     { return b.closedAt }
func (b *SettlementBatch) SubmittedAt() *time.Time                  { return b.submittedAt }
func (b *SettlementBatch) SettledAt() *time.Time                    { return b.settledAt }
func (b *SettlementBatch) DomainEvents() []event.DomainEvent        { return b.domainEvents }
func (b *SettlementBatch) ClearDomainEvents()                       { b.domainEvents = nil }
