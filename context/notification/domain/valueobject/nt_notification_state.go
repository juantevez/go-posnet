// Package valueobject contiene los Value Objects del BC Notification.
package valueobject

import "fmt"

// NotificationState representa el estado del ciclo de vida de una notificación.
type NotificationState string

const (
	StatePending  NotificationState = "PENDING"  // Creada, pendiente de envío
	StateSent     NotificationState = "SENT"     // Entregada exitosamente — estado terminal
	StateFailed   NotificationState = "FAILED"   // Último intento falló — reintentará
	StateRetrying NotificationState = "RETRYING" // En cola de reintento con backoff
	StateDead     NotificationState = "DEAD"     // Superó MaxDeliver — requiere intervención manual
)

// IsTerminal indica si el estado es final.
func (s NotificationState) IsTerminal() bool {
	return s == StateSent || s == StateDead
}

// CanTransitionTo valida si la transición es válida.
func (s NotificationState) CanTransitionTo(next NotificationState) bool {
	allowed := map[NotificationState][]NotificationState{
		StatePending:  {StateSent, StateFailed},
		StateFailed:   {StateRetrying, StateDead},
		StateRetrying: {StateSent, StateFailed},
	}
	for _, a := range allowed[s] {
		if a == next {
			return true
		}
	}
	return false
}

func (s NotificationState) String() string { return string(s) }

func ParseNotificationState(s string) (NotificationState, error) {
	switch NotificationState(s) {
	case StatePending, StateSent, StateFailed, StateRetrying, StateDead:
		return NotificationState(s), nil
	}
	return "", fmt.Errorf("unknown notification state %q", s)
}

// ─── NotificationChannel ──────────────────────────────────────────────────────

// NotificationChannel indica el canal de entrega de la notificación.
type NotificationChannel string

const (
	ChannelTerminalWebSocket NotificationChannel = "TERMINAL_WEBSOCKET"
	ChannelWebhook           NotificationChannel = "WEBHOOK"
	ChannelEmail             NotificationChannel = "EMAIL"
	ChannelSMS               NotificationChannel = "SMS"
)

func ParseNotificationChannel(s string) (NotificationChannel, error) {
	switch NotificationChannel(s) {
	case ChannelTerminalWebSocket, ChannelWebhook, ChannelEmail, ChannelSMS:
		return NotificationChannel(s), nil
	}
	return "", fmt.Errorf("unknown notification channel %q", s)
}

func (c NotificationChannel) String() string { return string(c) }
