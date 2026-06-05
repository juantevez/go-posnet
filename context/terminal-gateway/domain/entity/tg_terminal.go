// Package entity contiene las entidades del BC Terminal Gateway.
package entity

import (
	"fmt"
	"time"

	"github.com/tu-org/posnet-backend/pkg/domain"
)

// TerminalStatus indica el estado operativo de un terminal registrado.
type TerminalStatus string

const (
	TerminalActive      TerminalStatus = "ACTIVE"
	TerminalBlocked     TerminalStatus = "BLOCKED"
	TerminalMaintenance TerminalStatus = "MAINTENANCE"
)

// Terminal representa un terminal POSNET físico registrado en el sistema.
// Es una entidad con identidad propia (TerminalID).
// Se persiste en Postgres y se carga al autenticar la conexión WebSocket.
type Terminal struct {
	id            domain.TerminalID
	merchantID    domain.MerchantID
	terminalCode  string // ID físico del terminal (ej: "TRM-0042") — único
	certificateCN string // CN del certificado mTLS del terminal
	status        TerminalStatus
	createdAt     time.Time
	updatedAt     time.Time
}

// NewTerminal crea un Terminal validando las invariantes.
func NewTerminal(
	id domain.TerminalID,
	merchantID domain.MerchantID,
	terminalCode string,
	certificateCN string,
) (*Terminal, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("terminal: id cannot be zero")
	}
	if merchantID.IsZero() {
		return nil, fmt.Errorf("terminal: merchant_id cannot be zero")
	}
	if terminalCode == "" {
		return nil, fmt.Errorf("terminal: terminal_code cannot be empty")
	}
	if certificateCN == "" {
		return nil, fmt.Errorf("terminal: certificate_cn cannot be empty")
	}
	return &Terminal{
		id:            id,
		merchantID:    merchantID,
		terminalCode:  terminalCode,
		certificateCN: certificateCN,
		status:        TerminalActive,
		createdAt:     time.Now().UTC(),
		updatedAt:     time.Now().UTC(),
	}, nil
}

// ReconstitueTerminal reconstruye un Terminal desde la base de datos.
func ReconstitueTerminal(
	id domain.TerminalID,
	merchantID domain.MerchantID,
	terminalCode string,
	certificateCN string,
	status TerminalStatus,
	createdAt time.Time,
	updatedAt time.Time,
) *Terminal {
	return &Terminal{
		id:            id,
		merchantID:    merchantID,
		terminalCode:  terminalCode,
		certificateCN: certificateCN,
		status:        status,
		createdAt:     createdAt,
		updatedAt:     updatedAt,
	}
}

func (t *Terminal) ID() domain.TerminalID         { return t.id }
func (t *Terminal) MerchantID() domain.MerchantID { return t.merchantID }
func (t *Terminal) TerminalCode() string          { return t.terminalCode }
func (t *Terminal) CertificateCN() string         { return t.certificateCN }
func (t *Terminal) Status() TerminalStatus        { return t.status }
func (t *Terminal) CreatedAt() time.Time          { return t.createdAt }
func (t *Terminal) UpdatedAt() time.Time          { return t.updatedAt }

// IsActive indica si el terminal puede procesar transacciones.
func (t *Terminal) IsActive() bool { return t.status == TerminalActive }

// Block bloquea el terminal (fraude detectado, solicitud del comercio).
func (t *Terminal) Block() {
	t.status = TerminalBlocked
	t.updatedAt = time.Now().UTC()
}
