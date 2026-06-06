
CREATE SCHEMA IF NOT EXISTS terminal_gateway;


CREATE TABLE terminal_gateway.terminals (
    id              UUID        PRIMARY KEY,
    merchant_id     UUID        NOT NULL,
    terminal_code   VARCHAR(20) NOT NULL UNIQUE, 
    certificate_cn  TEXT        NOT NULL UNIQUE, 
    status          VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tg_terminals_merchant ON terminal_gateway.terminals(merchant_id);


CREATE TABLE terminal_gateway.payment_sessions (
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

CREATE INDEX idx_tg_sessions_terminal_active
    ON terminal_gateway.payment_sessions(terminal_id, state)
    WHERE state IN ('AWAITING_PAYMENT', 'PROCESSING');

CREATE INDEX idx_tg_sessions_expires_at
    ON terminal_gateway.payment_sessions(expires_at)
    WHERE state IN ('AWAITING_PAYMENT', 'PROCESSING');

CREATE TABLE terminal_gateway.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
