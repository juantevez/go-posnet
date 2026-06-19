package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/domain/aggregate"
	"github.com/juantevez/go-posnet/context/authorization/domain/service"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
)

// MockAcquirerGateway es un mock del adquirente para desarrollo y MVP.
// Siempre aprueba las transacciones con un auth code fijo.
// Reemplazar por la implementación ISO 8583 real en producción.
type MockAcquirerGateway struct{}

func NewMockAcquirerGateway() *MockAcquirerGateway {
	return &MockAcquirerGateway{}
}

// Authorize siempre retorna aprobación.
// AuthCode: 6 chars alfanuméricos uppercase — formato ISO 8583 DE-38.
// Usamos los últimos 6 dígitos del STAN con padding para garantizar exactamente 6 chars.
func (m *MockAcquirerGateway) Authorize(
	ctx context.Context,
	tx *aggregate.Transaction,
) (service.AcquirerResponse, error) {
	return service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED, // "00"
		AuthCode:     fmt.Sprintf("%06d", tx.STAN().Value()%1000000),
		ARPCBase64:   "",
		RespondedAt:  time.Now().UTC(),
	}, nil
}

// Reverse siempre retorna éxito.
func (m *MockAcquirerGateway) Reverse(
	ctx context.Context,
	tx *aggregate.Transaction,
) error {
	return nil
}
