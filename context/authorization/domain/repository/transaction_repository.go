// Package repository define los puertos de salida (interfaces) del BC Authorization
// hacia la capa de persistencia. El dominio depende de estas interfaces;
// la infraestructura (Postgres) las implementa.
package repository

import (
	"context"
	"time"

	"github.com/tu-org/posnet-backend/pkg/domain"
	"github.com/tu-org/posnet-backend/context/authorization/domain/aggregate"
	"github.com/tu-org/posnet-backend/context/authorization/domain/valueobject"
)

// TransactionRepository es el puerto de salida principal del BC Authorization.
// Define las operaciones de persistencia del aggregate Transaction.
//
// REGLA: esta interface está en el dominio. El adaptador Postgres
// en infrastructure/postgres/repository.go la implementa.
type TransactionRepository interface {
	// Save persiste una Transaction nueva o actualiza una existente.
	// Usa UPSERT internamente para garantizar idempotencia.
	Save(ctx context.Context, tx *aggregate.Transaction) error

	// FindByID recupera una Transaction por su ID.
	// Retorna ErrNotFound si no existe.
	FindByID(ctx context.Context, id domain.TransactionID) (*aggregate.Transaction, error)

	// FindBySTAN recupera una transacción por STAN y terminal en el día actual.
	// Usado para detectar duplicados antes de procesar.
	FindBySTAN(ctx context.Context, terminalID domain.TerminalID, stan domain.STAN, date time.Time) (*aggregate.Transaction, error)

	// UpdateState actualiza solo el estado y el resultado de una Transaction.
	// Más eficiente que Save completo para la mayoría de las transiciones.
	UpdateState(ctx context.Context, id domain.TransactionID, state valueobject.TransactionState) error

	// ExistsByID verifica si una transacción con ese ID ya fue persistida.
	// Usado para idempotencia: si ya existe, no volver a procesar.
	ExistsByID(ctx context.Context, id domain.TransactionID) (bool, error)
}
