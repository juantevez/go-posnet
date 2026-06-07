# Authorization Context

Orquesta la **saga de autorización** de cada transacción: recibe el evento del Terminal Gateway, solicita la evaluación de fraude, llama al adquirente externo (banco) y publica el resultado al resto del sistema.

Es el núcleo del flujo de pago. No expone un endpoint HTTP al terminal directamente; todo el ingreso llega vía NATS.

## Flujo de la Saga

```
Terminal Gateway
    │ posnet.transaction.received.v1
    ▼
Authorization ──► FraudCheckRequested ──► Fraud Detection
    │                                           │
    │◄──────────── posnet.fraud.score-calculated.v1
    │
    ├─ score < 70 → llama al Adquirente (TCP/TLS ISO 8583)
    │                   ├─ APPROVED → publica posnet.auth.approved.v1
    │                   └─ REJECTED → publica posnet.auth.rejected.v1
    │
    └─ score ≥ 70 → REJECTED directo (sin llamar al adquirente)
```

## Dominio

### `Transaction` (Aggregate Root)

```
RECEIVED → FRAUD_CHECKING → PROCESSING → APPROVED
                                       → REJECTED
                                       → INDETERMINATE
                         → REVERSED
```

**Campos clave:** `id`, `terminalID`, `merchantID`, `amount`, `currency`, `entryMode`, `state`, `rejectionCode`, `fraudDecision`

### Value Objects

| Tipo | Descripción |
|---|---|
| `EntryMode` | `CHIP`, `CONTACTLESS`, `MAGSTRIPE`, `MANUAL` |
| `RejectionCode` | Código ISO 8583 + fuente (`ACQUIRER`, `FRAUD`, `TIMEOUT`, `VALIDATION`) |
| `FraudDecision` | Score (0–100) + decisión (`APPROVE` / `REVIEW` / `REJECT`) |

## Arquitectura

```
application/
  authorize_transaction.go   → entry point de la saga
  apply_fraud_score.go       → recibe resultado del motor de fraude
  process_reversal.go        → reversas
domain/
  transaction.go             → aggregate root + state machine
  ports.go                   → interfaces (Repository, AcquirerGateway, EventPublisher)
infrastructure/
  postgres/                  → repositorio sqlc + pgx/v5
  nats/                      → publisher NATS JetStream
  acquirer/                  → gateway TCP/TLS al adquirente
config/
  config.go                  → carga de env vars
```

## Use Cases

| Use Case | Descripción |
|---|---|
| `AuthorizeTransaction` | Idempotencia → parseo → crea aggregate → persiste → solicita fraud check |
| `ApplyFraudScore` | Aplica la decisión del motor: rechaza o llama al adquirente |
| `ProcessReversal` | Llama al adquirente para reversa → persiste → publica `ReversalCompleted` |

## Eventos NATS

**Publica:**

| Subject | Cuándo |
|---|---|
| `posnet.fraud.check-requested.v1` | Transacción recibida y validada |
| `posnet.auth.approved.v1` | Adquirente aprobó |
| `posnet.auth.rejected.v1` | Fraude o adquirente rechazó |
| `posnet.auth.reversal-completed.v1` | Reversa completada |

**Suscribe:**

| Subject | Qué hace |
|---|---|
| `posnet.transaction.received.v1` | Inicia la saga |
| `posnet.fraud.score-calculated.v1` | Continúa la saga con el score |

## Configuración

| Variable | Default | Descripción |
|---|---|---|
| `GRPC_PORT` | `9090` | Puerto gRPC |
| `HTTP_PORT` | `8080` | Puerto health/admin |
| `POSTGRES_DSN` | — | Cadena de conexión PostgreSQL |
| `NATS_URL` | — | URL NATS JetStream |
| `ACQUIRER_HOST` | — | Host del adquirente externo |
| `ACQUIRER_PORT` | — | Puerto del adquirente |
| `ACQUIRER_TIMEOUT` | `30s` | Timeout de llamada al adquirente |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Tracing OpenTelemetry |

## Correr localmente

```bash
go run ./context/authorization/...
```
