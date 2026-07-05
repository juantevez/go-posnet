# ADR 004: sqlc sobre un ORM (GORM), para el acceso a Postgres

## Estado

Aceptado — parcialmente implementado (ver "Estado actual" en Consecuencias).

## Contexto

Cada BC persiste su propio Aggregate Root en Postgres a través de un repositorio concreto en
`infrastructure/postgres/` (`PaymentSessionRepo`, `TransactionRepo`, `FraudCaseRepo`,
`SettlementBatchRepo`, `NotificationRepo`). El acceso es siempre vía `pgx/v5`/`pgxpool`, nunca
`database/sql` genérico — eso ya estaba decidido de entrada por rendimiento y por el soporte nativo
de tipos de Postgres que da `pgx`. La pregunta de este ADR es una capa más arriba: **cómo se
escriben y mapean las queries** — con un ORM (GORM, el más usado en Go), a mano, o generadas desde
SQL con `sqlc`.

Las columnas de cada tabla no son un mapeo directo del aggregate: hay Value Objects (`Money` como
centavos `int64` + `Currency`, `STAN`, `TransactionID`) que se serializan a columnas primitivas y
se reconstruyen del otro lado vía el patrón `Reconstitute` (ver ADR 003) — no vía tags de struct
sobre la propia entidad de dominio.

## Decisión

**Nunca usamos GORM ni ningún ORM.** El acceso a datos se escribe como SQL explícito ejecutado con
`pgx.Tx`/`*pgxpool.Pool` (o una interfaz local acotada a los métodos que cada repo necesita —
`Exec`/`QueryRow`/`Begin`, ver el patrón `pgxPool` repetido en cada `infrastructure/postgres/`), y
se planea generar el boilerplate repetitivo de esas queries con `sqlc` a partir de SQL versionado,
no de un ORM que infiera el schema desde structs Go.

### Por qué no GORM (ni un ORM en general)

- **El aggregate no es la fila**: un ORM asume que tu entidad de dominio y tu fila de tabla son
  prácticamente lo mismo (con tags `gorm:"column:..."`). Acá el aggregate expone Value Objects
  (`session.Amount() domain.Money`, `session.STAN() domain.STAN`) que no tienen una columna 1:1 —
  `Amount` se persiste como dos columnas (`amount_cents`, `currency`) y se reconstruye con
  `domain.NewMoney(amountCents, cur)`. Forzar eso a través de tags de ORM significa terminar
  escribiendo hooks de serialización custom de todas formas — en la práctica, el mismo trabajo que
  escribir el `Scan` a mano, pero con una capa de "magia" del ORM encima que hay que entender
  primero.
- **Control total sobre el SQL que corre en un sistema de pagos**: un `UPDATE` generado
  dinámicamente por un ORM (`Save()` que decide qué columnas tocar según qué campos cambiaron) es
  más difícil de auditar que un `UPDATE ... SET state = $1, closed_at = $2` escrito a mano. En un
  dominio donde una escritura equivocada mueve dinero, preferimos que la query que se ejecuta sea
  exactamente la que está en el archivo `.go`, sin interpretación de por medio.
- **`ON CONFLICT DO UPDATE` explícito**: los repos usan upserts con `ON CONFLICT (id) DO UPDATE SET
  ...` actualizando solo un subconjunto de columnas (estado, `auth_code`, `closed_at` — nunca
  `created_at` ni las columnas inmutables). Expresar exactamente esa semántica en la API de un ORM
  típicamente exige salirse del ORM igual (raw SQL embebido) — en cuyo caso el ORM no está
  aportando nada para el caso que más importa.

### Por qué sqlc como capa de generación (no `database/sql` a mano para siempre)

- **Type-safety sin reflection en runtime**: `sqlc` genera código Go a partir de SQL real,
  chequeado contra el schema en tiempo de generación — un error de tipo entre una columna y el
  campo Go de destino se detecta al correr `sqlc generate`, no en producción con un `Scan` que
  falla en un tipo que no calza (el mismo tipo de error que dejamos documentado como riesgo en los
  tests de esta sesión: "los tipos deben calzar exactamente con los destinos de `Scan`, pgxmock no
  convierte tipos como pgx real").
- **El SQL sigue siendo la fuente de verdad**: a diferencia de un ORM, con `sqlc` seguís escribiendo
  el `.sql` vos mismo — el generador solo produce el binding a Go (structs de fila + firma de
  función), no decide qué query correr. Mantiene la ventaja de auditabilidad de la sección anterior.
- **Reduce el boilerplate real que sí molesta**: no elimina la necesidad de `Reconstitute` (eso
  sigue siendo responsabilidad del dominio, no de la capa de datos), pero sí elimina escribir a
  mano el `Scan(&id, &terminalID, ..., &closedAt)` columna por columna para cada query nueva —
  que es donde históricamente aparecen los bugs más tontos (un campo desalineado del `SELECT`).

## Consecuencias

- **Estado actual (importante)**: hoy el código NO usa `sqlc` generado todavía. Cada
  `infrastructure/postgres/{bc}` tiene un directorio `sqlc/` y otro `query/` reservados
  (`.gitkeep`, sin contenido) y no existe un `sqlc.yaml` en el repo. Todas las queries de los 5 BCs
  están escritas a mano con SQL embebido en constantes Go y `Scan()` explícito columna por columna
  (`scanSession`/`scanTerminal` en Terminal Gateway son el ejemplo probado en esta sesión de tests,
  pero el patrón se repite en Authorization, Fraud Detection, Settlement y Notification). Este ADR
  documenta la dirección elegida — SQL explícito + generación futura, nunca ORM — no un estado ya
  completado. Adoptar `sqlc` de verdad implica: escribir las queries en `query/*.sql`, un
  `sqlc.yaml` por BC (o uno global con múltiples paquetes), y migrar cada repo del `Scan` manual a
  las funciones generadas.
- **El costo de no tener sqlc todavía**: cada nuevo campo en una tabla implica tocar a mano el
  `SELECT`, el `Scan()` y el `struct` de parámetros en el repo correspondiente — exactamente el
  trabajo repetitivo que se buscaba evitar. El riesgo de un desalineamiento entre columnas del
  `SELECT` y argumentos de `Scan()` es real y hoy se mitiga solo con tests (`pgxmock` fila por fila,
  ver los tests de `infrastructure/postgres/*_test.go`), no con generación de código.
- **Las migraciones tampoco pasan por una herramienta de schema versionado activa**:
  `pkg/pgutil.Migrate` es hoy un no-op explícito para el MVP — las tablas se crean vía el script de
  init de Postgres (`deployments/docker/docker-entrypoint-initdb.d/01-init.sql`) y los archivos
  `*_schema.sql` de cada BC. Esto es relevante para `sqlc` porque su generación típica parte de un
  schema SQL de referencia: los `*_schema.sql` ya existentes por BC son ese punto de partida
  natural el día que se active la generación.
- **La interfaz `pgxPool` acotada por paquete** (solo los métodos que cada repo necesita —
  `Exec`/`QueryRow`/`Begin`, nunca el tipo concreto `*pgxpool.Pool`) es independiente de esta
  decisión y se mantiene igual con o sin `sqlc`: el código generado también recibe un `DBTX`
  (interfaz mínima) en vez de depender de un pool concreto, así que adoptar `sqlc` no exige
  revertir el trabajo de testabilidad ya hecho en cada repositorio.
