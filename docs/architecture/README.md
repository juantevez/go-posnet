# Arquitectura

Este documento complementa la [visión general del repo](../../README.md#arquitectura) con el detalle
técnico de cómo cada Bounded Context modela su dominio, cómo se comunican entre sí, y qué
garantías de consistencia y confiabilidad ofrece el sistema. Está pensado para quien va a tocar
código, no solo para orientarse en el repo.

---

## Estilo arquitectónico

**Domain-Driven Design + Arquitectura Hexagonal**, en un **monorepo de módulo único** (`go.mod` en
la raíz, sin `go work`). La justificación completa está en
[docs/adr/003-hexagonal-ddd.md](../adr/003-hexagonal-ddd.md).

Regla de dependencias, de afuera hacia adentro:

```
infrastructure → application → domain ← pkg/ (Shared Kernel)
```

- `domain/` es el núcleo: no importa nada de `application/` ni `infrastructure/`, ni de otro BC.
- Ningún BC importa código de otro BC — la única vía de comunicación entre ellos es NATS JetStream.
- `pkg/` es el único código transversal permitido en todos los BCs (ver más abajo).

Cada BC repite la misma estructura interna (`domain/{aggregate,entity,valueobject,repository,service,event}`,
`application/{command,query,port}`, `infrastructure/{postgres,nats,grpc,http}`, `config/`) — ver el
detalle en el [README principal](../../README.md#estructura-hexagonal-por-bc).

---

## Los 5 Bounded Contexts

Cada BC es dueño exclusivo de su modelo de dominio, su schema Postgres (`migrations/{bc}/`) y su
Aggregate Root con una máquina de estados propia. Ningún estado se comparte por referencia; todo
lo que un BC necesita saber de otro llega como payload de un evento NATS.

### Terminal Gateway (`context/terminal-gateway/`)

Conexión WebSocket/mTLS con las terminales POSNET físicas. Traduce mensajes ISO 8583 a eventos de
dominio y mantiene la sesión de pago mientras el terminal espera un resultado.

Aggregate Root: `PaymentSession`, estado (`SessionState`):

```
AWAITING_PAYMENT ──► PROCESSING ──► APPROVED   (terminal)
                          │    └──► REJECTED   (terminal)
                          └──► RECONNECTING ──► PROCESSING / APPROVED / REJECTED
AWAITING_PAYMENT ──► EXPIRED / CANCELLED        (terminal)
```

Es el único BC que hoy usa el patrón **Transactional Outbox** (ver más abajo) para publicar
`TransactionReceived` — el resto de sus eventos (`ReversalRequested`, `BatchCloseRequested`) se
publican directamente vía `natsutil.Publisher`.

### Authorization (`context/authorization/`)

Orquesta la Saga de autorización de punta a punta: dispara el chequeo de fraude, si pasa habla con
el adquirente, y difunde el resultado final a los demás BCs.

Aggregate Root: `Transaction`, estado (`TransactionState`):

```
RECEIVED ──► FRAUD_CHECKING ──► PROCESSING ──► APPROVED
                  │                  │    └──► REJECTED
                  │                  └──► INDETERMINATE  (timeout del adquirente, sin respuesta)
                  └──► REJECTED                            (rechazo directo del motor de fraude)
APPROVED ──► REVERSED
```

### Fraud Detection (`context/fraud-detection/`)

Motor de reglas antifraude, sin estado propio de larga duración por transacción más allá del caso
evaluado. Calcula un score 0–100 y una decisión:

| Score | Decisión | Efecto |
|---|---|---|
| < 50 | `APPROVE` | Continúa hacia el adquirente |
| 50–69 | `REVIEW` | Continúa, pero queda marcada para revisión posterior |
| ≥ 70 | `REJECT` | Se rechaza sin llegar a consultar al adquirente |

Aggregate Root: `FraudCase`, que acumula los resultados de cada regla evaluada y calcula el score
final vía `ApplyEvaluations`.

### Settlement (`context/settlement/`)

Cierre de lote (batch) por terminal, conciliación de totales y generación de remesas.

Aggregate Root: `SettlementBatch`, estado (`BatchState`):

```
OPEN ──► PENDING_CLOSE ──► CLOSED ──► SUBMITTED ──► SETTLED
                               │           │  ▲
                               └──► DISPUTED ┘  (resubmit tras resolver discrepancias)
```

`Close()` detecta discrepancias entre lo esperado y lo acumulado y puede pasar directo a
`DISPUTED` en el mismo paso que cierra el lote.

### Notification (`context/notification/`)

Comprobantes al terminal, webhooks al comercio, alertas operativas.

Aggregate Root: `Notification`, estado (`NotificationState`):

```
PENDING ──► SENT        (terminal)
   │   └──► FAILED ──► RETRYING ──► SENT / FAILED
                  └──► DEAD        (terminal — superó MaxDeliver, requiere intervención manual)
```

---

## Shared Kernel — `pkg/`

Contiene únicamente contratos y tipos transversales, nunca lógica de negocio de un BC específico.
Detalle de paquetes y reglas en el [README principal](../../README.md#shared-kernel--pkg).

Los tipos de error en `pkg/errors` (`ValidationError`, `ConflictError`, `NotFoundError`,
`TimeoutError`, `FraudError`, `AcquirerError`) son el mecanismo que usan los subscribers de NATS
para decidir entre reintentar y descartar un mensaje — ver "Clasificación de errores" más abajo.

---

## Comunicación entre Bounded Contexts

### Streams y subjects (NATS JetStream)

Los streams se declaran en código (`pkg/natsutil/streams.go`, función `allStreamConfigs`) y se
crean de forma idempotente al arrancar cada servicio (`natsutil.EnsureStreams`, invocado desde
`cmd/{bc}/*_wire.go`):

| Stream | Subjects |
|---|---|
| `POSNET_TRANSACTIONS` | `posnet.transaction.>` |
| `POSNET_FRAUD` | `posnet.fraud.>` |
| `POSNET_AUTH` | `posnet.auth.>` |
| `POSNET_SETTLEMENT` | `posnet.settlement.>` |
| `POSNET_NOTIFICATION` | `posnet.notification.>` |
| `POSNET_DLQ` | `posnet.dlq.>` |

> **Estado actual**: todos los streams se crean con `Replicas: 1` y `MaxAge: 0` (sin límite de
> retención por edad) — son los valores por defecto pensados para NATS single-node en desarrollo.
> Diferenciar retención por stream y subir a `Replicas: 3` es trabajo pendiente para producción con
> cluster NATS (el campo `StreamConfig.MaxAge` ya existe para esto, solo falta poblarlo por stream).

Los 11 eventos inter-BC están definidos como constantes en `pkg/events/subjects.go`:

```
posnet.transaction.received.v1
posnet.transaction.reversal-requested.v1
posnet.transaction.batch-close.v1
posnet.fraud.check-requested.v1
posnet.fraud.score-calculated.v1
posnet.auth.approved.v1
posnet.auth.rejected.v1
posnet.auth.reversal-completed.v1
posnet.settlement.batch-closed.v1
posnet.settlement.completed.v1
posnet.notification.dispatched.v1
```

Cada payload es un tipo concreto en `pkg/events` (ej. `AuthorizationApprovedPayload`), envuelto en
un `events.DomainEvent` (envelope común con `EventID`, `EventType`, `AggregateID`, `CorrelationID`,
`OccurredAt`) vía `events.Wrap`/`events.Unwrap[T]`.

### Saga de autorización — flujo de una transacción

```
Terminal POSNET
     │  ISO 8583 / WebSocket (mTLS)
     ▼
[Terminal Gateway]  posnet.transaction.received.v1
     ▼
[Authorization] ──► posnet.fraud.check-requested.v1 ──► [Fraud Detection]
     │              ◄── posnet.fraud.score-calculated.v1 ──┘
     │
     │  score < 70 → ISO 8583 / TCP+TLS al adquirente
     │  score ≥ 70 → rechazo directo, sin llamar al adquirente
     ▼
[Authorization]  posnet.auth.approved.v1  (o .rejected.v1)
     │
     ├──► [Terminal Gateway]  → resultado al terminal por WebSocket
     ├──► [Settlement]        → agrega la transacción al batch abierto del terminal
     └──► [Notification]      → comprobante + webhook al comercio
```

### Reglas de entrega

- **ACK explícito** (`AckExplicit`): el subscriber confirma solo después de persistir en Postgres.
- **Clasificación de errores permanentes vs. transitorios**: cada subscriber (`nak()` en
  `infrastructure/nats/*_subscriber.go`) usa `errors.As` sobre el error devuelto por el handler:
  - `*pkgerrors.ValidationError` / `*pkgerrors.ConflictError` → error permanente → `msg.Term()`
    (no tiene sentido reintentar un payload inválido o un evento ya procesado).
  - Cualquier otro error (timeout de red, Postgres caído) → transitorio → `msg.Nak()`, JetStream
    reintenta según `MaxDeliver`/`AckWait` del consumer.
- **Idempotencia**: antes de aplicar el efecto de un evento, cada handler intenta reclamar el
  `EventID` en la tabla `{schema}.processed_events` (`natsutil.IdempotencyStore.TryMarkAsProcessed`,
  `INSERT ... ON CONFLICT DO NOTHING` dentro de la misma transacción que persiste el aggregate). Si
  el insert no afecta filas, el evento ya fue procesado y se hace `Ack()` sin repetir el efecto.

### Transactional Outbox (`pkg/outbox`)

Usado hoy en **Terminal Gateway** (`cmd/terminal-gateway/tg_wire.go`) para evitar la ventana en la
que un pod puede morir entre "grabé en Postgres" y "publiqué en NATS":

1. El handler escribe el evento en `{schema}.outbox` dentro de la misma transacción que persiste el
   aggregate (`outbox.Store.InsertTx`).
2. `outbox.Relay` corre en background, lee filas pendientes con
   `SELECT ... FOR UPDATE SKIP LOCKED` y las publica a JetStream; si el pod muere entre el insert y
   el publish, el Relay las reintenta en el próximo ciclo (JetStream deduplica por `Nats-Msg-Id`).

Los demás BCs todavía publican directamente vía `natsutil.Publisher` sin pasar por el outbox —
llevarlos al mismo patrón es una extensión natural si aparecen inconsistencias entre lo persistido
y lo publicado.

---

## Comunicación síncrona (gRPC)

Además de los eventos NATS (asíncronos), algunos BCs exponen un puerto gRPC interno
(`infrastructure/grpc/server`) para consultas síncronas puntuales — ver
`context/terminal-gateway/infrastructure/grpc/` como referencia. El contrato está generado en
`pkg/proto/{bc}/v1`. `grpc.NewClient` es lazy (no bloquea ni falla en la construcción del cliente),
lo que se explota en los tests para simular targets inalcanzables sin necesitar un servidor real.

---

## Observabilidad

Cada transacción propaga su `TransactionID` como contexto de traza OpenTelemetry a través de los
headers de los mensajes NATS (`pkg/observability`, `InjectTraceContext`/`ExtractTraceContext`), de
forma que una traza en Jaeger/Tempo puede seguir la transacción a través de los 5 BCs. Ver
[README — Observabilidad](../../README.md#observabilidad) para las métricas Prometheus expuestas.

---

## Estrategia de testing

El sistema se testea sin infraestructura real: `pgxmock` para Postgres (mock de `pgx.Tx`/pool a
nivel de interfaz `pgxPool` acotada por paquete, no el tipo concreto `*pgxpool.Pool`), fakes
hechos a mano para `nats.JetStreamContext` y para los puertos de dominio (repositorios,
notificadores). Esto permite que `go test ./...` corra completo en CI sin levantar contenedores —
ver [.github/workflows/ci.yml](../../.github/workflows/ci.yml). La cobertura de línea por paquete
se mantiene cerca del 100% salvo en ramas defensivas inalcanzables sin infraestructura real
(errores de construcción de clientes gRPC/OTel, timeouts de conexión).

---

## Documentación relacionada

| Documento | Contenido |
|---|---|
| [../adr/001-go-language.md](../adr/001-go-language.md) | Por qué Go sobre Java/Rust |
| [../adr/002-nats-jetstream.md](../adr/002-nats-jetstream.md) | Por qué NATS sobre Kafka |
| [../adr/003-hexagonal-ddd.md](../adr/003-hexagonal-ddd.md) | Hexagonal + DDD + monorepo |
| [../adr/004-sqlc-over-orm.md](../adr/004-sqlc-over-orm.md) | Por qué sqlc sobre GORM |
| [../runbooks/incident-response.md](../runbooks/incident-response.md) | Respuesta ante incidentes |
| [../runbooks/batch-close-failure.md](../runbooks/batch-close-failure.md) | Fallo en cierre de lote |
| [../runbooks/nats-dlq-reprocess.md](../runbooks/nats-dlq-reprocess.md) | Reprocesamiento desde DLQ |
