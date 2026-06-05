-- Schema del BC Notification
-- Ejecutado por golang-migrate al arrancar el servicio.

CREATE SCHEMA IF NOT EXISTS notification;

-- ─── notifications ────────────────────────────────────────────────────────────

CREATE TABLE notification.notifications (
    id              UUID        PRIMARY KEY,
    transaction_id  UUID        NOT NULL,
    merchant_id     UUID        NOT NULL,
    channel         VARCHAR(30) NOT NULL, -- TERMINAL_WEBSOCKET | WEBHOOK | EMAIL | SMS
    state           VARCHAR(20) NOT NULL DEFAULT 'PENDING',

    -- Payload del comprobante (serializado como JSONB)
    receipt         JSONB       NOT NULL DEFAULT '{}',

    -- Control de reintentos
    attempt_count   INT         NOT NULL DEFAULT 0,
    max_attempts    INT         NOT NULL DEFAULT 5,
    next_retry_at   TIMESTAMPTZ,

    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at   TIMESTAMPTZ
);

CREATE INDEX idx_nt_notifications_transaction
    ON notification.notifications(transaction_id);

CREATE INDEX idx_nt_notifications_pending_retry
    ON notification.notifications(next_retry_at)
    WHERE state = 'RETRYING' AND next_retry_at IS NOT NULL;

CREATE INDEX idx_nt_notifications_dead
    ON notification.notifications(created_at DESC)
    WHERE state = 'DEAD';

-- ─── delivery_attempts ────────────────────────────────────────────────────────

CREATE TABLE notification.delivery_attempts (
    id               VARCHAR(100) PRIMARY KEY, -- "{notification_id}-{attempt_number}"
    notification_id  UUID         NOT NULL REFERENCES notification.notifications(id),
    attempt_number   INT          NOT NULL,
    success          BOOLEAN      NOT NULL,
    http_status      INT          NOT NULL DEFAULT 0,
    error_message    TEXT,
    attempted_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nt_attempts_notification
    ON notification.delivery_attempts(notification_id, attempt_number ASC);

-- ─── processed_events (idempotencia) ─────────────────────────────────────────

CREATE TABLE notification.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
