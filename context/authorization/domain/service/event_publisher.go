package service

import (
	"context"

	"github.com/tu-org/posnet-backend/pkg/domain"
	"github.com/tu-org/posnet-backend/context/authorization/domain/aggregate"
)

// EventPublisher es el puerto de salida hacia NATS JetStream.
// Publicar los eventos de dominio de una Transaction al bus de mensajería.
type EventPublisher interface {
	// PublishApproved publica AuthorizationApproved al stream POSNET_AUTH.
	PublishApproved(ctx context.Context, tx *aggregate.Transaction) error

	// PublishRejected publica AuthorizationRejected al stream POSNET_AUTH.
	PublishRejected(ctx context.Context, tx *aggregate.Transaction) error

	// PublishFraudCheckRequested publica FraudCheckRequested al stream POSNET_FRAUD.
	PublishFraudCheckRequested(ctx context.Context, tx *aggregate.Transaction) error

	// PublishReversalCompleted publica ReversalCompleted al stream POSNET_AUTH.
	PublishReversalCompleted(ctx context.Context, txID domain.TransactionID, tx *aggregate.Transaction) error
}
