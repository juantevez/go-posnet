package events

// ─── Subjects (NATS) ─────────────────────────────────────────────────────────
// Nomenclatura: posnet.{dominio}.{evento}.{versión}

const (
	SubjectTransactionReceived   = "posnet.transaction.received.v1"
	SubjectReversalRequested     = "posnet.transaction.reversal-requested.v1"
	SubjectBatchCloseRequested   = "posnet.transaction.batch-close.v1"
	SubjectFraudCheckRequested   = "posnet.fraud.check-requested.v1"
	SubjectFraudScoreCalculated  = "posnet.fraud.score-calculated.v1"
	SubjectAuthApproved          = "posnet.auth.approved.v1"
	SubjectAuthRejected          = "posnet.auth.rejected.v1"
	SubjectReversalCompleted     = "posnet.auth.reversal-completed.v1"
	SubjectBatchClosed           = "posnet.settlement.batch-closed.v1"
	SubjectSettlementCompleted   = "posnet.settlement.completed.v1"
	SubjectNotificationDispatched = "posnet.notification.dispatched.v1"
)

// ─── TransactionReceivedPayload ───────────────────────────────────────────────
// Publicado por: Terminal Gateway
// Consumido por: Authorization

type TransactionReceivedPayload struct {
	TransactionID string `json:"transaction_id"`
	TerminalID    string `json:"terminal_id"`
	MerchantID    string `json:"merchant_id"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	STAN          int    `json:"stan"`
	EntryMode     string `json:"entry_mode"`    // CHIP | CONTACTLESS | MAGSTRIPE
	CardLast4     string `json:"card_last4"`
	CardNetwork   string `json:"card_network"`
	EMVDataBase64 string `json:"emv_data_b64"`  // Tags EMV en base64 (para reenvío al adquirente)
	ISO8583Raw    []byte `json:"iso8583_raw"`   // Mensaje original para auditoría
	ReceivedAt    string `json:"received_at"`   // RFC3339 UTC
}

// ─── ReversalRequestedPayload ────────────────────────────────────────────────
// Publicado por: Terminal Gateway
// Consumido por: Authorization

type ReversalRequestedPayload struct {
	OriginalTransactionID string `json:"original_transaction_id"`
	TerminalID            string `json:"terminal_id"`
	MerchantID            string `json:"merchant_id"`
	AmountCents           int64  `json:"amount_cents"`
	Currency              string `json:"currency"`
	OriginalAuthCode      string `json:"original_auth_code"`
	RequestedAt           string `json:"requested_at"`
}

// ─── BatchCloseRequestedPayload ──────────────────────────────────────────────
// Publicado por: Terminal Gateway
// Consumido por: Settlement

type BatchCloseRequestedPayload struct {
	TerminalID     string `json:"terminal_id"`
	MerchantID     string `json:"merchant_id"`
	BatchDate      string `json:"batch_date"`      // "2025-06-04"
	TerminalCount  int    `json:"terminal_count"`  // Total de tx reportadas por el terminal
	TerminalAmount int64  `json:"terminal_amount"` // Total en centavos reportado por el terminal
	RequestedAt    string `json:"requested_at"`
}

// ─── FraudCheckRequestedPayload ──────────────────────────────────────────────
// Publicado por: Authorization
// Consumido por: Fraud Detection

type FraudCheckRequestedPayload struct {
	TransactionID string `json:"transaction_id"`
	TerminalID    string `json:"terminal_id"`
	MerchantID    string `json:"merchant_id"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	CardNetwork   string `json:"card_network"`
	EntryMode     string `json:"entry_mode"`
	OccurredAt    string `json:"occurred_at"` // RFC3339 UTC
}

// ─── FraudScoreCalculatedPayload ─────────────────────────────────────────────
// Publicado por: Fraud Detection
// Consumido por: Authorization

