// Package webhook contiene el adaptador de despacho de webhooks HTTP.
// Implementa domain/service.WebhookDispatcher.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// Dispatcher implementa domain/service.WebhookDispatcher.
// Envía el payload del comprobante al endpoint HTTP del comercio.
type Dispatcher struct {
	client          *http.Client
	defaultEndpoint string // Endpoint por defecto si el comercio no tiene uno configurado
}

// NewDispatcher construye el Dispatcher con el timeout configurado.
func NewDispatcher(timeout time.Duration, defaultEndpoint string) *Dispatcher {
	return &Dispatcher{
		client: &http.Client{
			Timeout: timeout,
		},
		defaultEndpoint: defaultEndpoint,
	}
}

// Dispatch envía el payload de la notificación al endpoint del comercio.
// Retorna el HTTP status code recibido y error si hubo fallo de red.
func (d *Dispatcher) Dispatch(ctx context.Context, n *aggregate.Notification) (int, error) {
	ctx, span := observability.StartSpan(ctx, "webhook.Dispatch")
	defer span.End()

	endpoint := d.resolveEndpoint(n)
	if endpoint == "" {
		return 0, fmt.Errorf("webhook dispatcher: no endpoint configured for merchant %s", n.MerchantID())
	}

	payload, err := json.Marshal(map[string]any{
		"notification_id": n.ID(),
		"transaction_id":  n.TransactionID().String(),
		"merchant_id":     n.MerchantID().String(),
		"channel":         n.Channel().String(),
		"receipt":         n.Receipt(),
		"dispatched_at":   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return 0, fmt.Errorf("webhook dispatcher: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("webhook dispatcher: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Posnet-Notification-ID", n.ID())
	req.Header.Set("X-Posnet-Transaction-ID", n.TransactionID().String())

	resp, err := d.client.Do(req)
	if err != nil {
		observability.RecordError(ctx, err)
		return 0, fmt.Errorf("webhook dispatcher: send request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// resolveEndpoint determina el endpoint del comercio.
// En producción este método consultaría una tabla merchant_webhooks en Postgres.
// Por ahora usa el endpoint por defecto configurado en EngineConfig.
func (d *Dispatcher) resolveEndpoint(n *aggregate.Notification) string {
	// TODO: consultar endpoint específico del comercio desde BD
	// endpoint, err := d.merchantRepo.FindWebhookEndpoint(ctx, n.MerchantID())
	return d.defaultEndpoint
}
