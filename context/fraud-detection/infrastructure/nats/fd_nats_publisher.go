// Package nats contiene los adaptadores NATS del BC Fraud Detection.
package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

// EventPublisher implementa domain/service.EventPublisher usando NATS JetStream.
type EventPublisher struct {
	pub *natsutil.Publisher
}

func NewEventPublisher(pub *natsutil.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

// PublishFraudScoreCalculated publica el score al stream POSNET_FRAUD.
// Consumido por: Authorization BC — continúa o corta la Saga según el score.
func (p *EventPublisher) PublishFraudScoreCalculated(
	ctx context.Context,
	fc *aggregate.FraudCase,
) error {
	payload := events.FraudScoreCalculatedPayload{
		TransactionID: fc.TransactionID().String(),
		Score:         fc.Score().Score(),
		Decision:      fc.Score().Decision().String(),
		RulesHit:      fc.Score().RulesHit(),
		EvaluatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectFraudScoreCalculated,
		events.SubjectFraudScoreCalculated,
		fc.TransactionID().String(),
		"FraudCase",
		fc.TransactionID().String(),
		"",
		payload,
	)
	if err != nil {
		return fmt.Errorf("fd publisher: publish FraudScoreCalculated: %w", err)
	}
	return nil
}
