package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// StreamConfig define la configuración de un stream de JetStream.
type StreamConfig struct {
	Name     string
	Subjects []string
	MaxAge   int64 // segundos
}

// EnsureStreams crea o verifica todos los streams del sistema.
// Es idempotente — seguro de llamar en cada arranque.
// Replicas=1 para desarrollo local (single-node NATS).
// En producción con NATS cluster, aumentar a 3.
func EnsureStreams(js nats.JetStreamContext) error {
	for _, cfg := range allStreamConfigs() {
		_, err := js.StreamInfo(cfg.Name)
		if err == nil {
			// Ya existe — no modificar
			continue
		}
		if err != nats.ErrStreamNotFound {
			return fmt.Errorf("natsutil: stream info %q: %w", cfg.Name, err)
		}

		// Crear el stream — Replicas=1 para dev local
		_, err = js.AddStream(&nats.StreamConfig{
			Name:      cfg.Name,
			Subjects:  cfg.Subjects,
			MaxAge:    time.Duration(cfg.MaxAge) * time.Second, // 0 = sin límite de retención por edad
			Replicas:  1, // ← 1 para single-node; cambiar a 3 en producción con cluster
			Storage:   nats.FileStorage,
			Retention: nats.LimitsPolicy,
		})
		if err != nil {
			return fmt.Errorf("natsutil: create stream %q: %w", cfg.Name, err)
		}
	}
	return nil
}

// allStreamConfigs retorna la definición de todos los streams del sistema.
func allStreamConfigs() []StreamConfig {
	return []StreamConfig{
		{
			Name:     "POSNET_TRANSACTIONS",
			Subjects: []string{"posnet.transaction.>"},
		},
		{
			Name:     "POSNET_FRAUD",
			Subjects: []string{"posnet.fraud.>"},
		},
		{
			Name:     "POSNET_AUTH",
			Subjects: []string{"posnet.auth.>"},
		},
		{
			Name:     "POSNET_SETTLEMENT",
			Subjects: []string{"posnet.settlement.>"},
		},
		{
			Name:     "POSNET_NOTIFICATION",
			Subjects: []string{"posnet.notification.>"},
		},
		{
			Name:     "POSNET_DLQ",
			Subjects: []string{"posnet.dlq.>"},
		},
	}
}
