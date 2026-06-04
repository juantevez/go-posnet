package events

// TransactionReceivedPayload es el payload del evento posnet.transaction.received.v1
//
// Publicado por: Terminal Gateway
// Consumido por: Authorization
//
// Se emite cuando el terminal envía un mensaje ISO 8583 al backend
// y el Gateway lo valida y traduce al dominio.
type TransactionReceivedPayload struct {
	TransactionID string `json:"transaction_id"` // UUID v4 — correlationID de toda la Saga
	TerminalID    string `json:"terminal_id"`
	MerchantID    string `json:"merchant_id"`
	AmountCents   int64  `json:"amount_cents"` // Siempre en centavos — nunca float
	Currency      string `json:"currency"`     // ISO 4217: "ARS", "USD"...
	STAN          int    `json:"stan"`         // System Trace Audit Number (1–999999)
	EntryMode     string `json:"entry_mode"`   // CHIP | CONTACTLESS | MAGSTRIPE
	CardLast4     string `json:"card_last4"`   // Solo últimos 4 dígitos
	CardNetwork   string `json:"card_network"` // VISA | MASTERCARD | AMEX | CABAL...
	EMVDataBase64 string `json:"emv_data_b64"` // Tags EMV cifrados en base64 (reenviados al adquirente)
	ISO8583Raw    []byte `json:"iso8583_raw"`  // Mensaje original para auditoría
	ReceivedAt    string `json:"received_at"`  // RFC3339 UTC
}
