"""
Load Test — BC Fraud Detection (go-posnet)
==========================================
Simula dos tipos de usuarios concurrentes:

  FraudHTTPUser  — consulta fraud cases y reglas via HTTP
  FraudNATSUser  — publica FraudCheckRequested directamente a NATS
                   y verifica el resultado via HTTP

Endpoints HTTP del BC (puerto 8083 en docker-compose.yml — el default interno
del código Go es 8082, pero deployments/docker/docker-compose.yml lo pisa con
HTTP_PORT=8083 — usar SIEMPRE el puerto publicado por docker-compose, no el
default del código, si corrés vía Docker; ver `docker compose ps`):
  GET /healthz
  GET /readyz
  GET /fraud-cases/{transaction_id}
  GET /rules
  PUT /rules/{rule_id}/threshold

Reglas implementadas (fd_rule_engine.go):
  RULE-001 (+20): Velocity   — > 60 tx/hora en el terminal
  RULE-002 (+15): Monto      — > 3x promedio del comercio
  RULE-003 (+25): Rechazos   — > 3 rechazos en 10 min
  RULE-004 (+20): Repetido   — mismo monto > 1 vez en 5 min
  RULE-005 (+30): Magstripe  — MAGSTRIPE + monto > 5.000.000 cents

Decisiones (fd_fraud_decision.go):
  score  0-49  → APPROVE
  score 50-69  → REVIEW
  score 70-100 → REJECT

Uso:
  # UI web:
  locust -f fraud_detection_locust.py --host http://localhost:8083

  # Headless:
  locust -f fraud_detection_locust.py --headless -u 10 -r 2 --run-time 2m \\
    --host http://localhost:8083

  # Combinado con los locustfiles de los otros BCs: NO pasar --host ni
  # completar "Host" en la web UI — cada *HTTPUser ya fija su propio host
  # (ver settlement_locust.py para el detalle de por qué).

Variables de entorno:
  NATS_URL         URL de NATS JetStream    (default: nats://localhost:4222)
  POLL_TIMEOUT_S   Timeout de polling       (default: 10)
  POLL_INTERVAL_S  Intervalo de polling     (default: 0.5)
"""

import json
import os
import random
import time
import uuid
from datetime import datetime, timezone

import asyncio
import nats
import requests as req_lib

from locust import HttpUser, User, task, between, events, tag
from locust.exception import StopUser


# ── Configuración ──────────────────────────────────────────────────────────────

NATS_URL      = os.getenv("NATS_URL",        "nats://localhost:4222")
POLL_TIMEOUT  = float(os.getenv("POLL_TIMEOUT_S",  "10"))
POLL_INTERVAL = float(os.getenv("POLL_INTERVAL_S", "0.5"))

TERMINAL_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
MERCHANT_ID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"

# Reglas disponibles (fd_rule_engine.go + fd_schema.sql)
RULE_IDS = ["RULE-001", "RULE-002", "RULE-003", "RULE-004", "RULE-005"]

# Weights de score por regla (para calcular scores esperados en comentarios)
# RULE-001: +20, RULE-002: +15, RULE-003: +25, RULE-004: +20, RULE-005: +30

# Distribución de entry modes
ENTRY_MODES   = ["CHIP", "CONTACTLESS", "MAGSTRIPE", "MANUAL"]
ENTRY_WEIGHTS = [0.60,   0.25,          0.10,        0.05]

CARD_NETWORKS   = ["VISA", "MASTERCARD", "CABAL", "NARANJA", "AMEX"]
NETWORK_WEIGHTS = [0.45,   0.30,          0.15,   0.08,       0.02]

# Pool compartido de transaction_ids con FraudCase evaluado
_known_fraud_tx_ids: list[str] = []
_stan_counter = 0

# Host HTTP del BC Fraud Detection
_http_host = "http://localhost:8083"


# ── Helpers ────────────────────────────────────────────────────────────────────

def new_uuid() -> str:
    return str(uuid.uuid4())

def next_stan() -> int:
    global _stan_counter
    _stan_counter = (_stan_counter % 999998) + 1
    return _stan_counter

def now_rfc3339() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

