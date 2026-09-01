package domain

// Códigos de respuesta ISO 8583 (DE-39) que instruyen al terminal a retener
// físicamente la tarjeta ("pick-up card").
//
// Viven en el shared kernel porque más de un Bounded Context necesita la misma
// semántica: Authorization la deriva al construir el rechazo, Terminal Gateway
// la usa para instruir al terminal y Notification para escalar la alerta.
// Duplicar la lista en cada BC llevaría a que se desincronicen.
const (
	ISOCaptureCard = "04" // Capture card — el emisor pide retenerla
	ISOLostCard    = "41" // Lost card, pick up
	ISOStolenCard  = "43" // Stolen card, pick up
)

// CodeCardBlocked es el código de rechazo propio (no ISO) que emite la
// blocklist interna. También exige retención: la autoridad para retener no
// viene de este rechazo sino del 41/43 del emisor que originó el bloqueo.
const CodeCardBlocked = "CARD_BLOCKED"

// captureCardCodes es el conjunto de códigos que exigen retención de la tarjeta.
var captureCardCodes = map[string]bool{
	ISOCaptureCard:  true,
	ISOLostCard:     true,
	ISOStolenCard:   true,
	CodeCardBlocked: true,
}

// RequiresCardCapture indica si el código ISO 8583 obliga al terminal a retener
// la tarjeta en lugar de devolverla al portador.
//
// Solo aplica a códigos emitidos por el adquirente: un rechazo local
// (validación, timeout, antifraude) nunca autoriza a retener la tarjeta.
func RequiresCardCapture(isoCode string) bool {
	return captureCardCodes[isoCode]
}
