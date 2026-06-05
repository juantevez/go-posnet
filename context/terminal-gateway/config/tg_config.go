// Package config centraliza la configuración del BC Terminal Gateway.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contiene toda la configuración del BC Terminal Gateway.
type Config struct {
	GRPCPort int
	HTTPPort int
	WSPort   int // Puerto dedicado para conexiones WebSocket de terminales

	Postgres PostgresConfig
	NATS     NATSConfig
	OTEL     OTELConfig
	TLS      TLSConfig
	Session  SessionConfig
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

// TLSConfig contiene los certificados para mTLS con los terminales POSNET.
// Cada terminal presenta su propio certificado X.509 firmado por la CA interna.
type TLSConfig struct {
	CertPath string // Certificado del servidor Gateway
	KeyPath  string // Clave privada del servidor Gateway
	CAPath   string // CA que firmó los certificados de los terminales
}

// SessionConfig contiene los parámetros de las sesiones de pago.
type SessionConfig struct {
	TTLSeconds          int           // Tiempo de vida de una sesión QR (default: 300 = 5 min)
	ExpiredCleanupEvery time.Duration // Frecuencia del job de limpieza de sesiones expiradas
}

// Load carga la configuración desde variables de entorno.
// Hace panic inmediato si una variable requerida no está configurada.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort: envInt("GRPC_PORT", 9091),
		HTTPPort: envInt("HTTP_PORT", 8081),
		WSPort:   envInt("WS_PORT", 8082),

		Postgres: PostgresConfig{
			DSN:             requireEnv("POSTGRES_DSN"),
			MaxConns:        int32(envInt("POSTGRES_MAX_CONNS", 20)),
			MinConns:        int32(envInt("POSTGRES_MIN_CONNS", 2)),
			MaxConnLifetime: envDuration("POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime: envDuration("POSTGRES_MAX_CONN_IDLE", 5*time.Minute),
			MigrationsDir:   envStr("MIGRATIONS_DIR", "migrations/terminal-gateway"),
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
			ServiceName:    envStr("OTEL_SERVICE_NAME", "posnet-terminal-gateway"),
			ServiceVersion: envStr("OTEL_SERVICE_VERSION", "1.0.0"),
			OTLPEndpoint:   envStr("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
			Environment:    envStr("ENVIRONMENT", "development"),
		},

		TLS: TLSConfig{
			CertPath: requireEnv("TLS_CERT_PATH"),
			KeyPath:  requireEnv("TLS_KEY_PATH"),
			CAPath:   requireEnv("TLS_CA_PATH"),
		},

		Session: SessionConfig{
			TTLSeconds:          envInt("SESSION_TTL_SECONDS", 300),
			ExpiredCleanupEvery: envDuration("SESSION_CLEANUP_EVERY", 1*time.Minute),
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