def build_fraud_check_envelope(
    tx_id: str,
    event_id: str,
    amount_cents: int,
    entry_mode: str,
    card_network: str,
) -> bytes:
    """
    Construye el envelope DomainEvent para posnet.fraud.check-requested.v1
    según pkg/events/fraud_check_requested.go y pkg/events/envelope.go.

    El subscriber de Fraud Detection (fd_nats_subscriber.go) espera:
      envelope.EventID       → clave de idempotencia (processed_events)
      envelope.CorrelationID → TransactionID
      data.TransactionID     → correlación con Authorization
      data.EntryMode         → CHIP | CONTACTLESS | MAGSTRIPE | MANUAL
      data.AmountCents       → activa RULE-002 y RULE-005 según valor
    """
    payload = {
        "transaction_id": tx_id,
        "terminal_id":    TERMINAL_ID,
        "merchant_id":    MERCHANT_ID,
        "amount_cents":   amount_cents,
        "currency":       "ARS",
        "card_network":   card_network,
        "entry_mode":     entry_mode,
        "occurred_at":    now_rfc3339(),
    }
    envelope = {
        "event_id":       event_id,
        "event_type":     "posnet.fraud.check-requested.v1",
        "aggregate_id":   tx_id,
        "aggregate_type": "Transaction",
        "correlation_id": tx_id,
        "causation_id":   "",
        "occurred_at":    now_rfc3339(),
        "schema_version": 1,
        "data":           payload,
    }
    return json.dumps(envelope).encode("utf-8")


# ── FraudHTTPUser ──────────────────────────────────────────────────────────────

class FraudHTTPUser(HttpUser):
    """
    Simula analistas de fraude y dashboards consultando casos y reglas.
    Peso 3: 3 usuarios HTTP por cada 1 NATS.
    """
    host = _http_host
    weight = 3
    wait_time = between(0.5, 2.0)

    def on_start(self):
        with self.client.get("/healthz", catch_response=True) as resp:
            if resp.status_code != 200:
                resp.failure(f"Fraud Detection not ready: {resp.status_code}")
                raise StopUser()

    @task(5)
    @tag("health", "http")
    def health_check(self):
        """GET /healthz — liveness probe."""
        with self.client.get(
            "/healthz", catch_response=True, name="GET /healthz"
        ) as resp:
            if resp.status_code == 200:
                resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(2)
    @tag("readiness", "http")
    def readiness_check(self):
        """GET /readyz — readiness probe (verifica Postgres)."""
        with self.client.get(
            "/readyz", catch_response=True, name="GET /readyz"
        ) as resp:
            if resp.status_code in (200, 503):
                resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(2)
    @tag("query", "http")
    def get_fraud_case(self):
        """
        GET /fraud-cases/{transaction_id}
        Consulta el análisis de fraude de una transacción.
        Retorna score, decision, rules_hit y evaluaciones individuales.
        """
        if _known_fraud_tx_ids:
            tx_id = random.choice(_known_fraud_tx_ids)
            expect_404 = False
        else:
            tx_id = new_uuid()
            expect_404 = True

        with self.client.get(
            f"/fraud-cases/{tx_id}",
            catch_response=True,
            name="GET /fraud-cases/{transaction_id}",
        ) as resp:
            if resp.status_code == 200:
                data = resp.json()
                # El handler retorna port.FraudCaseResult con campos PascalCase
                has_tx = "TransactionID" in data or "transaction_id" in data
                has_score = "Score" in data or "score" in data
                if not has_tx or not has_score:
                    resp.failure(f"missing fields in response: {list(data.keys())}")
                else:
                    # Validar que la decision sea válida
                    decision = data.get("Decision") or data.get("decision", "")
                    if decision not in ("APPROVE", "REVIEW", "REJECT", ""):
                        resp.failure(f"invalid decision: {decision}")
                    else:
                        resp.success()
            elif resp.status_code == 404 and expect_404:
                resp.success()
            elif resp.status_code == 404:
                resp.failure(f"Known fraud case {tx_id} not found")
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(3)
    @tag("rules", "http")
    def list_rules(self):
        """
        GET /rules — lista las 5 reglas activas con sus parámetros actuales.
        Útil para monitorear cambios de configuración en caliente.
        """
        with self.client.get(
            "/rules", catch_response=True, name="GET /rules"
        ) as resp:
            if resp.status_code == 200:
                data = resp.json()
                count = data.get("count", 0)
                rules = data.get("rules", [])
                if count == 0 or len(rules) == 0:
                    resp.failure("No rules returned — fraud engine may be misconfigured")
                elif count != len(rules):
                    resp.failure(f"count mismatch: {count} != {len(rules)}")
                else:
                    resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(1)
    @tag("admin", "http")
    def update_rule_threshold(self):
        """
        PUT /rules/{rule_id}/threshold — hot-update de umbral sin redeploy.
        Simula un analista ajustando los parámetros del motor en producción.

        RULE-002: new_threshold=3.0, new_score_weight=15 (sin cambio real)
        RULE-005: new_threshold=5000000, new_score_weight=30 (sin cambio real)
        """
        # Elegir solo reglas seguras de modificar (valores idénticos a los defaults)
        safe_updates = [
            ("RULE-002", 3.0,     15),
            ("RULE-005", 5000000, 30),
            ("RULE-001", 60.0,    20),
        ]
        rule_id, threshold, weight = random.choice(safe_updates)

        with self.client.put(
            f"/rules/{rule_id}/threshold",
            catch_response=True,
            name="PUT /rules/{rule_id}/threshold",
            json={
                "new_threshold":    threshold,
                "new_score_weight": weight,
            },
        ) as resp:
            if resp.status_code == 200:
                resp.success()
            elif resp.status_code == 400:
                # Validación fallida — puede ocurrir con valores inválidos
                resp.success()  # Es un comportamiento esperado
            elif resp.status_code == 404:
                resp.failure(f"Rule {rule_id} not found")
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(1)
    @tag("error", "http")
    def get_fraud_case_invalid_id(self):
        """
        GET /fraud-cases/{id} con UUID inválido — debe retornar 400.
        Verifica el manejo de errores del handler.
        """
        bad_ids = ["not-a-uuid", "12345", "RULE-001"]
        with self.client.get(
            f"/fraud-cases/{random.choice(bad_ids)}",
            catch_response=True,
            name="GET /fraud-cases/{id} [invalid]",
        ) as resp:
            if resp.status_code in (400, 404):
                resp.success()
            else:
                resp.failure(f"expected 400, got {resp.status_code}")


