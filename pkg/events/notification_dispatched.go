package events

// NotificationDispatchedPayload es el payload del evento posnet.notification.dispatched.v1
//
// Publicado por: Notification
// Consumido por: (auditoría — ningún BC lo consume en el flujo normal)
//
// Evento de auditoría que confirma que una notificación fue enviada
// exitosamente. Permite rastrear el ciclo de vida completo de cada
// notificación: cuántos intentos se necesitaron y por qué canal salió.
type NotificationDispatchedPayload struct {
	NotificationID string `json:"notification_id"`
	TransactionID  string `json:"transaction_id"`
	Channel        string `json:"channel"`       // TERMINAL_WEBSOCKET | WEBHOOK | EMAIL | SMS
	Attempts       int    `json:"attempts"`      // Cantidad de intentos hasta el éxito
	DispatchedAt   string `json:"dispatched_at"` // RFC3339 UTC
}
