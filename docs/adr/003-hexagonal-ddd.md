# ADR 003: Hexagonal + DDD, en un monorepo de módulo único

## Estado

Aceptado.

## Contexto

El sistema se divide en 5 Bounded Contexts que representan procesos de negocio genuinamente
distintos (Terminal Gateway, Authorization, Fraud Detection, Settlement, Notification — ver
[docs/architecture/README.md](../architecture/README.md)) con sus propios modelos de estado,
schemas de Postgres y ciclos de cambio. Necesitábamos decidir tres cosas relacionadas pero
separables:

1. **Cómo organizar el código dentro de cada BC** — qué separa reglas de negocio de detalles de
   infraestructura (Postgres, NATS, gRPC, HTTP).
2. **Qué patrones tácticos de DDD aplicar** — Aggregate, Value Object, Repository como interfaz vs.
   implementación concreta.
3. **Cómo distribuir el código en repositorios/módulos** — un repo y un `go.mod` por BC (poli-repo o
   multi-módulo), o un monorepo con módulo único.

## Decisión

### Hexagonal (Ports & Adapters) dentro de cada BC

Cada BC sigue la misma estructura, con una regla de dependencias en una sola dirección:

```
infrastructure → application → domain ← pkg/ (Shared Kernel)
```

```
context/{bc}/
├── domain/             # Núcleo — sin dependencias externas
│   ├── aggregate/      # Aggregate Roots y sus invariantes
│   ├── entity/         # Entidades de dominio
│   ├── valueobject/    # Value Objects (inmutables)
│   ├── repository/     # Puertos de salida hacia BD (interfaces)
│   ├── service/        # Puertos de salida hacia NATS/adquirente (interfaces)
│   └── event/           # Domain Events internos del BC
├── application/        # Casos de uso — orquesta el dominio
│   ├── command/        # Command handlers (escritura)
│   ├── query/           # Query handlers (lectura, CQRS)
│   └── port/            # Puertos de entrada (contratos públicos del BC)
├── infrastructure/      # Adaptadores concretos
│   ├── postgres/        # Repositorio Postgres (pgx/v5 + sqlc)
│   ├── nats/             # Publisher y Subscriber de JetStream
│   ├── grpc/             # Server y clients gRPC
│   └── http/             # Handler REST
└── config/               # Configuración del BC (env vars)
```

Verificamos empíricamente que la regla se sostiene hoy: ningún archivo bajo `domain/` en ninguno de
los 5 BCs importa `application/` ni `infrastructure/`, y ningún BC importa código de otro BC — la
única vía de comunicación entre BCs es NATS JetStream (ADR 002).

**Por qué no una arquitectura en capas clásica (Controller → Service → DAO)**: en ese estilo el
dominio suele terminar dependiendo del framework de persistencia (entidades anotadas para el ORM,
DTOs que se filtran hacia el core). Acá el dominio no sabe que existe Postgres — `domain/repository`
define la interfaz (`PaymentSessionRepository.FindByID(...) (*aggregate.PaymentSession, error)`,
por ejemplo) y `infrastructure/postgres` es quien la implementa. Eso es lo que permite testear cada
BC completo con `pgxmock` y fakes, sin levantar contenedores (ver la sección de testing en
`docs/architecture/README.md`).

### DDD táctico: Aggregate Root + reconstitución explícita

Cada BC tiene exactamente un Aggregate Root con su propia máquina de estados
(`PaymentSession`, `Transaction`, `FraudCase`, `SettlementBatch`, `Notification` — el detalle de
cada una está en `docs/architecture/README.md`). Dos decisiones tácticas se repiten en los 5 BCs:

- **Transiciones de estado validadas en el propio aggregate**, no en el handler de aplicación. Cada
  BC tiene su `CanTransitionTo`/`transition()` que rechaza una transición inválida antes de que
  llegue a tocar la base — el command handler solo orquesta, no decide si la transición es legal.