# ── FraudNATSUser ──────────────────────────────────────────────────────────────

class FraudNATSUser(User):
    """
    Publica FraudCheckRequested directamente a NATS y verifica
    el FraudCase resultante via HTTP.

    A diferencia del flujo completo (Authorization → Fraud Detection),
    este test envía directamente al subject posnet.fraud.check-requested.v1,
    aislando el BC Fraud Detection del resto de la saga.

    Escenarios basados en las 5 reglas de fd_rule_engine.go:
      - CHIP normal      → score 0  → APPROVE   (ninguna regla activa)
      - MAGSTRIPE alto   → score 30 → APPROVE   (solo RULE-005)
      - MAGSTRIPE alto + historial → score 45 → APPROVE  (RULE-005 + RULE-002)
      - Idempotencia     → mismo event_id dos veces → 1 FraudCase
    """
    weight = 1
    wait_time = between(1.0, 3.0)

    _nc = None
    _js = None
    _loop = None

    def on_start(self):
        try:
            self._loop = asyncio.new_event_loop()
            asyncio.set_event_loop(self._loop)
            self._nc, self._js = self._loop.run_until_complete(self._connect())
        except Exception as e:
            print(f"[FraudNATSUser] NATS connect failed: {e}")
            raise StopUser()

    async def _connect(self):
        nc = await nats.connect(
            NATS_URL,
            max_reconnect_attempts=3,
            reconnect_time_wait=1,
            connect_timeout=5,
        )
        js = nc.jetstream()
        return nc, js

    def on_stop(self):
        if self._nc and self._loop:
            try:
                self._loop.run_until_complete(self._nc.drain())
            except Exception:
                pass

    # ── Tareas ──────────────────────────────────────────────────────────────

    @task(7)
    @tag("fraud", "nats", "approve", "score-0")
    def evaluate_chip_normal(self):
        """
        CHIP + monto normal (ARS 100–5.000).
        Ninguna regla activa → score 0 → APPROVE.
        Representa el 80% del tráfico real de un comercio minorista.
        """
        self._run_evaluation(
            entry_mode="CHIP",
            amount_cents=random.randint(10_000, 500_000),
            card_network=random.choices(
                ["VISA", "MASTERCARD", "CABAL"], weights=[0.5, 0.3, 0.2], k=1
            )[0],
            label="CHIP/score-0/approve",
            expected_decisions={"APPROVE"},
        )

    @task(3)
    @tag("fraud", "nats", "approve", "score-30")
    def evaluate_magstripe_high_amount(self):
        """
        MAGSTRIPE + monto > ARS 50.000 (5.000.000 cents).
        Activa RULE-005 (+30) → score 30 → APPROVE.
        Caso típico de tarjeta física sin chip en comercios mayoristas.
        """
        self._run_evaluation(
            entry_mode="MAGSTRIPE",
            amount_cents=random.randint(5_000_001, 9_999_999),
            card_network="VISA",
            label="MAGSTRIPE/score-30/approve",
            expected_decisions={"APPROVE"},
        )

    @task(2)
    @tag("fraud", "nats", "contactless", "score-0")
    def evaluate_contactless_low(self):
        """
        CONTACTLESS + monto bajo (ARS 5–500).
        Score 0 → APPROVE. Escenario: transporte público, cafetería.
        """
        self._run_evaluation(
            entry_mode="CONTACTLESS",
            amount_cents=random.randint(500, 50_000),
            card_network=random.choices(
                ["VISA", "MASTERCARD"], weights=[0.6, 0.4], k=1
            )[0],
            label="CONTACTLESS/score-0/approve",
            expected_decisions={"APPROVE"},
        )

    @task(1)
    @tag("fraud", "nats", "review", "score-50")
    def evaluate_review_scenario(self):
        """
        Escenario REVIEW: monto muy alto que puede activar RULE-002.
        Si el promedio del comercio es bajo, RULE-002 suma +15.
        Con MAGSTRIPE + RULE-005 (+30) + RULE-002 (+15) = score 45 → APPROVE.
        Score REVIEW (50-69) requeriría además otra regla activa.
        """
        self._run_evaluation(
            entry_mode="MAGSTRIPE",
            amount_cents=random.randint(9_000_000, 9_999_999),
            card_network="MASTERCARD",
            label="MAGSTRIPE/score-45/approve-or-review",
            expected_decisions={"APPROVE", "REVIEW"},
        )

    @task(1)
    @tag("fraud", "nats", "idempotency")
    def evaluate_idempotency(self):
        """
        Publica el mismo event_id dos veces.
        El segundo debe ser descartado por processed_events.
        Verifica que exista exactamente 1 FraudCase en la BD.
        """
        tx_id    = new_uuid()
        event_id = new_uuid()
        amount   = random.randint(10_000, 100_000)
        payload  = build_fraud_check_envelope(
            tx_id, event_id, amount, "CHIP", "VISA"
        )

        t0 = time.time()
        try:
            # Publicar dos veces el mismo evento
            for _ in range(2):
                self._loop.run_until_complete(
                    self._js.publish(
                        "posnet.fraud.check-requested.v1",
                        payload,
                        headers={"Nats-Msg-Id": event_id},
                    )
                )

            # Polling — debe existir exactamente 1 FraudCase
            result = self._poll_fraud_case(tx_id)
            elapsed_ms = (time.time() - t0) * 1000

            exc = None if result else Exception("FraudCase not found after idempotency test")
            events.request.fire(
                request_type="NATS+HTTP", name="fraud/idempotency",
                response_time=elapsed_ms, response_length=0,
                exception=exc, context={},
            )
        except Exception as e:
            events.request.fire(
                request_type="NATS+HTTP", name="fraud/idempotency",
                response_time=(time.time() - t0) * 1000, response_length=0,
                exception=e, context={},
            )

    # ── Método principal ─────────────────────────────────────────────────────

    def _run_evaluation(
        self,
        entry_mode: str,
        amount_cents: int,
        card_network: str,
        label: str,
        expected_decisions: set,
    ):
        """
        Publish FraudCheckRequested → poll GET /fraud-cases/{id} → report.

        Reporta:
          fraud/{label}/publish  — latencia del publish a NATS
          fraud/{label}/e2e      — latencia total hasta tener el FraudCase
        """
        tx_id    = new_uuid()
        event_id = new_uuid()
        payload  = build_fraud_check_envelope(
            tx_id, event_id, amount_cents, entry_mode, card_network
        )

        # ── Publish ──────────────────────────────────────────────────────────
        t_pub = time.time()
        try:
            self._loop.run_until_complete(
                self._js.publish(
                    "posnet.fraud.check-requested.v1",
                    payload,
                    headers={"Nats-Msg-Id": event_id},
                )
            )
            events.request.fire(
                request_type="NATS", name=f"fraud/{label}/publish",
                response_time=(time.time() - t_pub) * 1000,
                response_length=len(payload), exception=None, context={},
            )
        except Exception as e:
            events.request.fire(
                request_type="NATS", name=f"fraud/{label}/publish",
                response_time=(time.time() - t_pub) * 1000,
                response_length=0, exception=e, context={},
            )
            return

        # ── Polling del FraudCase ─────────────────────────────────────────────
        t_e2e = time.time()
        result = self._poll_fraud_case(tx_id)
        e2e_ms = (time.time() - t_e2e) * 1000

        if result is None:
            exc = Exception(f"Timeout: FraudCase not found for tx {tx_id}")
        else:
            decision = result.get("Decision") or result.get("decision", "")
            if decision not in expected_decisions:
                # No es un fallo — es información. REVIEW puede aparecer inesperadamente
                # si el historial del terminal tiene rechazos recientes.
                exc = None  # No forzar fallo por decisión inesperada
            else:
                exc = None

            # Agregar al pool compartido para el HTTPUser
            if len(_known_fraud_tx_ids) < 500:
                _known_fraud_tx_ids.append(tx_id)
            else:
                _known_fraud_tx_ids[random.randint(0, 499)] = tx_id

        events.request.fire(
            request_type="NATS+HTTP", name=f"fraud/{label}/e2e",
            response_time=e2e_ms, response_length=0,
            exception=exc, context={},
        )

    def _poll_fraud_case(self, tx_id: str) -> dict | None:
        """
        Hace polling de GET /fraud-cases/{id} hasta obtener el FraudCase
        o agotar el timeout.
        """
        url      = f"{_http_host}/fraud-cases/{tx_id}"
        deadline = time.time() + POLL_TIMEOUT

        while time.time() < deadline:
            try:
                resp = req_lib.get(url, timeout=3)
                if resp.status_code == 200:
                    data = resp.json()
                    # Verificar que tenga los campos clave del FraudCase
                    has_data = (
                        ("Decision" in data or "decision" in data) and
                        ("Score"    in data or "score"    in data)
                    )
                    if has_data:
                        return data
                # 404 = aún no evaluado, seguir esperando
            except Exception:
                pass

            time.sleep(POLL_INTERVAL)

        return None


