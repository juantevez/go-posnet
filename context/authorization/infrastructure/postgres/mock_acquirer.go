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

// Authorize siempre retorna aprobación con código "AUTH01".
func (m *MockAcquirerGateway) Authorize(
	ctx context.Context,
	tx *aggregate.Transaction,
) (service.AcquirerResponse, error) {
	return service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED, // "00"
		AuthCode:     fmt.Sprintf("A%05d", tx.STAN().Value()),
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
