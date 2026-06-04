package events

// AuthorizationApprovedPayload es el payload del evento posnet.auth.approved.v1
//
// Publicado por: Authorization
// Consumido por:
//   - Terminal Gateway  → envía el resultado al terminal vía WebSocket
//   - Settlement        → agrega la transacción al batch del día
//   - Notification      → genera el comprobante y dispara el webhook al comercio
type AuthorizationApprovedPayload struct {
	TransactionID string `json:"transaction_id"`
	TerminalID    string `json:"terminal_id"`
	MerchantID    string `json:"merchant_id"`
	AuthCode      string `json:"auth_code"` // 6 chars alfanuméricos del banco emisor
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	CardLast4     string `json:"card_last4"`
	CardNetwork   string `json:"card_network"`
	EntryMode     string `json:"entry_mode"`
	FraudScore    int    `json:"fraud_score"`   // Score calculado (0–100) — para auditoría
	AuthorizedAt  string `json:"authorized_at"` // RFC3339 UTC
}
