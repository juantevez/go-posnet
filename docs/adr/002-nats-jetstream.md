# ADR 002: NATS JetStream como bus de eventos, sobre Kafka y RabbitMQ

## Estado

Aceptado.

## Contexto

Los 5 Bounded Contexts (ver [docs/architecture/README.md](../architecture/README.md)) no se llaman
entre sí directamente — toda la comunicación inter-BC es asíncrona, vía eventos publicados en un
bus compartido. Ese bus tiene que darnos:

- **Persistencia con reintento**: si Settlement está caído cuando Authorization publica
  `posnet.auth.approved.v1`, el evento tiene que seguir ahí cuando Settlement vuelva — no es
  pub/sub efímero.
- **Entrega al menos una vez con deduplicación**: los handlers son idempotentes por diseño
  (`{schema}.processed_events`, ver ADR de arquitectura), pero el transporte también deduplica del
  lado del broker para evitar procesamiento redundante innecesario.
- **Reintento con clasificación de errores**: un error transitorio (Postgres caído) debe reintentar;
  un payload inválido o un evento ya procesado no debe reintentar para siempre (`Term()` vs
  `Nak()`, ver `nak()` en cada `infrastructure/nats/*_subscriber.go`).
- **Baja huella operativa**: un solo equipo mantiene 5 servicios Go; el broker no puede exigir un
  clúster de coordinación aparte a mantener con el mismo esfuerzo que los propios BCs.

Las opciones evaluadas fueron Apache Kafka, RabbitMQ y NATS JetStream.

## Decisión

Usamos **NATS JetStream** como bus de eventos y **RPC gRPC directo** para las consultas síncronas
puntuales que sí lo necesitan.

### Frente a Kafka

- **Overhead operativo desproporcionado para el volumen real**: Kafka brilla con throughput de
  cientos de miles de mensajes/segundo y necesita ZooKeeper (o KRaft) más brokers dimensionados en
  consecuencia. Los umbrales de este sistema (`LOCUST.md`: sagas de a decenas por segundo en carga
  de prueba) no justifican esa infraestructura ni el conocimiento operativo que exige mantenerla
  sana (rebalanceos de partición, tuning de retención de log-segments, etc.).
  JetStream corre embebido en el mismo binario de `nats-server`, sin proceso de coordinación
  externo.
- **El cliente Go de JetStream es más simple para lo que hacemos**: `nats.go` expone
  `QueueSubscribe` con `Durable`, `AckExplicit`, `MaxDeliver`, `AckWait` como opciones directas —
  el mapeo con lo que necesitábamos (un durable consumer con reintento acotado por handler, ver
  `context/*/infrastructure/nats/*_subscriber.go`, todos con `AckWait(30*time.Second)` y
  `MaxDeliver` configurable) fue casi 1:1. El cliente Go de Kafka (`sarama`/`kafka-go`) exige más
  conceptos propios (consumer groups, offsets manuales, rebalance listeners) para lograr lo mismo.
- **Dedup nativo suficiente para nuestro caso**: JetStream deduplica por header `Nats-Msg-Id`
  dentro de una ventana configurable — lo usamos poniendo el `EventID` del envelope como
  `Nats-Msg-Id` en cada publish (`pkg/natsutil/publisher.go`, `pkg/outbox/outbox.go`). Kafka no
  tiene un equivalente de "no vuelvas a aceptar este mensaje si ya lo viste" a nivel de broker —
  la deduplicación ahí es responsabilidad exclusiva del productor/consumidor, que es exactamente
  el trabajo extra que JetStream nos ahorra en el borde.
- Kafka sigue siendo la elección correcta si este sistema necesitara reprocesar meses de historial
  completo como fuente de verdad (event sourcing a gran escala) o alimentar pipelines de analytics
  de streaming — no es el caso hoy.

### Frente a RabbitMQ

- **RabbitMQ es un exchange/queue clásico, no un log**: una vez que un mensaje se consume y se
  ackea, desaparece. JetStream retiene el stream (con política de límites configurable por
  `MaxAge`/tamaño, ver `pkg/natsutil/streams.go`), lo que nos deja reproducir eventos para
  diagnóstico o reconciliar un BC que estuvo caído un rato, sin depender de una Dead Letter Queue
  como único mecanismo de recuperación.
- **Un solo binario para colas y streams**: NATS Core (pub/sub simple) y JetStream (persistente,
  con streams y consumers) conviven en el mismo `nats-server`. No hace falta correr un broker
  aparte para casos que no necesitan persistencia — aunque hoy todo el tráfico inter-BC pasa por
  JetStream para tener la misma garantía de entrega en todos los flujos.
- **Autenticación e infraestructura ya resuelta en el mismo paquete**: `nats.go` trae soporte nativo
  para NKey (Ed25519, `pkg/crypto/nkey.go`, `pkg/natsutil/connect.go`) y TLS 1.3 sin plugins
  adicionales — en RabbitMQ, mTLS + un esquema de autenticación equivalente exige más configuración
  externa al broker.

## Consecuencias

- **Streams como catálogo cerrado**: los 6 streams (`POSNET_TRANSACTIONS`, `POSNET_FRAUD`,
  `POSNET_AUTH`, `POSNET_SETTLEMENT`, `POSNET_NOTIFICATION`, `POSNET_DLQ`) se declaran en código
  (`pkg/natsutil/streams.go`) y se crean de forma idempotente al arrancar cada BC
  (`natsutil.EnsureStreams`) — no hay UI de administración separada del código fuente, lo cual es
  intencional (el catálogo de streams versiona junto con el código que los usa).
- **Replicas=1 hoy**: la configuración actual asume NATS single-node (documentado en el propio
  comentario de `EnsureStreams`). Pasar a un clúster real de NATS con `Replicas: 3` es un cambio de
  configuración, no de arquitectura — pero significa que hoy no hay tolerancia a la caída del único
  nodo de NATS en producción; es una brecha conocida, no una limitación del diseño.
- **DLQ es manual, no automática todavía**: `POSNET_DLQ` existe como stream, pero mover ahí los
  mensajes que agotan `MaxDeliver` requiere un paso explícito de reprocesamiento — ver
  [../runbooks/nats-dlq-reprocess.md](../runbooks/nats-dlq-reprocess.md).
- **Menor ecosistema de terceros que Kafka**: herramientas de observabilidad, conectores a
  data warehouses y tooling de terceros para Kafka superan ampliamente a los de NATS. Si en el
  futuro este sistema necesita alimentar un pipeline de analítica/BI a gran escala desde el bus de
  eventos, es un punto a revisar — probablemente como un consumer adicional que puentee eventos
  seleccionados hacia Kafka, no como reemplazo del bus interno.
