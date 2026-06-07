# Notification Context

Entrega **notificaciones multi-canal** con reintentos y dead-letter: recibos al terminal vía WebSocket (a través del Terminal Gateway), webhooks al endpoint del comercio y notificaciones de cierre de lote.

No toma decisiones de negocio. Consume eventos del resto del sistema y garantiza que cada notificación llegue o quede en la DLQ para revisión manual.

## Dominio

### `Notification` (Aggregate Root)

```
PENDING → SENT
        → FAILED → RETRYING → SENT
                             → DEAD
```

**Campos clave:** `id`, `transactionID`, `channel`, `state`, `attempts`, `payload`

### `DeliveryAttempt` (Entity)

Cada intento de entrega: timestamp, HTTP status, mensaje de error (si falló).

### Canales (`NotificationChannel`)

| Canal | Destino |
|---|---|
| `TERMINAL_WEBSOCKET` | Terminal físico vía gRPC → Terminal Gateway |
| `WEBHOOK` | HTTP POST al endpoint configurado del comercio |
| `EMAIL` | Email (futuro) |
| `SMS` | SMS (futuro) |

### `ReceiptPayload` (Value Object)

Datos estructurados del recibo: tipo de operación, monto, últimos 4 dígitos de PAN, código de autorización, motivo de rechazo.

## Política de Reintentos (Backoff Exponencial)

| Intento | Espera |
|---|---|
| 1° retry | 30 s |
| 2° retry | 2 min |
| 3° retry | 10 min |
| 4° retry | 1 h |
| 5° intento → `DEAD` | Sin más reintentos |

Un job periódico (cada 1 min, batch de 50) procesa las notificaciones en estado `RETRYING`.

## Arquitectura

```
application/
  notify_approval.go         → recibo de aprobación
  notify_rejection.go        → recibo de rechazo
  notify_batch_closed.go     → resumen de cierre de lote
  retry_failed.go            → job periódico de reintentos
  get_notification.go        → consulta por ID
  get_by_transaction.go      → todas las notificaciones de una transacción
  list_dead.go               → revisión de DLQ
  force_retry.go             → reintento manual de notificaciones DEAD
domain/
  notification.go            → aggregate root + state machine
  delivery_attempt.go        → entity
  receipt_payload.go         → value object
  ports.go                   → interfaces (TerminalNotifier, WebhookDispatcher, EventPublisher)
infrastructure/
  postgres/                  → estado de notificaciones e historial de intentos (sqlc)
  nats/                      → subscriber JetStream
  grpc/                      → cliente gRPC al Terminal Gateway (SendReceipt)
  webhook/                   → HTTP client con timeout configurable
config/
  config.go                  → carga de env vars
```

## Use Cases

| Use Case | Descripción |
|---|---|
| `NotifyApproval` | Crea y despacha recibo de aprobación |
| `NotifyRejection` | Crea y despacha recibo de rechazo |
| `NotifyBatchClosed` | Envía resumen del cierre de lote |
| `RetryFailed` | Job periódico: procesa notificaciones en `RETRYING` |
| `GetNotification` | Consulta por ID |
| `GetByTransactionID` | Todas las notificaciones de una transacción |
| `ListDead` | DLQ para revisión manual |
| `ForceRetry` | Reintento manual de notificaciones `DEAD` |

## Eventos NATS

**Publica:**

| Subject | Cuándo |
|---|---|
| `posnet.notification.dispatched.v1` | Notificación entregada exitosamente (audit trail) |

**Suscribe:**

| Subject | Qué hace |
|---|---|
| `posnet.auth.approved.v1` | Dispara recibo de aprobación |
| `posnet.auth.rejected.v1` | Dispara recibo de rechazo |
| `posnet.settlement.batch-closed.v1` | Notifica cierre de lote al comercio |

## Configuración

| Variable | Default | Descripción |
|---|---|---|
| `GRPC_PORT` | `9094` | Puerto gRPC |
| `HTTP_PORT` | `8084` | Puerto health/admin |
| `POSTGRES_DSN` | — | Cadena de conexión PostgreSQL |
| `NATS_URL` | — | URL NATS JetStream |
| `TERMINAL_GATEWAY_GRPC_ADDR` | — | Dirección gRPC del Terminal Gateway (ej. `terminal-gateway:9091`) |
| `WEBHOOK_TIMEOUT` | `10s` | Timeout HTTP por intento (máx. 30 s) |
| `WEBHOOK_DEFAULT_ENDPOINT` | — | Endpoint de fallback para comercios sin webhook configurado |
| `RETRY_JOB_INTERVAL` | `1m` | Frecuencia del job de reintentos |
| `RETRY_BATCH_SIZE` | `50` | Notificaciones por corrida (rango 1–500) |

## Correr localmente

```bash
go run ./context/notification/...
```
