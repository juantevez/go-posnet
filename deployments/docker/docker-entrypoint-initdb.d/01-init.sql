
CREATE SCHEMA IF NOT EXISTS pn_authorization;
CREATE SCHEMA IF NOT EXISTS terminal_gateway;
CREATE SCHEMA IF NOT EXISTS fraud_detection;
CREATE SCHEMA IF NOT EXISTS settlement;
CREATE SCHEMA IF NOT EXISTS notification;

-- pn_authorization
CREATE TABLE IF NOT EXISTS pn_authorization.transactions (
    id                  UUID        PRIMARY KEY,
    terminal_id         UUID        NOT NULL,
    merchant_id         UUID        NOT NULL,
    state               VARCHAR(20) NOT NULL DEFAULT 'RECEIVED',
    amount_cents        BIGINT      NOT NULL,
    currency            CHAR(3)     NOT NULL,
    pan_last4           CHAR(4)     NOT NULL,
    card_network        VARCHAR(20) NOT NULL,
    entry_mode          VARCHAR(20) NOT NULL,
    stan                INTEGER     NOT NULL,
    auth_code           VARCHAR(6),
    rejection_code      VARCHAR(20),
    rejection_source    VARCHAR(20),
    fraud_score         SMALLINT,
    fraud_decision      VARCHAR(10),
    fraud_rules_hit     JSONB,
    emv_data_b64        TEXT,
    iso8583_raw         BYTEA,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    authorized_at       TIMESTAMPTZ,
    rejected_at         TIMESTAMPTZ,
    settlement_batch_id UUID
);
CREATE TABLE IF NOT EXISTS pn_authorization.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- terminal_gateway
CREATE TABLE IF NOT EXISTS terminal_gateway.terminals (
    id              UUID        PRIMARY KEY,
    merchant_id     UUID        NOT NULL,
    terminal_code   VARCHAR(20) NOT NULL UNIQUE,
    certificate_cn  TEXT        NOT NULL UNIQUE,
    status          VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS terminal_gateway.payment_sessions (
    id               UUID        PRIMARY KEY,
    terminal_id      UUID        NOT NULL REFERENCES terminal_gateway.terminals(id),
    merchant_id      UUID        NOT NULL,
    state            VARCHAR(20) NOT NULL DEFAULT 'AWAITING_PAYMENT',
    channel          VARCHAR(20) NOT NULL,
    amount_cents     BIGINT      NOT NULL,
    currency         CHAR(3)     NOT NULL,
    stan             INTEGER     NOT NULL,
    auth_code        VARCHAR(6),
    rejection_code   VARCHAR(20),
    rejection_reason TEXT,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at        TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS terminal_gateway.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- fraud_detection
CREATE TABLE IF NOT EXISTS fraud_detection.fraud_cases (
    id              UUID        PRIMARY KEY,
    transaction_id  UUID        NOT NULL UNIQUE,
    terminal_id     UUID        NOT NULL,
    merchant_id     UUID        NOT NULL,
    amount_cents    BIGINT      NOT NULL,
    currency        CHAR(3)     NOT NULL,
    card_network    VARCHAR(20) NOT NULL,
    entry_mode      VARCHAR(20) NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL,
    score           SMALLINT    NOT NULL,
    decision        VARCHAR(10) NOT NULL,
    rules_hit       JSONB       NOT NULL DEFAULT '[]',
    evaluations     JSONB       NOT NULL DEFAULT '[]',
    evaluated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS fraud_detection.fraud_rules (
    id              VARCHAR(20)  PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    score_weight    SMALLINT     NOT NULL,
    threshold_value NUMERIC,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
INSERT INTO fraud_detection.fraud_rules (id, name, description, score_weight, threshold_value) VALUES
    ('RULE-001', 'Velocity Check',        'Mas de 60 tx por hora en terminal',        20, 60),
    ('RULE-002', 'Unusual Amount',        'Monto mayor a 3x promedio del comercio',   15,  3),
    ('RULE-003', 'Multiple Rejections',   'Mas de 3 rechazos en 10 minutos',          25,  3),
    ('RULE-004', 'Repeated Amount',       'Mismo monto mas de una vez en 5 minutos',  20,  1),
    ('RULE-005', 'High Amount Magstripe', 'Magstripe con monto alto mayor ARS 50000', 30,  5000000)
ON CONFLICT (id) DO NOTHING;
CREATE TABLE IF NOT EXISTS fraud_detection.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- settlement
CREATE TABLE IF NOT EXISTS settlement.settlement_batches (
    id              UUID        PRIMARY KEY,
    terminal_id     UUID        NOT NULL,
    merchant_id     UUID        NOT NULL,
    batch_date      DATE        NOT NULL,
    state           VARCHAR(20) NOT NULL DEFAULT 'OPEN',
    currency        CHAR(3)     NOT NULL,
    total_count     INTEGER,
    total_amount    BIGINT,
    purchase_count  INTEGER,
    purchase_amount BIGINT,
    reversal_count  INTEGER,
    reversal_amount BIGINT,
    discrepancies   INTEGER     NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ,
    submitted_at    TIMESTAMPTZ,
    settled_at      TIMESTAMPTZ,
    UNIQUE (terminal_id, batch_date)
);
CREATE TABLE IF NOT EXISTS settlement.batch_transactions (
    id              UUID        PRIMARY KEY,
    batch_id        UUID        NOT NULL REFERENCES settlement.settlement_batches(id),
    transaction_id  UUID        NOT NULL UNIQUE,
    amount_cents    BIGINT      NOT NULL,
    currency        CHAR(3)     NOT NULL,
    tx_type         VARCHAR(20) NOT NULL,
    included_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS settlement.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- notification
CREATE TABLE IF NOT EXISTS notification.notifications (
    id              UUID        PRIMARY KEY,
    transaction_id  UUID        NOT NULL,
    merchant_id     UUID        NOT NULL,
    channel         VARCHAR(30) NOT NULL,
    state           VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    receipt         JSONB       NOT NULL DEFAULT '{}',
    attempt_count   INT         NOT NULL DEFAULT 0,
    max_attempts    INT         NOT NULL DEFAULT 5,
    next_retry_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at   TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS notification.delivery_attempts (
    id               VARCHAR(100) PRIMARY KEY,
    notification_id  UUID         NOT NULL REFERENCES notification.notifications(id),
    attempt_number   INT          NOT NULL,
    success          BOOLEAN      NOT NULL,
    http_status      INT          NOT NULL DEFAULT 0,
    error_message    TEXT,
    attempted_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS notification.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
