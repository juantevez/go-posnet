# Runbook: Fallo en cierre de lote (Settlement)

## Contexto del flujo

Un terminal cierra su lote del día publicando `posnet.transaction.batch-close.v1`
(`pkg/events.SubjectBatchCloseRequested`). El BC Settlement lo consume con el durable
`settlement-batch-consumer` (stream `POSNET_TRANSACTIONS`, `MaxDeliver: 5`) y lo procesa en
`BatchHandler.ProcessBatchClose` (`context/settlement/application/command/st_batch_handler.go`).

Máquina de estados del `SettlementBatch` (`context/settlement/domain/valueobject/st_batch_state.go`):

```
OPEN ──► PENDING_CLOSE ──► CLOSED ──► SUBMITTED ──► SETTLED
                               │           │  ▲
                               └──► DISPUTED ┘  (resubmit tras resolver)
```

Dentro de `ProcessBatchClose`, en orden:

1. Busca el batch `OPEN` del terminal para esa fecha (`FindOpenByTerminal`). Si no hay uno, **loguea
   un warning y retorna sin error** — no es un fallo, pero tampoco pasa nada.
2. `batch.RequestClose()` → `OPEN → PENDING_CLOSE`.
3. `batch.Close(terminalCount, terminalAmount)` → `PENDING_CLOSE → CLOSED`, calcula los totales del
   backend a partir de `batch_transactions` y los compara contra lo que reportó el terminal.
4. Reclama idempotencia (`processed_events`) y **persiste el batch ya en estado `CLOSED`** dentro de
   esa transacción — esto pasa *antes* de decidir si hay discrepancias o de someterlo al procesador.
5. Si `batch.Discrepancies() > 0` → `MarkDisputed(...)` → `CLOSED → DISPUTED`, se guarda de nuevo, y
   termina ahí (no se llama al procesador externo).
6. Si no hay discrepancias → `submitBatch`: llama a `processor.Submit(ctx, batch)` (adquirente/
   procesador externo) y, si acepta, `batch.Submit()` → `CLOSED → SUBMITTED`.

---

## Escenario A — Discrepancia detectada (`DISPUTED`)

**Síntoma**: `GET /batches/{id}` devuelve `state: "DISPUTED"`. En logs: `"batch has discrepancies —
marking as disputed"` con el conteo de `discrepancies`.

**Causa**: el conteo o el monto reportado por el terminal no coincide con lo que Settlement
acumuló en `batch_transactions` para ese lote (`Close()` compara ambos; un desajuste de monto
cuenta como discrepancia aunque el conteo coincida).

**Diagnóstico**:

```sql
-- Totales que ve el backend para el lote
SELECT id, state, total_count, total_amount, purchase_count, purchase_amount,
       reversal_count, reversal_amount, discrepancies
FROM settlement.settlement_batches WHERE id = '<batch_id>';

-- Transacciones individuales que componen el lote
SELECT transaction_id, amount_cents, tx_type, included_at
FROM settlement.batch_transactions WHERE batch_id = '<batch_id>' ORDER BY included_at;
```

Comparar contra lo que el terminal reportó (log del evento `batch-close` original, o el sistema del
terminal) para encontrar la transacción faltante/duplicada/con monto distinto — típicamente una
reversión que no llegó a procesarse a tiempo en Settlement antes del cierre, o una transacción
duplicada por un reintento del terminal que sí se contó dos veces en el backend.

**Resolución**: no hay hoy un comando para "recalcular y reabrir" un batch `DISPUTED` — una vez ahí,
la corrección es manual: conciliar los datos (agregar/eliminar la transacción en disputa vía SQL
directo si se confirma el origen del desajuste) y luego usar `POST /batches/{id}/force-close` (ver
más abajo) para forzar el avance, **con la limitación descripta en el Escenario C**.

---

## Escenario B — "No open batch found" (no-op silencioso)

**Síntoma**: el terminal envió un cierre de lote pero nada cambió. En logs:
`"no open batch found for terminal — nothing to close"`. El mensaje de NATS se **Ackea igual** —
no hay error, no hay reintento, y `GET /merchants/{merchant_id}/batches` puede no mostrar ningún
batch para esa fecha si nunca se creó uno (`AddTransaction` es lo que crea/abre un batch al llegar
la primera transacción del día).

**Causa típica**: el terminal cerró un lote que ya estaba cerrado (reintento de su lado tras un
timeout) o para una fecha en la que nunca llegó ninguna transacción a Settlement.

**Diagnóstico**:

