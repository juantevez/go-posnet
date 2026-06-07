# Terminal Gateway Context

**Punto de conexión entre los terminales POSNET físicos y el sistema**. Gestiona sesiones de pago vía WebSocket con mTLS, traduce mensajes ISO 8583 a eventos de dominio, devuelve resultados de autorización a la pantalla del terminal y coordina el cierre de lote EOD.

Es el único contexto que habla directamente con hardware. Todo el resto del sistema se comunica a través de él.

## Flujo Principal

```
Terminal (WebSocket/mTLS)
    │ ISO 8583
    ▼
Terminal Gateway
    │ crea PaymentSession (TTL 5 min)
    │ traduce ISO 8583 → TransactionReceived event
    │
    ▼ posnet.transaction.received.v1
Authorization Context
    │
    │ posnet.auth.approved.v1 / posnet.auth.rejected.v1
    ▼
Terminal Gateway
    │ actualiza PaymentSession → APPROVED / REJECTED
    │ entrega resultado al terminal vía WebSocket
```

## Dominio

### `PaymentSession` (Aggregate Root)

Sesión de pago con TTL de 5 minutos. Un terminal solo puede tener una sesión activa a la vez.

```
IDLE → AWAITING_PAYMENT → PROCESSING → APPROVED
                                     → REJECTED
                                     → EXPIRED
                                     → CANCELLED
```

**Campos clave:** `id`, `terminalID`, `transactionID`, `channel`, `state`, `expiresAt`

### `Terminal` (Entity)

Terminal físico registrado:

| Campo | Descripción |
|---|---|
| `id` | Identificador único |
| `merchantID` | Comercio al que pertenece |
| `status` | `ACTIVE`, `BLOCKED`, `MAINTENANCE` |
| `certCN` | Common Name del certificado mTLS para autenticación |

### `PaymentChannel`

| Canal | Descripción |
|---|---|
| `QR` | Código QR escaneado por el cliente |
| `NFC` | Contactless / tap |
| `APPLE_PAY` | Apple Pay |
| `GOOGLE_PAY` | Google Pay |
| `MAGSTRIPE` | Banda magnética |

## Arquitectura

```
application/
  create_session.go          → nueva sesión de pago (genera TransactionID)
  process_payment.go         → recibe ISO 8583, publica TransactionReceived
  request_reversal.go        → inicia reversa de transacción aprobada
  cancel_session.go          → cancelación manual por cajero
  request_batch_close.go     → cierre de lote EOD iniciado por terminal
  apply_approval.go          → recibe aprobación, notifica al terminal
  apply_rejection.go         → recibe rechazo, notifica al terminal
  get_session.go             → consulta estado de sesión
  list_active_sessions.go    → sesiones activas (admin)
domain/
  payment_session.go         → aggregate root + state machine
  terminal.go                → entity
  ports.go                   → interfaces (SessionRepository, TerminalRepository, TerminalNotifier, EventPublisher)
infrastructure/
  postgres/                  → sesiones activas y terminales registrados (sqlc)
  websocket/                 → servidor WebSocket mTLS + framing ISO 8583 + heartbeat
  nats/                      → subscriber + publisher JetStream
config/
  config.go                  → carga de env vars
```

## Use Cases

| Use Case | Descripción |
|---|---|
| `CreateSession` | Crea sesión con TTL de 5 min y genera `TransactionID` |
| `ProcessPayment` | Traduce ISO 8583 a evento de dominio y publica a NATS |
| `RequestReversal` | Inicia reversa de transacción aprobada |
| `CancelSession` | Cancelación manual por cajero |
| `RequestBatchClose` | Cierre de lote EOD iniciado desde el terminal |
| `ApplyApproval` | Recibe aprobación → actualiza sesión → notifica terminal |
| `ApplyRejection` | Recibe rechazo → actualiza sesión → notifica terminal |
| `GetSession` | Consulta estado actual de una sesión |
| `ListActiveSessions` | Lista sesiones activas (admin/monitoreo) |

## Eventos NATS

**Publica:**

| Subject | Cuándo |
|---|---|
| `posnet.transaction.received.v1` | Transacción ISO 8583 recibida y parseada |
| `posnet.transaction.reversal-requested.v1` | Terminal solicitó reversa |
| `posnet.transaction.batch-close.v1` | Terminal solicitó cierre de lote |

**Suscribe:**

| Subject | Qué hace |
|---|---|
| `posnet.auth.approved.v1` | Actualiza sesión y envía aprobación al terminal |
| `posnet.auth.rejected.v1` | Actualiza sesión y envía rechazo al terminal |

## Configuración

| Variable | Default | Descripción |
|---|---|---|
| `GRPC_PORT` | `9091` | Puerto gRPC (usado por Notification para SendReceipt) |
| `HTTP_PORT` | `8081` | Puerto health/admin |
| `WS_PORT` | `8082` | Puerto WebSocket para terminales |
| `TLS_CERT_PATH` | — | Certificado del servidor (mTLS) |
| `TLS_KEY_PATH` | — | Clave privada del servidor |
| `TLS_CA_PATH` | — | CA para validar certificados de terminales |
| `SESSION_TTL_SECONDS` | `300` | TTL de sesión de pago (5 min) |
| `SESSION_CLEANUP_EVERY` | `1m` | Frecuencia del job de limpieza de sesiones expiradas |
| `NATS_URL` | — | URL NATS JetStream |
| `POSTGRES_DSN` | — | Cadena de conexión PostgreSQL |

## Correr localmente

```bash
# TLS es opcional en dev (sin variables TLS_* configuradas usa modo inseguro)
go run ./context/terminal-gateway/...
```
