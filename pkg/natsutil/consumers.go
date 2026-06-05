package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// ConsumerConfig define la configuración de un durable consumer de JetStream.
type ConsumerConfig struct {
	Name          string        // Durable name — persiste la posición de lectura
	Stream        string        // Stream al que pertenece
	FilterSubject string        // Subject específico a consumir (puede usar wildcards)
	MaxDeliver    int           // Intentos máximos antes de ir a DLQ (-1 = infinito)
	AckWait       time.Duration // Tiempo máximo para recibir el Ack antes de reentrega
}

// EnsureConsumers crea o actualiza todos los durable consumers del sistema.
// Es idempotente — seguro de llamar en cada arranque del servicio.
// Si el consumer ya existe con la misma configuración, no hace nada.
func EnsureConsumers(js nats.JetStreamContext) error {
	for _, cfg := range allConsumerConfigs() {
		info, err := js.ConsumerInfo(cfg.Stream, cfg.Name)
		if err != nil && err != nats.ErrConsumerNotFound {
			return fmt.Errorf("natsutil: consumer info %q: %w", cfg.Name, err)
		}
		if info != nil {
			// Ya existe — no es necesario actualizar consumers en JetStream 2.x
			// a menos que cambie la configuración. Se deja como está.
			continue
		}
		if _, err := js.AddConsumer(cfg.Stream, &nats.ConsumerConfig{
			Durable:       cfg.Name,
			FilterSubject: cfg.FilterSubject,
			AckPolicy:     nats.AckExplicitPolicy, // Siempre ACK explícito
			MaxDeliver:    cfg.MaxDeliver,
			AckWait:       cfg.AckWait,
			DeliverPolicy: nats.DeliverNewPolicy, // Solo mensajes nuevos desde el arranque
			ReplayPolicy:  nats.ReplayInstantPolicy,
		}); err != nil {
			return fmt.Errorf("natsutil: create consumer %q on stream %q: %w", cfg.Name, cfg.Stream, err)
		}
	}
	return nil
}

// allConsumerConfigs retorna la definición de todos los durable consumers del sistema.
// Un consumer por cada (BC destino × subject que consume).
func allConsumerConfigs() []ConsumerConfig {
	const (
		defaultAckWait  = 30 * time.Second
		criticalDeliver = 5 // Transacciones críticas: 5 intentos
		normalDeliver   = 3 // Flujos normales: 3 intentos
	)

	return []ConsumerConfig{
		// ── BC Authorization ──────────────────────────────────────────────────
		{
			Name:          "auth-txn-receiver",
			Stream:        "POSNET_TRANSACTIONS",
			FilterSubject: "posnet.transaction.received.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},
		{
			Name:          "auth-reversal-processor",
			Stream:        "POSNET_TRANSACTIONS",
			FilterSubject: "posnet.transaction.reversal-requested.v1",
			MaxDeliver:    normalDeliver,
			AckWait:       defaultAckWait,
		},
		{
			Name:          "auth-fraud-score-consumer",
			Stream:        "POSNET_FRAUD",
			FilterSubject: "posnet.fraud.score-calculated.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},

		// ── BC Fraud Detection ────────────────────────────────────────────────
		{
			Name:          "fraud-check-consumer",
			Stream:        "POSNET_FRAUD",
			FilterSubject: "posnet.fraud.check-requested.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},

		// ── BC Terminal Gateway ───────────────────────────────────────────────
		{
			Name:          "gateway-auth-consumer",
			Stream:        "POSNET_AUTH",
			FilterSubject: "posnet.auth.>", // Aprobaciones y rechazos
			MaxDeliver:    normalDeliver,
			AckWait:       defaultAckWait,
		},

		// ── BC Settlement ─────────────────────────────────────────────────────
		{
			Name:          "settlement-auth-consumer",
			Stream:        "POSNET_AUTH",
			FilterSubject: "posnet.auth.approved.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},
		{
			Name:          "settlement-reversal-consumer",
			Stream:        "POSNET_AUTH",
			FilterSubject: "posnet.auth.reversal-completed.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},
		{
			Name:          "settlement-batch-consumer",
			Stream:        "POSNET_TRANSACTIONS",
			FilterSubject: "posnet.transaction.batch-close.v1",
			MaxDeliver:    criticalDeliver,
			AckWait:       defaultAckWait,
		},

		// ── BC Notification ───────────────────────────────────────────────────
		{
			Name:          "notify-auth-consumer",
			Stream:        "POSNET_AUTH",
			FilterSubject: "posnet.auth.>", // Aprobaciones y rechazos
			MaxDeliver:    normalDeliver,
			AckWait:       defaultAckWait,
		},
		{
			Name:          "notify-settlement-consumer",
			Stream:        "POSNET_SETTLEMENT",
			FilterSubject: "posnet.settlement.>", // BatchClosed y SettlementCompleted
			MaxDeliver:    normalDeliver,
			AckWait:       defaultAckWait,
		},
	}
}
