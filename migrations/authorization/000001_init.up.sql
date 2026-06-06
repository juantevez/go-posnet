
CREATE SCHEMA IF NOT EXISTS pn_authorization;

CREATE TABLE pn_authorization.transactions (
    id              UUID        PRIMARY KEY,
    terminal_id     UUID        NOT NULL,
    merchant_id     UUID        NOT NULL,

    state           VARCHAR(20) NOT NULL DEFAULT 'RECEIVED',

    amount_cents    BIGINT      NOT NULL,
    currency        CHAR(3)     NOT NULL,

    pan_last4       CHAR(4)     NOT NULL,
    card_network    VARCHAR(20) NOT NULL,
    entry_mode      VARCHAR(20) NOT NULL,
    stan            INTEGER     NOT NULL,

    auth_code           VARCHAR(6),
    rejection_code      VARCHAR(20),
    rejection_source    VARCHAR(20),

    fraud_score         SMALLINT,
    fraud_decision      VARCHAR(10),
    fraud_rules_hit     JSONB,

    emv_data_b64        TEXT,
    iso8583_raw         BYTEA,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    authorized_at   TIMESTAMPTZ,
    rejected_at     TIMESTAMPTZ,

    settlement_batch_id UUID
);

CREATE INDEX idx_auth_txn_terminal_date
    ON pn_authorization.transactions(terminal_id, created_at DESC);

CREATE INDEX idx_auth_txn_state
    ON pn_authorization.transactions(state)
    WHERE state IN ('RECEIVED', 'FRAUD_CHECKING', 'PROCESSING', 'INDETERMINATE');

CREATE INDEX idx_auth_txn_stan
    ON pn_authorization.transactions(terminal_id, stan, created_at::date);

CREATE TABLE pn_authorization.processed_events (
    event_id        UUID        PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