```sql
SELECT id, state, batch_date, closed_at FROM settlement.settlement_batches
WHERE terminal_id = '<terminal_id>' ORDER BY batch_date DESC LIMIT 5;
```

Si ya existe un batch `CLOSED`/`SUBMITTED`/`SETTLED` para esa fecha, el cierre ya se había
procesado — no hay nada que hacer, es el comportamiento esperado (idempotente por resultado, aunque
no pase por la tabla de `processed_events` en este camino en particular). Si no existe *ningún*
batch para la fecha, faltan transacciones previas — revisar por qué no llegaron eventos
`posnet.auth.approved.v1` para ese terminal ese día (ver [incident-response.md](incident-response.md)).

---

## Escenario C — Falla el envío al procesador externo (batch atascado en `CLOSED`)

**Síntoma**: `GET /batches/{id}` muestra `state: "CLOSED"` de forma persistente, nunca avanza a
`SUBMITTED`. En logs, un error propagado desde `submitBatch: processor submit: ...`.

**Esto es lo más delicado de este runbook — leer completo antes de actuar.**

Cuando `processor.Submit()` falla, `ProcessBatchClose` devuelve error y el mensaje de NATS se
reintenta (`Nak`). **Pero el batch ya quedó guardado como `CLOSED` en el paso 4**, que ocurre antes
del intento de envío al procesador. Eso significa que en la redelivery, `FindOpenByTerminal` ya no
encuentra un batch `OPEN` (porque ya está `CLOSED`) — cae directo en el Escenario B ("no open batch
found — nothing to close") y el mensaje se Ackea sin haber reintentado realmente el envío al
procesador. **El mecanismo de reintento de NATS no resuelve este caso**, aunque el error que lo
disparó haya sido transitorio (timeout de red al procesador, por ejemplo).

Tampoco `POST /batches/{id}/force-close` (`AdminHandler.ForceClose`) sirve para este caso puntual:
llama a `batch.RequestClose()` primero, que solo permite la transición `OPEN → PENDING_CLOSE` — un
batch ya `CLOSED` la rechaza, así que `ForceClose` sobre un batch atascado en `CLOSED` falla con un
error de transición de estado.

**Resolución — `POST /batches/{id}/resubmit`** (`AdminHandler.ResubmitBatch`): reintenta el envío al
procesador de un batch en estado `CLOSED`, sin recalcular discrepancias (asume que el batch ya fue
conciliado y solo faltó completar el envío). Requiere `operator_id` en el body para auditoría.

1. Confirmá el diagnóstico:
   ```sql
   SELECT id, state, closed_at, submitted_at FROM settlement.settlement_batches
   WHERE id = '<batch_id>';
   -- state = 'CLOSED', submitted_at IS NULL confirma el escenario
   ```
2. Confirmá que el procesador externo ya está disponible de nuevo (esto no se soluciona solo,
   necesita intervención).
3. Reenviá:
   ```bash
   curl -X POST http://<host>:<port>/batches/<batch_id>/resubmit \
     -H 'Content-Type: application/json' \
     -d '{"operator_id":"<tu_usuario>"}'
   ```
   Si el procesador sigue caído, devuelve `500` con el error propagado — no reintenta solo, hay que
   repetir el `curl` una vez confirmada la disponibilidad. Si el batch no está en `CLOSED` (ya
   `SUBMITTED`/`SETTLED`, o todavía `OPEN`), devuelve `400 VALIDATION_ERROR` sin tocar nada.

---

## Referencia rápida — endpoints disponibles

| Endpoint | Uso |
|---|---|
| `GET /batches/{id}` | Estado y totales de un batch puntual |
| `GET /merchants/{merchant_id}/batches` | Lista de batches de un comercio (requiere fecha) |
| `POST /batches/{id}/force-close` | Fuerza `OPEN/PENDING_CLOSE → CLOSED` con totales del backend (`operator_id` obligatorio, queda en auditoría). **No** resuelve el Escenario C — usar `resubmit` en su lugar |
| `POST /batches/{id}/resubmit` | Reintenta el envío al procesador de un batch `CLOSED` (`operator_id` obligatorio). Es la resolución del Escenario C |

```sql
-- Encontrar todos los batches que necesitan atención (índice ya existe para esta query)
SELECT id, terminal_id, merchant_id, batch_date, state, discrepancies, closed_at, submitted_at
FROM settlement.settlement_batches
WHERE state IN ('DISPUTED', 'CLOSED')
ORDER BY batch_date DESC;
```
