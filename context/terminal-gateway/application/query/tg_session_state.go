// Package valueobject contiene los Value Objects del BC Terminal Gateway.
package valueobject

import "fmt"

// SessionState representa el estado de una sesión de pago activa en el terminal.
// Es la máquina de estados central del aggregate PaymentSession.
type SessionState string

const (
	StateIdle            SessionState = "IDLE"             // Terminal conectado, sin operación activa
	StateAwaitingPayment SessionState = "AWAITING_PAYMENT" // QR en pantalla o lector NFC activo
	StateProcessing      SessionState = "PROCESSING"       // Pago en curso — enviado al backend
	StateApproved        SessionState = "APPROVED"         // Transacción aprobada — estado terminal
	StateRejected        SessionState = "REJECTED"         // Transacción rechazada — estado terminal
	StateExpired         SessionState = "EXPIRED"          // TTL del QR venció sin pago
	StateCancelled       SessionState = "CANCELLED"        // Cajero canceló manualmente
	StateReconnecting    SessionState = "RECONNECTING"     // Reconexión WebSocket en curso
)

// IsTerminal indica si el estado es final — no hay transiciones posibles.
func (s SessionState) IsTerminal() bool {
	switch s {
	case StateApproved, StateRejected, StateExpired, StateCancelled:
		return true
	}
	return false
}

// CanTransitionTo valida si la transición al nuevo estado es válida.
func (s SessionState) CanTransitionTo(next SessionState) bool {
	allowed := map[SessionState][]SessionState{
		StateIdle:            {StateAwaitingPayment},
		StateAwaitingPayment: {StateProcessing, StateExpired, StateCancelled},
		StateProcessing:      {StateApproved, StateRejected, StateReconnecting},
		StateReconnecting:    {StateProcessing, StateApproved, StateRejected},
	}
	for _, a := range allowed[s] {
		if a == next {
			return true
		}
	}
	return false
}

func (s SessionState) String() string { return string(s) }

// ─── PaymentChannel ───────────────────────────────────────────────────────────

// PaymentChannel indica el canal de pago utilizado en la sesión.
type PaymentChannel string

const (
	ChannelQR        PaymentChannel = "QR"
	ChannelNFC       PaymentChannel = "NFC"
	ChannelApplePay  PaymentChannel = "APPLE_PAY"
	ChannelGooglePay PaymentChannel = "GOOGLE_PAY"
	ChannelMagstripe PaymentChannel = "MAGSTRIPE"
)

func ParsePaymentChannel(s string) (PaymentChannel, error) {
	switch PaymentChannel(s) {
	case ChannelQR, ChannelNFC, ChannelApplePay, ChannelGooglePay, ChannelMagstripe:
		return PaymentChannel(s), nil
	}
	return "", fmt.Errorf("unknown payment channel %q", s)
}

func (c PaymentChannel) String() string { return string(c) }

// ToEntryMode convierte el canal al EntryMode equivalente para el mensaje ISO 8583.
func (c PaymentChannel) ToEntryMode() string {
	switch c {
	case ChannelQR:
		return "QR"
	case ChannelNFC, ChannelApplePay, ChannelGooglePay:
		return "CONTACTLESS"
	case ChannelMagstripe:
		return "MAGSTRIPE"
	default:
		return "CHIP"
	}
}
