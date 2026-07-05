# Runbook: Respuesta ante incidentes

## Antes de empezar — qué observabilidad existe hoy realmente

Este runbook asume el estado **actual** del sistema, no el aspiracional. Para no perder tiempo
buscando algo que no existe todavía:

- **No hay dashboards de Prometheus con datos reales.** Cada BC expone `/metrics` (el exporter de
  `pkg/observability.InitMeter` está montado en el router de los 5 BCs), pero ningún BC instancia
  todavía los contadores/histogramas documentados como convención de nombres en
  `pkg/observability/meter.go` (`posnet_transactions_total`, `posnet_nats_consumer_lag`, etc.). Hoy
  ese endpoint solo expone las métricas por defecto del runtime de Go/OTel — **no** los datos de
  negocio del README. No busques un dashboard de Grafana con esas queries: todavía no hay nada que
  las alimente.
- **El collector de OpenTelemetry exporta a `debug` (consola), no a Jaeger/Prometheus reales**
  (`deployments/docker/otel-collector-config.yml`). Las trazas y métricas hoy se ven en el log del
  propio collector, no en una UI.
- **No hay integración de alerting** (Alertmanager, PagerDuty, Slack) en el repo. Si te llegó una
  alerta, vino de una herramienta externa a este repositorio — este runbook no sabe cómo se
  disparó, solo qué hacer una vez que estás mirando el sistema.
- Lo que sí existe y funciona hoy: **health checks HTTP**, **logs estructurados con `slog`**
  correlacionados por `TransactionID`/`EventID`, **trazas OTel** propagadas entre BCs vía headers de
  NATS, y el **CLI de `nats`** para inspeccionar streams y consumers directamente.

Si te toca ser el primer respondedor, tu herramienta principal hoy son los logs (`journalctl`/`docker
logs`/lo que agregue tu plataforma) y consultas directas a Postgres/NATS — no un dashboard.

---

## 1. Triage inicial

### 1.1 ¿Qué servicio está afectado?

Cada BC expone los mismos dos endpoints en su router HTTP
(`context/{bc}/infrastructure/http/*_router.go`):

```bash
curl -s http://<host>:<port>/healthz   # liveness — siempre 200 si el proceso está vivo
curl -s http://<host>:<port>/readyz    # readiness — hace ping real a Postgres (pgutil.HealthCheck)
```

`readyz` en `503` con `{"reason": "database unavailable"}` es la señal más común de "el proceso está
vivo pero no puede servir tráfico" — andá directo a la sección de Postgres.

Puertos por BC: cada uno corre en el puerto configurado por su `config/{bc}_config.go`
(variable de entorno, ver `.env`/`docker-compose.yml`) — no hay un puerto fijo común entre BCs.

### 1.2 ¿Qué tan grave es?

| Severidad | Ejemplo | Acción |
|---|---|---|
| **Crítico** | Un BC no levanta / `readyz` en 503 sostenido / la saga de autorización no avanza para ningún terminal | Página al equipo de guardia, seguí este runbook de punta a punta |
| **Alto** | Un subconjunto de transacciones se queda en un estado no terminal (`PROCESSING`, `FRAUD_CHECKING`) por más de lo esperado | Sección 2 (logs) + sección 3 (NATS) antes de escalar |
| **Medio** | Batches en `DISPUTED` acumulándose, notificaciones en `RETRYING`/`DEAD` | Ver el runbook específico ([batch-close-failure.md](batch-close-failure.md)) o revisar `notification.notifications` por estado |
| **Bajo** | Un solo evento quedó sin procesar (visible en logs como error puntual) | Sección 4 (reprocesamiento) cuando corresponda |

---

## 2. Logs — correlación por transacción

Todos los logs son JSON estructurado (`slog`). El campo más útil para seguir un caso puntual es el
que cada handler agrega al logger antes de escribir (`observability.FromContext(ctx).With(...)`)
— normalmente `transaction_id`, `event_id`, o `terminal_id` según el BC. Si tenés un
`TransactionID` de un reclamo, buscalo en los logs de **todos** los BCs que participan en la saga
(Terminal Gateway → Authorization → Fraud Detection → Settlement/Notification, ver
[docs/architecture/README.md](../architecture/README.md)) — la traza completa de una transacción
cruza los 5 procesos.

Patrones de log a buscar según lo que sospechás:

```
"failed to unmarshal envelope"          → payload de NATS corrupto/inesperado — revisar productor
"permanent error — terminating message" → ValidationError/ConflictError — Term(), no se reintenta
"transient error — nacking for retry"   → error transitorio — JetStream va a reintentar solo
"unmet pgxmock expectations"            → esto es de tests, no debería aparecer en logs de prod
"database unavailable"                  → readyz fallando — ver sección Postgres
```

Si el mismo `event_id` aparece repetidas veces con "transient error — nacking for retry", el
consumer está reintentando sin éxito — pasá a la sección 3 para ver cuántos intentos le quedan
antes de que JetStream deje de redeliverlo.

---

## 3. NATS JetStream

