package valueobject

import "fmt"

// ReceiptPayload contiene los datos estructurados del comprobante de la transacción.
// Es inmutable una vez construido — se serializa a JSON para enviarlo al terminal
// y al webhook del comercio.
type ReceiptPayload struct {
	TransactionID   string
	MerchantName    string
	MerchantAddress string
	TerminalCode    string
	Result          string // "APPROVED" | "REJECTED" | "REVERSED"
	AuthCode        string // Solo si Result == "APPROVED"
	RejectionCode   string // Solo si Result == "REJECTED"
	RejectionReason string // Solo si Result == "REJECTED"
	AmountCents     int64
	Currency        string
	CardLast4       string
	CardNetwork     string
	EntryMode       string
	TransactionAt   string // RFC3339 UTC
}

// NewReceiptPayload crea un ReceiptPayload validando los campos requeridos.
func NewReceiptPayload(
	transactionID, merchantName, terminalCode string,
	result string,
	amountCents int64,
	currency, cardLast4, cardNetwork, entryMode, transactionAt string,
) (ReceiptPayload, error) {
	if transactionID == "" {
		return ReceiptPayload{}, fmt.Errorf("receipt_payload: transaction_id cannot be empty")
	}
	if result != "APPROVED" && result != "REJECTED" && result != "REVERSED" {
		return ReceiptPayload{}, fmt.Errorf("receipt_payload: invalid result %q", result)
	}
	if amountCents <= 0 {
		return ReceiptPayload{}, fmt.Errorf("receipt_payload: amount_cents must be positive")
	}
	return ReceiptPayload{
		TransactionID: transactionID,
		MerchantName:  merchantName,
		TerminalCode:  terminalCode,
		Result:        result,
		AmountCents:   amountCents,
		Currency:      currency,
		CardLast4:     cardLast4,
		CardNetwork:   cardNetwork,
		EntryMode:     entryMode,
		TransactionAt: transactionAt,
	}, nil
}

// IsApproved indica si el comprobante es de una transacción aprobada.
func (r ReceiptPayload) IsApproved() bool { return r.Result == "APPROVED" }
