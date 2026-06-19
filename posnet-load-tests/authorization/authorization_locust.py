"""
Load Test — BC Authorization (go-posnet)
=========================================
Simula dos tipos de usuarios concurrentes:

  AuthHTTPUser   — consulta estado de transacciones via HTTP
  AuthNATSUser   — publica TransactionReceived a NATS + polling del resultado

Uso:
  # UI web:
  locust -f authorization_locust.py --host http://localhost:8080

  # Headless:
  locust -f authorization_locust.py --headless -u 10 -r 2 --run-time 2m \
    --host http://localhost:8080

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

ENTRY_MODES    = ["CHIP", "CONTACTLESS", "MAGSTRIPE", "MANUAL"]
ENTRY_WEIGHTS  = [0.60,   0.25,          0.10,        0.05]

CARD_NETWORKS  = ["VISA", "MASTERCARD", "CABAL", "NARANJA", "AMEX"]
NETWORK_WEIGHTS = [0.45,  0.30,          0.15,   0.08,       0.02]

AMOUNT_RANGES = [
    (500,     5_000,    0.40),
    (5_001,   50_000,   0.35),
    (50_001,  500_000,  0.20),
    (500_001, 9_999_999, 0.05),
]

# Pool compartido de IDs conocidos para que HTTPUser pueda consultarlos
_known_tx_ids: list[str] = []
_stan_counter = 0

# Host HTTP — se setea en on_locust_init desde environment.host
_http_host = "http://localhost:8080"


# ── Helpers ────────────────────────────────────────────────────────────────────

def new_uuid() -> str:
    return str(uuid.uuid4())

def random_amount_cents() -> int:
    roll = random.random()
    cumulative = 0.0
    for lo, hi, weight in AMOUNT_RANGES:
        cumulative += weight
        if roll <= cumulative:
            return random.randint(lo, hi)
    return random.randint(500, 50_000)

def random_entry_mode() -> str:
    return random.choices(ENTRY_MODES, weights=ENTRY_WEIGHTS, k=1)[0]

def random_card_network() -> str:
    return random.choices(CARD_NETWORKS, weights=NETWORK_WEIGHTS, k=1)[0]

def next_stan() -> int:
    global _stan_counter
    _stan_counter = (_stan_counter % 999998) + 1
    return _stan_counter

def build_envelope(tx_id, event_id, amount_cents, entry_mode, card_network, last4, stan) -> bytes:
    """
    Construye el DomainEvent envelope según pkg/events/envelope.go
    y pkg/events/transaction_received.go.
    """
    payload = {
        "transaction_id": tx_id,
        "terminal_id":    TERMINAL_ID,
        "merchant_id":    MERCHANT_ID,
        "amount_cents":   amount_cents,
        "currency":       "ARS",
        "stan":           stan,
        "entry_mode":     entry_mode,
        "card_last4":     last4,
        "card_network":   card_network,
        "emv_data_b64":   "",
        "iso8583_raw":    None,
        "received_at":    datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    envelope = {
        "event_id":       event_id,
        "event_type":     "posnet.transaction.received.v1",
        "aggregate_id":   tx_id,
        "aggregate_type": "Transaction",
        "correlation_id": tx_id,
        "causation_id":   "",
        "occurred_at":    datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "schema_version": 1,
        "data":           payload,
    }
    return json.dumps(envelope).encode("utf-8")


# ── AuthHTTPUser ───────────────────────────────────────────────────────────────

class AuthHTTPUser(HttpUser):
    """
    Simula operadores y dashboards consultando el estado de transacciones.
    Peso 3: 3 usuarios HTTP por cada 1 NATS.
    """
    weight = 3
    wait_time = between(0.5, 2.0)

    def on_start(self):
        with self.client.get("/healthz", catch_response=True) as resp:
            if resp.status_code != 200:
                resp.failure(f"Authorization not ready: {resp.status_code}")
                raise StopUser()

    @task(6)
    @tag("health", "http")
    def health_check(self):
        with self.client.get("/healthz", catch_response=True, name="GET /healthz") as resp:
            if resp.status_code == 200:
                resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(3)
    @tag("readiness", "http")
    def readiness_check(self):
        with self.client.get("/readyz", catch_response=True, name="GET /readyz") as resp:
            if resp.status_code in (200, 503):
                resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(1)
    @tag("query", "http")
    def get_transaction_status(self):
        if _known_tx_ids:
            tx_id = random.choice(_known_tx_ids)
            expected_404 = False
        else:
            tx_id = new_uuid()
            expected_404 = True

        with self.client.get(
            f"/transactions/{tx_id}",
            catch_response=True,
            name="GET /transactions/{id}",
        ) as resp:
            if resp.status_code == 200:
                data = resp.json()
                if "TransactionID" not in data and "transaction_id" not in data:
                    resp.failure("missing transaction_id")
                else:
                    resp.success()
            elif resp.status_code == 404 and expected_404:
                resp.success()
            else:
                resp.failure(f"status: {resp.status_code}")

    @task(1)
    @tag("error", "http")
    def get_transaction_invalid_id(self):
        bad_id = random.choice(["not-a-uuid", "12345", "abc-def"])
        with self.client.get(
            f"/transactions/{bad_id}",
            catch_response=True,
            name="GET /transactions/{id} [invalid]",
        ) as resp:
            if resp.status_code in (400, 404):
                resp.success()
            else:
                resp.failure(f"expected 400, got {resp.status_code}")


# ── AuthNATSUser ───────────────────────────────────────────────────────────────

class AuthNATSUser(User):
    """
    Simula un terminal POSNET: publica a NATS y hace polling del resultado.
    Peso 1: flujo real completo de autorización.
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
            print(f"[NATSUser] NATS connect failed: {e}")
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

    @task(8)
    @tag("saga", "nats", "happy-path")
    def publish_chip_transaction(self):
        """Flujo feliz: CHIP, monto normal → score 0 → APPROVED."""
        self._run_saga(
            entry_mode="CHIP",
            amount_cents=random.randint(500, 500_000),
            label="CHIP/normal",
        )

    @task(3)
    @tag("saga", "nats", "contactless")
    def publish_contactless_transaction(self):
        """CONTACTLESS monto bajo: transporte, kioscos."""
        self._run_saga(
            entry_mode="CONTACTLESS",
            amount_cents=random.randint(500, 10_000),
            label="CONTACTLESS/low",
        )

    @task(1)
    @tag("saga", "nats", "fraud-trigger")
    def publish_high_amount_magstripe(self):
        """MAGSTRIPE monto alto: activa RULE-005 (+30) + RULE-002 (+15) = score 45."""
        self._run_saga(
            entry_mode="MAGSTRIPE",
            amount_cents=random.randint(500_001, 9_999_999),
            label="MAGSTRIPE/high-amount",
        )

    @task(1)
    @tag("saga", "nats", "idempotency")
    def publish_duplicate_event(self):
        """Mismo event_id dos veces — el segundo debe ser descartado."""
        tx_id    = new_uuid()
        event_id = new_uuid()
        payload  = build_envelope(
            tx_id, event_id,
            random.randint(5_000, 50_000),
            "CHIP", "VISA",
            f"{random.randint(1000,9999)}",
            next_stan(),
        )

        t0 = time.time()
        try:
            # Publicar dos veces el mismo evento
            self._loop.run_until_complete(
                self._js.publish("posnet.transaction.received.v1", payload,
                                 headers={"Nats-Msg-Id": event_id})
            )
            self._loop.run_until_complete(
                self._js.publish("posnet.transaction.received.v1", payload,
                                 headers={"Nats-Msg-Id": event_id})
            )

            final_state = self._poll_until_final(tx_id)
            elapsed_ms  = (time.time() - t0) * 1000
            exc = None if final_state in ("APPROVED", "REJECTED") else \
                  Exception(f"No final state: {final_state}")

            events.request.fire(
                request_type="NATS+HTTP", name="saga/idempotency",
                response_time=elapsed_ms, response_length=0,
                exception=exc, context={},
            )
        except Exception as e:
            events.request.fire(
                request_type="NATS+HTTP", name="saga/idempotency",
                response_time=(time.time() - t0) * 1000, response_length=0,
                exception=e, context={},
            )

    # ── Método principal del flujo Saga ─────────────────────────────────────

    def _run_saga(self, entry_mode: str, amount_cents: int, label: str):
        """
        Publish → poll → report.
        Reporta dos métricas:
          saga/{label}/publish  — latencia del publish a NATS
          saga/{label}/e2e      — latencia total hasta estado final
        """
        tx_id    = new_uuid()
        event_id = new_uuid()
        stan     = next_stan()
        network  = random_card_network()
        last4    = f"{random.randint(1000, 9999)}"

        payload = build_envelope(tx_id, event_id, amount_cents, entry_mode, network, last4, stan)

        # ── Publish ──────────────────────────────────────────────────────────
        t_pub = time.time()
        try:
            self._loop.run_until_complete(
                self._js.publish(
                    "posnet.transaction.received.v1",
                    payload,
                    headers={"Nats-Msg-Id": event_id},
                )
            )
            events.request.fire(
                request_type="NATS", name=f"saga/{label}/publish",
                response_time=(time.time() - t_pub) * 1000,
                response_length=len(payload), exception=None, context={},
            )
        except Exception as e:
            events.request.fire(
                request_type="NATS", name=f"saga/{label}/publish",
                response_time=(time.time() - t_pub) * 1000,
                response_length=0, exception=e, context={},
            )
            return

        # ── Polling hasta estado final ────────────────────────────────────────
        t_e2e = time.time()
        final_state = self._poll_until_final(tx_id)
        e2e_ms = (time.time() - t_e2e) * 1000

        success = final_state in ("APPROVED", "REJECTED")
        exc = None if success else Exception(f"Timeout or unexpected state: {final_state}")

        events.request.fire(
            request_type="NATS+HTTP", name=f"saga/{label}/e2e",
            response_time=e2e_ms, response_length=0,
            exception=exc, context={},
        )

        if final_state == "APPROVED":
            if len(_known_tx_ids) < 500:
                _known_tx_ids.append(tx_id)
            else:
                _known_tx_ids[random.randint(0, 499)] = tx_id

    def _poll_until_final(self, tx_id: str) -> str | None:
        """
        Hace polling de GET /transactions/{id} con requests directo.
        Retorna el estado final o None si se agotó el timeout.
        """
        url      = f"{_http_host}/transactions/{tx_id}"
        deadline = time.time() + POLL_TIMEOUT

        final_states = {"APPROVED", "REJECTED", "REVERSED", "INDETERMINATE"}

        while time.time() < deadline:
            try:
                resp = req_lib.get(url, timeout=3)
                if resp.status_code == 200:
                    data = resp.json()
                    # El handler Go retorna PascalCase (sin json tags en el struct)
                    # Buscar ambas variantes por si se agrega json tags en el futuro
                    state = data.get("State") or data.get("state", "")
                    if state in final_states:
                        return state
                # 404 = aún no procesada, seguir esperando
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
║   POSNET Load Test — BC Authorization               ║
║   HTTP:  {_http_host:<43}║
║   NATS:  {NATS_URL:<43}║
║   Poll:  timeout={POLL_TIMEOUT}s  interval={POLL_INTERVAL}s             ║
╚══════════════════════════════════════════════════════╝
    """)


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    stats = environment.stats
    print("\n── Authorization Load Test Summary ──────────────────────")
    for name, entry in stats.entries.items():
        if entry.num_requests > 0:
            print(
                f"  {str(name[1]):42s} "
                f"req={entry.num_requests:5d}  "
                f"fail={entry.num_failures:4d}  "
                f"p50={entry.get_response_time_percentile(0.50):6.0f}ms  "
                f"p95={entry.get_response_time_percentile(0.95):6.0f}ms  "
                f"p99={entry.get_response_time_percentile(0.99):6.0f}ms"
            )
    print(f"  Known TX IDs in pool: {len(_known_tx_ids)}")
    print("─────────────────────────────────────────────────────────\n")
    