// Package config centraliza la configuración del BC Fraud Detection.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contiene toda la configuración del BC Fraud Detection.
type Config struct {
	GRPCPort int
	HTTPPort int

	Postgres PostgresConfig
	NATS     NATSConfig
	OTEL     OTELConfig
	Engine   EngineConfig
}

type PostgresConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	MigrationsDir   string
}

type NATSConfig struct {
	URL           string
	NKeyPath      string
	TLSCertPath   string
	TLSKeyPath    string
	TLSCAPath     string
	MaxReconnect  int
	ReconnectWait time.Duration
}

type OTELConfig struct {
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string
	Environment    string
}

// EngineConfig contiene los parámetros operativos del motor de reglas.
type EngineConfig struct {
	// EvalTimeout es el tiempo máximo para evaluar todas las reglas de una transacción.
	// Si se supera, las reglas que no respondieron se omiten con score 0.
	// Valor recomendado: 200ms — debe ser menor al timeout de Authorization (500ms).
	EvalTimeout time.Duration

	// RulesCacheTTL es el tiempo que las reglas activas se mantienen en memoria
	// antes de recargarse desde Postgres. Permite cambiar umbrales sin redespliegue.
	RulesCacheTTL time.Duration

	// ScoreThresholdReject es el score mínimo para rechazar automáticamente.
	// Default: 70. Configurable sin redespliegue via variable de entorno.
	ScoreThresholdReject int

	// ScoreThresholdReview es el score mínimo para marcar como REVIEW.
	// Default: 50. Transacciones REVIEW van al adquirente pero quedan marcadas.
	ScoreThresholdReview int
}

// Load carga la configuración desde variables de entorno.
// Hace panic inmediato si una variable requerida no está configurada.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort: envInt("GRPC_PORT", 9092),
		HTTPPort: envInt("HTTP_PORT", 8082),

		Postgres: PostgresConfig{
			DSN:             requireEnv("POSTGRES_DSN"),
			MaxConns:        int32(envInt("POSTGRES_MAX_CONNS", 15)),
			MinConns:        int32(envInt("POSTGRES_MIN_CONNS", 2)),
			MaxConnLifetime: envDuration("POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime: envDuration("POSTGRES_MAX_CONN_IDLE", 5*time.Minute),
			MigrationsDir:   envStr("MIGRATIONS_DIR", "migrations/fraud-detection"),
		},

		NATS: NATSConfig{
			URL:           requireEnv("NATS_URL"),
			NKeyPath:      envStr("NATS_NKEY_PATH", ""),
			TLSCertPath:   envStr("NATS_TLS_CERT", ""),
			TLSKeyPath:    envStr("NATS_TLS_KEY", ""),
			TLSCAPath:     envStr("NATS_TLS_CA", ""),
			MaxReconnect:  -1,
			ReconnectWait: 2 * time.Second,
		},

		OTEL: OTELConfig{
			ServiceName:    envStr("OTEL_SERVICE_NAME", "posnet-fraud-detection"),
			ServiceVersion: envStr("OTEL_SERVICE_VERSION", "1.0.0"),
			OTLPEndpoint:   envStr("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
			Environment:    envStr("ENVIRONMENT", "development"),
		},

		Engine: EngineConfig{
			EvalTimeout:          envDuration("ENGINE_EVAL_TIMEOUT", 200*time.Millisecond),
			RulesCacheTTL:        envDuration("ENGINE_RULES_CACHE_TTL", 5*time.Minute),
			ScoreThresholdReject: envInt("ENGINE_SCORE_THRESHOLD_REJECT", 70),
			ScoreThresholdReview: envInt("ENGINE_SCORE_THRESHOLD_REVIEW", 50),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate verifica coherencia entre valores configurados.
func (c *Config) validate() error {
	if c.Engine.ScoreThresholdReview >= c.Engine.ScoreThresholdReject {
		return fmt.Errorf("config: ENGINE_SCORE_THRESHOLD_REVIEW (%d) must be less than ENGINE_SCORE_THRESHOLD_REJECT (%d)",
			c.Engine.ScoreThresholdReview, c.Engine.ScoreThresholdReject)
	}
	if c.Engine.EvalTimeout > 400*time.Millisecond {
		return fmt.Errorf("config: ENGINE_EVAL_TIMEOUT (%s) too high — must be < 400ms to fit within Authorization's 500ms fraud timeout",
			c.Engine.EvalTimeout)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %q is not set", key))
	}
	return v
}

func envStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
