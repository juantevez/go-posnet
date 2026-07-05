# ADR 001: Go como lenguaje principal, sobre Java y Rust

## Estado

Aceptado.

## Contexto

POSNET es un sistema de procesamiento de pagos con tarjeta en tiempo real, dividido en 5 Bounded
Contexts que se comunican de forma asíncrona vía NATS JetStream (ver
[docs/architecture/README.md](../architecture/README.md)). Cada BC corre como su propio proceso:
recibe eventos, valida, persiste en Postgres y publica el siguiente evento de la saga.

Los umbrales de referencia del sistema (`LOCUST.md`) fijan la vara alta para la ruta síncrona:

| Métrica | Objetivo (p99 / p95) |
|---|---|
| `GET /healthz` | < 10 ms |
| `GET /transactions/{id}` | < 100 ms |
| Saga completa CHIP (p50 / p95) | < 2 s / < 4 s |
| Saga con monto alto + motor de fraude (p95) | < 5 s |

A esto se suma que cada proceso mantiene cientos de conexiones WebSocket persistentes con
terminales físicos (Terminal Gateway) y docenas de consumers concurrentes de JetStream por BC —
mucha concurrencia de I/O, poca CPU-bound work real por request.

Las opciones consideradas fueron Java (con Spring Boot, la opción "default" en muchos equipos de
pagos), Rust, y Go.

## Decisión

Usamos **Go 1.25** como único lenguaje del backend.

### Frente a Java

- **Arranque y footprint**: los binarios Go arrancan en milisegundos y no necesitan JVM warm-up ni
  JIT — relevante porque en Kubernetes los pods de un BC escalan horizontalmente y se reciclan con
  frecuencia; un arranque lento se traduce directo en latencia de readiness durante un despliegue o
  un evento de autoscaling.
- **Pausas de GC predecibles**: el colector de Go apunta a pausas sub-milisegundo y no requiere el
  tuning de generaciones/heap que sí exige la JVM para sostener p99 bajo en un sistema con SLA de
  segundos en la saga completa. Con montos de tarjeta y datos de autorización en juego, una pausa de
  Full GC de cientos de ms en el momento equivocado es un problema real, no cosmético.
- **Concurrencia sin ceremonia**: goroutines y channels expresan directamente los patrones que este
  sistema usa todo el tiempo — un consumer de JetStream por handler
  (`infrastructure/nats/*_subscriber.go`), un relay de outbox corriendo en su propia goroutine
  (`pkg/outbox.Relay.Run`, ver comentario "llamar en una goroutine independiente") — sin la carga
  cognitiva ni el volumen de código boilerplate que implica levantar y coordinar un thread pool o un
  `ExecutorService` en Java.
- **Un binario, una imagen mínima**: `CGO_ENABLED=0 go build` produce un binario estático que se
  copia a una imagen `alpine` sin runtime adicional (ver `deployments/docker/Dockerfile.*`). Nada de
  imagen base con JDK completo ni gestión de heap size vía flags de contenedor.
- **Costo real, no solo técnico**: Java sigue siendo válido para este dominio (Spring tiene un
  ecosistema de pagos maduro), pero exige más código para el mismo resultado — interfaces,
  DTOs, mappers — y un equipo chico paga ese costo en cada Bounded Context nuevo.

### Frente a Rust

- **Rust habría sido una elección técnicamente defendible** para este dominio — de hecho, más
  segura en cuanto a manejo de memoria. Se descartó por costo de desarrollo, no por rendimiento:
  el borrow checker impone una curva de aprendizaje pronunciada y ralentiza la iteración,
  particularmente en un sistema con 5 BCs que evolucionan en paralelo y un solo equipo
  manteniéndolos.
- **El presupuesto de latencia no lo exige**: los umbrales de la tabla de arriba están dominados
  por I/O (red hacia el adquirente, Postgres, NATS), no por el tiempo de CPU del propio servicio.
  Rust brinda control de bajo nivel (sin GC, layout de memoria explícito) que este sistema no
  necesita explotar para cumplir sus SLAs — el GC de Go no aparece como cuello de botella en los
  umbrales que maneja `LOCUST.md`.
- **Ecosistema**: librerías maduras para lo que este sistema necesita en el día a día — cliente de
  NATS JetStream, `pgx`/`pgxpool` para Postgres, SDK de OpenTelemetry, gRPC — existen y están
  probadas en producción en Go; en Rust habría que asumir más riesgo de integración en varias de
  esas piezas simultáneamente.

## Consecuencias

- La tipificación fuerte de dominio (Value Objects como `Money`, `PAN`, `TransactionID` en
  `pkg/domain`) compensa en buena medida la ausencia de un sistema de tipos tan expresivo como el de
  Rust, a costo de más disciplina manual (constructores que validan, sin invariantes verificadas por
  el compilador).
- No hay borrow checker que prevenga condiciones de carrera en tiempo de compilación — la
  disciplina de concurrencia (qué corre en qué goroutine, qué se comparte) queda en manos de
  revisión de código y de `go test -race`, que corre en CI en cada PR
  (`.github/workflows/ci.yml`).
- El GC de Go es una apuesta consciente: si en el futuro aparece un BC con carga de cómputo intensivo
  (ej. un motor de fraude con modelos más pesados que reglas simples), puede justificar revisar esta
  decisión para ese componente puntual — no para el sistema completo.
