# Fraud Detection Context

Motor de **evaluación de riesgo en tiempo real**. Calcula un score de fraude (0–100) para cada transacción usando reglas configurables y decide si aprobar, revisar o rechazar, sin bloquear el camino crítico (timeout máximo: 200 ms).

No llama al adquirente ni toma decisiones de negocio finales: solo provee el score. La decisión final la toma el contexto de Authorization.

## Dominio

### `FraudCase` (Aggregate Root)

Representa el análisis de una transacción. Contiene las evaluaciones individuales de cada regla y el score final consolidado.

### `FraudRule` (Entity)

Regla configurable en caliente (sin redeploy):

| Campo | Descripción |
|---|---|
| `id` | Identificador (`RULE-001`, etc.) |
| `scoreWeight` | Contribución al score total (1–100) |
| `threshold` | Límite de activación de la regla |
| `active` | Habilitada/deshabilitada |

### `FraudScore` (Value Object)

Score final + decisión:

| Rango | Decisión |
|---|---|
| 0–49 | `APPROVE` |
| 50–69 | `REVIEW` |
| 70–100 | `REJECT` |

## Reglas Implementadas

| ID | Nombre | Condición |
|---|---|---|
| `RULE-001` | Velocity | > 60 transacciones/hora en el terminal |
| `RULE-002` | Monto inusual | > 3× el promedio del comercio |
| `RULE-003` | Rechazos múltiples | > 3 rechazos en los últimos 10 min |
| `RULE-004` | Monto repetido | Mismo monto > 1 vez en 5 min |
| `RULE-005` | Magstripe + monto alto | Magstripe con monto > 5.000.000 centavos |

Las reglas se evalúan en **paralelo** con goroutines. Cada una consulta el historial de transacciones y aporta su peso al score final.

## Arquitectura

```
application/
  evaluate_transaction.go    → flujo principal: idempotencia → engine → publicar score
  get_fraud_case.go          → consulta de análisis (admin)
  list_active_rules.go       → reglas configuradas
  update_rule_threshold.go   → hot-update de umbrales
domain/
  fraud_case.go              → aggregate root
  fraud_rule.go              → entity + evaluación
  fraud_score.go             → value object con decisión
  ports.go                   → interfaces (Repositories, EventPublisher)
infrastructure/
  postgres/                  → FraudCase, reglas, historial de transacciones (sqlc)
  nats/                      → subscriber + publisher JetStream
  cache/                     → caché in-memory de reglas (TTL 5 min)
config/
  config.go                  → carga de env vars
```

## Use Cases

| Use Case | Descripción |
|---|---|
| `EvaluateTransaction` | Evalúa reglas en paralelo, persiste `FraudCase`, publica score |
| `GetFraudCase` | Retorna análisis completo con desglose por regla |
| `ListActiveRules` | Lista reglas activas (útil para monitoreo) |
| `UpdateRuleThreshold` | Modifica el umbral de una regla sin redeploy |

## Eventos NATS

**Publica:**

| Subject | Cuándo |
|---|---|
| `posnet.fraud.score-calculated.v1` | Evaluación completada |

**Suscribe:**

| Subject | Qué hace |
|---|---|
| `posnet.fraud.check-requested.v1` | Dispara la evaluación |

## Configuración

| Variable | Default | Descripción |
|---|---|---|
| `GRPC_PORT` | `9092` | Puerto gRPC |
| `HTTP_PORT` | `8082` | Puerto health/admin |
| `POSTGRES_DSN` | — | Cadena de conexión PostgreSQL |
| `NATS_URL` | — | URL NATS JetStream |
| `ENGINE_EVAL_TIMEOUT` | `200ms` | Timeout máximo de evaluación (debe ser < 400ms) |
| `ENGINE_RULES_CACHE_TTL` | `5m` | TTL del caché de reglas |
| `ENGINE_SCORE_THRESHOLD_REJECT` | `70` | Score mínimo para rechazar |
| `ENGINE_SCORE_THRESHOLD_REVIEW` | `50` | Score mínimo para revisión manual |

## Correr localmente

```bash
go run ./context/fraud-detection/...
```
