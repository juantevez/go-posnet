// Package config centraliza la configuración del BC Authorization.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	GRPCPort int
	HTTPPort int
	Postgres PostgresConfig
	NATS     NATSConfig
	OTEL     OTELConfig
	Acquirer AcquirerConfig
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

type AcquirerConfig struct {
	Host string
	Port int
	// TLS campos opcionales — en dev pueden estar vacíos o ser "none"
	TLSCertPath    string
	TLSKeyPath     string
	TLSCAPath      string
	TimeoutSeconds int
}

// TLSEnabled indica si hay configuración TLS real (no vacío ni "none").
func (a AcquirerConfig) TLSEnabled() bool {
	return a.TLSCertPath != "" && a.TLSCertPath != "none" &&
		a.TLSKeyPath != "" && a.TLSKeyPath != "none"
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort: envInt("GRPC_PORT", 9090),
		HTTPPort: envInt("HTTP_PORT", 8080),

		Postgres: PostgresConfig{
			DSN:             requireEnv("POSTGRES_DSN"),
			MaxConns:        int32(envInt("POSTGRES_MAX_CONNS", 20)),
			MinConns:        int32(envInt("POSTGRES_MIN_CONNS", 2)),
			MaxConnLifetime: envDuration("POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime: envDuration("POSTGRES_MAX_CONN_IDLE", 5*time.Minute),
			MigrationsDir:   envStr("MIGRATIONS_DIR", "migrations/authorization"),
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
			ServiceName:    envStr("OTEL_SERVICE_NAME", "posnet-authorization"),
			ServiceVersion: envStr("OTEL_SERVICE_VERSION", "1.0.0"),
			OTLPEndpoint:   envStr("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
			Environment:    envStr("ENVIRONMENT", "development"),
		},

		Acquirer: AcquirerConfig{
			Host: envStr("ACQUIRER_HOST", "localhost"),
			Port: envInt("ACQUIRER_PORT", 9100),
			// Opcionales — no hacen panic si están vacíos
			TLSCertPath:    envStr("ACQUIRER_TLS_CERT", ""),
			TLSKeyPath:     envStr("ACQUIRER_TLS_KEY", ""),
			TLSCAPath:      envStr("ACQUIRER_TLS_CA", ""),
			TimeoutSeconds: envInt("ACQUIRER_TIMEOUT_SECONDS", 30),
		},
	}

	return cfg, nil
}

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