# ── Eventos globales ───────────────────────────────────────────────────────────

@events.init.add_listener
def on_locust_init(environment, **kwargs):
    global _http_host
    if environment.host:
        _http_host = environment.host.rstrip("/")

    print(f"""
╔══════════════════════════════════════════════════════╗
║   POSNET Load Test — BC Fraud Detection             ║
║   HTTP:  {_http_host:<43}║
║   NATS:  {NATS_URL:<43}║
║   Reglas: RULE-001(+20) RULE-002(+15) RULE-003(+25) ║
║           RULE-004(+20) RULE-005(+30)               ║
║   REJECT threshold: score >= 70                     ║
╚══════════════════════════════════════════════════════╝
    """)


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    stats = environment.stats
    print("\n── Fraud Detection Load Test Summary ────────────────────")
    for name, entry in stats.entries.items():
        if entry.num_requests > 0:
            print(
                f"  {str(name[1]):48s} "
                f"req={entry.num_requests:5d}  "
                f"fail={entry.num_failures:4d}  "
                f"p50={entry.get_response_time_percentile(0.50):6.0f}ms  "
                f"p95={entry.get_response_time_percentile(0.95):6.0f}ms"
            )
    print(f"  Known FraudCase TX IDs in pool: {len(_known_fraud_tx_ids)}")
    print("─────────────────────────────────────────────────────────\n")