### 3.1 Estado de un consumer

```bash
nats consumer info <STREAM> <DURABLE>
```

Streams y consumers reales del sistema (catálogo completo en `pkg/natsutil/consumers.go` —
aunque `EnsureConsumers` no se invoca hoy desde ningún `cmd/*/wire.go`; cada subscriber crea su
propio consumer al llamar `QueueSubscribe` con las mismas opciones que documenta ese catálogo, así
que **la fuente de verdad real es el `Durable(...)`/`MaxDeliver(...)` que ves en
`context/{bc}/infrastructure/nats/*_subscriber.go`, no el catálogo**, que hoy es solo referencia):

| Durable | Stream | Subject | MaxDeliver |
|---|---|---|---|
| `auth-txn-receiver` | `POSNET_TRANSACTIONS` | `posnet.transaction.received.v1` | 5 |
| `auth-fraud-score-consumer` | `POSNET_FRAUD` | `posnet.fraud.score-calculated.v1` | 5 |
| `auth-reversal-processor` | `POSNET_TRANSACTIONS` | `posnet.transaction.reversal-requested.v1` | 3 |
| `fraud-check-consumer` | `POSNET_FRAUD` | `posnet.fraud.check-requested.v1` | 5 |
| `gateway-auth-consumer` | `POSNET_AUTH` | `posnet.auth.>` | 3 |
| `settlement-auth-consumer` | `POSNET_AUTH` | `posnet.auth.approved.v1` | 5 |
| `settlement-reversal-consumer` | `POSNET_AUTH` | `posnet.auth.reversal-completed.v1` | 5 |
| `settlement-batch-consumer` | `POSNET_TRANSACTIONS` | `posnet.transaction.batch-close.v1` | 5 |
| `notify-auth-consumer` | `POSNET_AUTH` | `posnet.auth.>` | 3 |
| `notify-settlement-consumer` | `POSNET_SETTLEMENT` | `posnet.settlement.>` | 3 |

En la salida de `nats consumer info`, lo que importa:

- `Num Pending` alto y creciendo → el consumer no está llegando a procesar al ritmo que se publica
  (proceso caído, o lento). Revisá `readyz` del BC dueño de ese consumer.
- `Num Redelivered` cerca de `Max Deliver` → mensajes a punto de agotar reintentos. Andá a
  [nats-dlq-reprocess.md](nats-dlq-reprocess.md) antes de que se agoten — después de eso, el
  mensaje deja de redeliverse (los streams son `Retention: LimitsPolicy`, así que el mensaje
  original sigue en el stream, pero nadie lo va a volver a entregar solo).

### 3.2 Estado de un stream

```bash
nats stream info <STREAM>
```

Streams del sistema: `POSNET_TRANSACTIONS`, `POSNET_FRAUD`, `POSNET_AUTH`, `POSNET_SETTLEMENT`,
`POSNET_NOTIFICATION`, `POSNET_DLQ`. Hoy todos corren con `Replicas: 1` (sin tolerancia a la caída
del nodo NATS — ver [ADR 002](../adr/002-nats-jetstream.md)) y `MaxAge: 0` (retención sin límite de
tiempo), así que un stream lleno indica volumen real, no vencimiento de mensajes.

---

## 4. Postgres

Cada BC tiene su propio schema (`{bc}` en minúsculas con guiones bajos:
`terminal_gateway`, `pn_authorization`, `fraud_detection`, `settlement`, `notification`). Todas las
tablas de auditoría de idempotencia se llaman `processed_events` dentro de su schema.

```sql
-- ¿Se procesó ya este evento? (reemplazá el schema según el BC)
SELECT * FROM settlement.processed_events WHERE event_id = '<uuid>';

-- Conexiones activas del pool — útil si readyz falla por agotamiento de conexiones
SELECT count(*), state FROM pg_stat_activity WHERE datname = 'posnet' GROUP BY state;
```

`pkg/pgutil.HealthCheck` (lo que corre `readyz`) es solo un `pool.Ping(ctx)` — si falla, es
conectividad de red o el pool agotado, no un problema de una query puntual.

---

## 5. Cuándo derivar a un runbook específico

| Síntoma | Runbook |
|---|---|
| Batches de liquidación en `DISPUTED` o atascados en `CLOSED` sin pasar a `SUBMITTED` | [batch-close-failure.md](batch-close-failure.md) |
| Mensajes que agotaron `MaxDeliver` y dejaron de redeliverse | [nats-dlq-reprocess.md](nats-dlq-reprocess.md) |

## 6. Después del incidente

- Guardá los `event_id`/`transaction_id` afectados — son la clave para auditar en
  `processed_events` y en los logs de los 5 BCs.
- Si el incidente reveló un gap de recuperación (algo que tuviste que arreglar a mano por SQL
  porque no había una vía self-service), documentalo — varios de esos gaps ya están señalados
  explícitamente en los ADRs y en los otros runbooks; si encontrás uno nuevo, agregalo ahí en vez
  de dejarlo solo en la memoria del que respondió.