type FraudScoreCalculatedPayload struct {
	TransactionID string   `json:"transaction_id"`
	Score         int      `json:"score"`          // 0–100
	Decision      string   `json:"decision"`       // APPROVE | REJECT | REVIEW
	RulesHit      []string `json:"rules_hit"`      // IDs de reglas que activaron
	EvaluatedAt   string   `json:"evaluated_at"`
}

// ─── AuthorizationApprovedPayload ────────────────────────────────────────────
// Publicado por: Authorization
// Consumido por: Terminal Gateway, Settlement, Notification

type AuthorizationApprovedPayload struct {
	TransactionID string `json:"transaction_id"`
	TerminalID    string `json:"terminal_id"`
	MerchantID    string `json:"merchant_id"`
	AuthCode      string `json:"auth_code"`      // 6 chars del emisor
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	CardLast4     string `json:"card_last4"`
	CardNetwork   string `json:"card_network"`
	EntryMode     string `json:"entry_mode"`
	FraudScore    int    `json:"fraud_score"`
	AuthorizedAt  string `json:"authorized_at"`
}

// ─── AuthorizationRejectedPayload ────────────────────────────────────────────
// Publicado por: Authorization
// Consumido por: Terminal Gateway, Notification

type AuthorizationRejectedPayload struct {
	TransactionID  string `json:"transaction_id"`
	TerminalID     string `json:"terminal_id"`
	MerchantID     string `json:"merchant_id"`
	RejectionCode  string `json:"rejection_code"`   // ISO 8583: "51","54","05"...
	RejectionReason string `json:"rejection_reason"` // Descripción legible
	IsRetryable    bool   `json:"is_retryable"`
	Source         string `json:"source"`           // FRAUD | ACQUIRER | TIMEOUT | VALIDATION
	RejectedAt     string `json:"rejected_at"`
}

// ─── ReversalCompletedPayload ────────────────────────────────────────────────
// Publicado por: Authorization
// Consumido por: Settlement, Notification

type ReversalCompletedPayload struct {
	OriginalTransactionID string `json:"original_transaction_id"`
	TerminalID            string `json:"terminal_id"`
	MerchantID            string `json:"merchant_id"`
	AmountCents           int64  `json:"amount_cents"`
	Currency              string `json:"currency"`
	CompletedAt           string `json:"completed_at"`
}

// ─── BatchClosedPayload ───────────────────────────────────────────────────────
// Publicado por: Settlement
// Consumido por: Notification

type BatchClosedPayload struct {
	BatchID        string `json:"batch_id"`
	TerminalID     string `json:"terminal_id"`
	MerchantID     string `json:"merchant_id"`
	BatchDate      string `json:"batch_date"`
	TotalCount     int    `json:"total_count"`
	TotalAmount    int64  `json:"total_amount"`
	Currency       string `json:"currency"`
	Discrepancies  int    `json:"discrepancies"` // Cantidad de gaps detectados
	ClosedAt       string `json:"closed_at"`
}

// ─── SettlementCompletedPayload ───────────────────────────────────────────────
// Publicado por: Settlement
// Consumido por: Notification

type SettlementCompletedPayload struct {
	MerchantID      string `json:"merchant_id"`
	SettlementDate  string `json:"settlement_date"`
	TotalBatches    int    `json:"total_batches"`
	TotalAmount     int64  `json:"total_amount"`
	Currency        string `json:"currency"`
	NetAmount       int64  `json:"net_amount"`  // Después de MDR
	MDRPercent      float64 `json:"mdr_percent"`
	CompletedAt     string `json:"completed_at"`
}

// ─── NotificationDispatchedPayload ───────────────────────────────────────────
// Publicado por: Notification
// Es un evento de auditoría — confirma que la notificación fue enviada.

type NotificationDispatchedPayload struct {
	NotificationID string `json:"notification_id"`
	TransactionID  string `json:"transaction_id"`
	Channel        string `json:"channel"`     // TERMINAL_WEBSOCKET | WEBHOOK | EMAIL | SMS
	Attempts       int    `json:"attempts"`
	DispatchedAt   string `json:"dispatched_at"`
}
