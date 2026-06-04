package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// EnsureStreams crea o actualiza todos los streams del sistema POSNET.
// Es idempotente — seguro de llamar en cada arranque del servicio.
// Si el stream ya existe con la misma configuración, no hace nada.
func EnsureStreams(js nats.JetStreamContext) error {
	for _, cfg := range allStreamConfigs() {
		info, err := js.StreamInfo(cfg.Name)
		if err != nil && err != nats.ErrStreamNotFound {
			return fmt.Errorf("natsutil: stream info %q: %w", cfg.Name, err)
		}
		if info != nil {
			// El stream ya existe — actualizar configuración si cambió.
			if _, err := js.UpdateStream(cfg); err != nil {
				return fmt.Errorf("natsutil: update stream %q: %w", cfg.Name, err)
			}
			continue
		}
		// Crear el stream.
		if _, err := js.AddStream(cfg); err != nil {
			return fmt.Errorf("natsutil: create stream %q: %w", cfg.Name, err)
		}
	}
	return nil
}

// allStreamConfigs devuelve la definición de los 5 streams del sistema.
func allStreamConfigs() []*nats.StreamConfig {
	return []*nats.StreamConfig{
		{
			Name:      "POSNET_TRANSACTIONS",
			Subjects:  []string{"posnet.transaction.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    7 * 24 * time.Hour,
			Replicas:  3,
			Storage:   nats.FileStorage,
			Discard:   nats.DiscardOld,
		},
		{
			Name:      "POSNET_FRAUD",
			Subjects:  []string{"posnet.fraud.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    72 * time.Hour,
			Replicas:  3,
			Storage:   nats.FileStorage,
		},
		{
			Name:      "POSNET_AUTH",
			Subjects:  []string{"posnet.auth.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    7 * 24 * time.Hour,
			Replicas:  3,
			Storage:   nats.FileStorage,
		},
		{
			// WorkQueue: cada mensaje es entregado a exactamente un consumer.
			// Ideal para Settlement donde no queremos procesamiento duplicado.
			Name:      "POSNET_SETTLEMENT",
			Subjects:  []string{"posnet.settlement.>"},
			Retention: nats.WorkQueuePolicy,
			MaxAge:    30 * 24 * time.Hour,
			Replicas:  3,
			Storage:   nats.FileStorage,
		},
		{
			Name:      "POSNET_NOTIFICATION",
			Subjects:  []string{"posnet.notification.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    48 * time.Hour,
			Replicas:  2,
			Storage:   nats.FileStorage,
		},
		{
			// Dead Letter Queue — mensajes que superaron MaxDeliver.
			Name:     "POSNET_DLQ",
			Subjects: []string{"posnet.dlq.>"},
			MaxAge:   30 * 24 * time.Hour,
			Replicas: 2,
			Storage:  nats.FileStorage,
		},
	}
}
