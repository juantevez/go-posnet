package events

// ReversalCompletedPayload es el payload del evento posnet.auth.reversal-completed.v1
//
// Publicado por: Authorization
// Consumido por:
//   - Settlement    → descuenta la transacción del batch del día
//   - Notification  → genera el comprobante de anulación
type ReversalCompletedPayload struct {
	OriginalTransactionID string `json:"original_transaction_id"` // TransactionID de la tx anulada
	TerminalID            string `json:"terminal_id"`
	MerchantID            string `json:"merchant_id"`
	AmountCents           int64  `json:"amount_cents"` // Monto revertido (siempre positivo)
	Currency              string `json:"currency"`
	CompletedAt           string `json:"completed_at"` // RFC3339 UTC
}
