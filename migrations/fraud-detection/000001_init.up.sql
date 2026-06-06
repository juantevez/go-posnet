
CREATE SCHEMA IF NOT EXISTS fraud_detection;

CREATE TABLE fraud_detection.fraud_cases (
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

CREATE INDEX idx_fd_cases_terminal
    ON fraud_detection.fraud_cases(terminal_id, evaluated_at DESC);

CREATE INDEX idx_fd_cases_decision
    ON fraud_detection.fraud_cases(decision)
    WHERE decision IN ('REJECT', 'REVIEW');


CREATE TABLE fraud_detection.fraud_rules (
    id              VARCHAR(20) PRIMARY KEY,    
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    score_weight    SMALLINT    NOT NULL,       
    threshold_value NUMERIC,                    
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO fraud_detection.fraud_rules (id, name, description, score_weight, threshold_value) VALUES
    ('RULE-001', 'Velocity Check',         'Más de 60 transacciones por hora en el terminal',              20,  60),
    ('RULE-002', 'Unusual Amount',         'Monto mayor a 3x el promedio del comercio (últimos 30 días)', 15,   3),
    ('RULE-003', 'Multiple Rejections',    'Más de 3 rechazos en los últimos 10 minutos',                 25,   3),
    ('RULE-004', 'Repeated Amount',        'Mismo monto exacto más de una vez en 5 minutos',              20,   1),
    ('RULE-005', 'High Amount Magstripe',  'Tarjeta sin chip con monto alto (> ARS 50.000)',               30,   5000000)
ON CONFLICT (id) DO NOTHING;


CREATE TABLE fraud_detection.processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