- **Patrón `Reconstitute`/`ReconstituteParams`** (`*_reconstitute.go` en cada
  `domain/aggregate/`): reconstruye un aggregate en cualquier estado válido a partir de columnas ya
  persistidas, sin correr las validaciones de creación (que solo aplican al `New*` inicial). Esto
  resultó igual de valioso para los tests como para el código de producción: permite construir un
  aggregate en, por ejemplo, `StateProcessing` directamente en un test, sin tener que reproducir
  toda la secuencia de transiciones previas.

**Por qué no un modelo anémico** (aggregates que son solo structs con getters/setters y toda la
lógica en el application layer): la máquina de estados es exactamente el tipo de invariante que DDD
recomienda proteger dentro del aggregate — si "no se puede aprobar una sesión que ya fue
cancelada" viviera en el handler, cada nuevo caso de uso que toque el aggregate tendría que
acordarse de repetir esa validación.

### Monorepo con módulo Go único

Un solo `go.mod` en la raíz, sin `go work` ni un módulo por BC:

```
module github.com/juantevez/go-posnet
go 1.25
```

- **Cambios atómicos en el Shared Kernel**: `pkg/domain`, `pkg/events`, `pkg/errors` son
  contratos que varios BCs consumen (el envelope de eventos, los Value Objects compartidos). Con
  módulos separados, cambiar un payload en `pkg/events` implicaría versionar y publicar un módulo,
  y actualizar la dependencia en cada BC por separado, con la ventana de inconsistencia que eso
  abre entre "publiqué la librería" y "todos los BCs la actualizaron". En monorepo, el
  compilador fuerza que todo el árbol compile con el cambio en el mismo commit.
- **Un único `go test ./...`** ejercita los 5 BCs y el Shared Kernel en una sola corrida de CI (ver
  `.github/workflows/ci.yml`), sin coordinar versiones entre repos.
- **El costo que aceptamos**: cualquier desarrollador puede importar código de otro BC — no hay una
  frontera física (repo separado) que lo impida. Esa frontera es hoy únicamente disciplina de
  revisión de código; no hay un chequeo automático en CI que la haga cumplir (ver Consecuencias).

## Consecuencias

- **La frontera entre BCs no está reforzada por herramientas todavía**: el `go.mod` único significa
  que técnicamente cualquier paquete puede importar cualquier otro — hoy la regla "ningún BC importa
  otro BC" se sostiene porque la verificamos manualmente (grep sobre todo el árbol), no porque haya
  un linter de arquitectura corriendo en CI. Adoptar una herramienta tipo `go-arch-lint` con una
  regla explícita por BC es la extensión natural para que una violación falle el build en lugar de
  depender de que alguien la note en code review.
- **Todos los BCs comparten versión de Go y de cada dependencia** (una sola entrada en `go.sum` por
  librería) — no puede un BC quedarse en una versión vieja de `pgx` mientras otro migra a una
  nueva; la migración es atómica para todo el repo, lo cual es una ventaja de coordinación pero
  también significa que actualizar una dependencia usada por un solo BC obliga a validar que el
  resto sigue compilando.
- **`Reconstitute` es una puerta trasera deliberada** a las invariantes de creación — construye un
  aggregate en cualquier estado sin pasar por `New*`. Su único uso legítimo es reconstruir desde
  filas ya persistidas (que se asume ya pasaron esas validaciones alguna vez) y en tests. Usarlo
  para otra cosa reintroduce exactamente el problema que el aggregate rico busca evitar.
- **Todo lo que un BC necesita de otro debe modelarse como evento explícito** en `pkg/events` — no
  hay atajo de "importar el aggregate del otro BC para leer un campo". Esto es intencional
  (desacopla el ciclo de release de cada BC) pero implica que agregar un dato nuevo a un flujo
  cruzado siempre pasa por extender un payload versionado, nunca por un import directo.
