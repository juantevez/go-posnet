package events

// ReversalRequestedPayload es el payload del evento posnet.transaction.reversal-requested.v1
//
// Publicado por: Terminal Gateway
// Consumido por: Authorization
//
// Se emite cuando el cajero solicita la anulación de una transacción aprobada
// en el mismo día (antes del cierre de lote). Authorization procesa el reversal
// contra el host adquirente y publica ReversalCompletedPayload al finalizar.
type ReversalRequestedPayload struct {
	OriginalTransactionID string `json:"original_transaction_id"` // TransactionID de la tx a anular
	TerminalID            string `json:"terminal_id"`
	MerchantID            string `json:"merchant_id"`
	AmountCents           int64  `json:"amount_cents"`
	Currency              string `json:"currency"`
	OriginalAuthCode      string `json:"original_auth_code"` // Código de auth de la tx original
	RequestedAt           string `json:"requested_at"`       // RFC3339 UTC
}
