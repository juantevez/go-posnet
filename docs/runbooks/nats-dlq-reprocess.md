# Runbook: Reprocesamiento de mensajes que agotaron reintentos

## Antes de empezar — cómo funciona la "DLQ" hoy realmente

El stream `POSNET_DLQ` (subject `posnet.dlq.>`) existe en `pkg/natsutil/streams.go`, pero
**ningún código publica ahí hoy**. Un mensaje que agota `MaxDeliver` **no se mueve** a
`POSNET_DLQ` automáticamente — JetStream simplemente deja de reentregarlo a ese consumer. El
mensaje original sigue existiendo en su stream de origen (todos los streams usan
`Retention: LimitsPolicy`, así que el contenido no depende de si algún consumer lo ackeó), acotado
solo por `MaxAge`/tamaño del stream (hoy sin límite — `MaxAge: 0` en todos).

Esto significa que "reprocesar desde la DLQ" hoy es, en la práctica: **encontrar el mensaje
original en su stream de origen y volver a publicarlo** para que el consumer lo vea como una
entrega nueva. Este runbook documenta ese procedimiento manual — no un pipeline automático.

Hay dos formas distintas de que un mensaje deje de reintentarse, y la respuesta correcta es
distinta para cada una:

| Motivo | Qué hizo el subscriber | ¿Reprocesar? |
|---|---|---|
| Error permanente (`*pkgerrors.ValidationError` / `*pkgerrors.ConflictError`) | `msg.Term()` inmediato — ni siquiera cuenta reintentos | Solo después de corregir la causa (payload inválido, o confirmar que el `ConflictError` es un duplicado legítimo que no necesita reprocesarse) |
| Error transitorio repetido hasta agotar `MaxDeliver` | `msg.Nak()` en cada intento, hasta el límite | Sí, una vez resuelta la causa transitoria (Postgres/NATS/red) |

Ver `nak()` en cada `context/{bc}/infrastructure/nats/*_subscriber.go` — todos siguen esta misma
clasificación.

---

## 1. Identificar el mensaje afectado

### Por logs

Buscá el `event_id` en los logs del BC dueño del consumer. Un error permanente deja una sola línea:

```
"permanent error — terminating message" event_id=<uuid> error="..."
```

Un error transitorio deja una línea por cada intento (hasta `MaxDeliver`):

```
"transient error — nacking for retry" event_id=<uuid> error="..."
```

Si ves la segunda varias veces seguidas para el mismo `event_id`, andá contando — cuando llegue al
`MaxDeliver` del consumer (tabla abajo), esa fue la última entrega real.

### Por el consumer en NATS

```bash
nats consumer info <STREAM> <DURABLE>
```

`Num Redelivered` cercano a `Max Deliver` para más mensajes de los que esperás indica que hay
varios atascados. `Num Ack Pending` en 0 con mensajes que sabés que fallaron confirma que ya se
agotaron los reintentos (si siguiera pendiente, JetStream lo seguiría reintentando).

Consumers reales del sistema y su `MaxDeliver` (definidos inline en cada subscriber — ver
[incident-response.md](incident-response.md#31-estado-de-un-consumer) para la tabla completa).

---

## 2. Localizar y recuperar el mensaje original

Los streams no exponen el payload por `event_id` directamente — hay que recorrer por secuencia o
filtrar por subject/tiempo aproximado.

```bash
# Ver los últimos N mensajes del stream/subject en cuestión (sin ackear, no interfiere con el consumer real)
nats stream view <STREAM> --subject '<filter.subject>' --last 50

# Traer un mensaje puntual por número de secuencia (una vez identificado en el paso anterior)
nats stream get <STREAM> <SEQ> --raw > /tmp/msg-<event_id>.json
```

El contenido es el `events.DomainEvent` completo (envelope + `data` en base64/raw JSON según el
`Content-Type`) — el mismo formato que arma `events.Wrap` y que decodifica cada handler con
`events.UnmarshalEnvelope`. Confirmá el `event_id` dentro del JSON (`.event_id`) antes de continuar,
para no reprocesar el mensaje equivocado.

---

## 3. Confirmar y corregir la causa raíz

**No reproceses todavía.** Si la causa era transitoria (Postgres caído, NATS con problemas de red),
confirmá que ya está resuelta (`readyz` del BC en 200, ver
[incident-response.md](incident-response.md)) antes de continuar — si no, vas a volver a agotar los
reintentos con el mismo resultado.

Si la causa fue un `ValidationError` (payload mal formado), el error real puede estar en el
productor del evento — reprocesar el mismo payload sin arreglar el origen va a fallar exactamente
igual. Si fue un `ConflictError`, primero confirmá con `processed_events` que realmente no se
procesó (ver abajo) — si ya se procesó, reprocesar es innecesario y potencialmente doble efecto si
el handler no fuera perfectamente idempotente en ese punto.

```sql
-- Reemplazá {schema} por el schema del BC dueño del consumer
SELECT * FROM {schema}.processed_events WHERE event_id = '<event_id>';
```

Si aparece una fila, el evento ya tuvo efecto — no lo reproceses a menos que estés seguro de que el
efecto se necesita repetir (poco común; normalmente indica que el problema está en otro lado).

---

## 4. Republicar

```bash
nats pub <subject-original> \
  --header "Nats-Msg-Id: <event_id>" \
  < /tmp/msg-<event_id>.json
```

Usar el mismo `event_id` original como `Nats-Msg-Id` es intencional: si por algún motivo JetStream
todavía tuviera ese ID dentro de su ventana de deduplicación (poco probable en un reprocesamiento
manual horas/días después), lo rechazaría en vez de duplicar la publicación. La protección real
contra doble efecto, de todas formas, es la tabla `processed_events` del paso 3 — el republish solo
le da al consumer una nueva oportunidad de entrega.

Confirmá que se procesó viendo aparecer un log de éxito del handler correspondiente, o repitiendo
la consulta a `processed_events` del paso anterior.

---

## 5. Si no hay forma de corregir la causa raíz

Si el evento nunca va a poder procesarse tal cual (ej: hace referencia a una entidad que ya no
existe, o el `ValidationError` refleja un caso que el dominio nunca va a aceptar), no lo reproceses
indefinidamente. Documentá el `event_id` y la razón, y coordiná con el equipo si hace falta una
corrección de datos manual en la tabla afectada en lugar de forzar el evento por NATS.
