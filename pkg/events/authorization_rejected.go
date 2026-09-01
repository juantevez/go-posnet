package events

// AuthorizationRejectedPayload es el payload del evento posnet.auth.rejected.v1
//
// Publicado por: Authorization
// Consumido por:
//   - Terminal Gateway  → envía el rechazo al terminal vía WebSocket
//   - Notification      → genera alerta si aplica (ej: fraude detectado)
//
// Source indica el origen del rechazo para que los consumers puedan
// actuar de forma diferente según el caso:
//   - ACQUIRER    → rechazo del banco emisor (código ISO 8583)
//   - FRAUD       → motor antifraude interno
//   - TIMEOUT     → el adquirente no respondió a tiempo
//   - VALIDATION  → error de validación local (formato, límites)
//
// CaptureCard viaja resuelto en el payload — no derivado del código por cada
// consumer — para que ninguno tenga que replicar la semántica ISO 8583 ni
// pueda desincronizarse de la decisión que tomó Authorization.
type AuthorizationRejectedPayload struct {
	TransactionID   string `json:"transaction_id"`
	TerminalID      string `json:"terminal_id"`
	MerchantID      string `json:"merchant_id"`
	RejectionCode   string `json:"rejection_code"`   // ISO 8583: "51","54","05"... o "FRAUD_REJECTED","TIMEOUT"
	RejectionReason string `json:"rejection_reason"` // Descripción legible para logs y comprobante
	IsRetryable     bool   `json:"is_retryable"`     // ¿El cliente puede reintentar con otro medio?
	CaptureCard     bool   `json:"capture_card"`     // "pick-up card": el terminal debe retener la tarjeta (ISO 04/41/43)
	Source          string `json:"source"`           // ACQUIRER | FRAUD | TIMEOUT | VALIDATION
	AmountCents     int64  `json:"amount_cents"`
	Currency        string `json:"currency"`
	CardLast4       string `json:"card_last4"`
	CardNetwork     string `json:"card_network"`
	EntryMode       string `json:"entry_mode"`
	RejectedAt      string `json:"rejected_at"` // RFC3339 UTC
}
