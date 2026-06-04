// Package natsutil centraliza la conexión y setup de NATS JetStream.
// Elimina el boilerplate de conexión en cada Bounded Context.
package natsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// Config contiene los parámetros de conexión a NATS.
type Config struct {
	URL           string        // ej: "nats://nats:4222"
	NKeyPath      string        // Path al archivo .nk de autenticación (NKey seed)
	TLSCertPath   string        // Certificado TLS cliente
	TLSKeyPath    string        // Clave privada TLS cliente
	TLSCAPath     string        // CA de la infraestructura
	MaxReconnect  int           // -1 = infinito
	ReconnectWait time.Duration // Espera entre intentos de reconexión
}

// Connect establece la conexión con autenticación NKey y TLS.
// Configura reconexión automática con backoff progresivo.
func Connect(cfg Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.MaxReconnects(cfg.MaxReconnect),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				fmt.Printf("natsutil: disconnected: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("natsutil: reconnected to %s\n", nc.ConnectedUrl())
		}),
	}

	// Autenticación NKey
	if cfg.NKeyPath != "" {
		nkeyOpt, err := nkeyOption(cfg.NKeyPath)
		if err != nil {
			return nil, fmt.Errorf("natsutil: load nkey: %w", err)
		}
		opts = append(opts, nkeyOpt)
	}

	// TLS
	if cfg.TLSCertPath != "" {
		tlsCfg, err := buildTLSConfig(cfg.TLSCertPath, cfg.TLSKeyPath, cfg.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("natsutil: build tls config: %w", err)
		}
		opts = append(opts, nats.Secure(tlsCfg))
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("natsutil: connect to %s: %w", cfg.URL, err)
	}
	return conn, nil
}

// JetStream retorna el contexto JetStream a partir de una conexión existente.
func JetStream(conn *nats.Conn) (nats.JetStreamContext, error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("natsutil: init jetstream: %w", err)
	}
	return js, nil
}

// nkeyOption carga un NKey seed desde archivo y retorna la opción de auth.
func nkeyOption(path string) (nats.Option, error) {
	seed, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nkey seed file %q: %w", path, err)
	}
	kp, err := nkeys.FromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("parse nkey seed: %w", err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return nil, err
	}
	return nats.Nkey(pub, func(nonce []byte) ([]byte, error) {
		return kp.Sign(nonce)
	}), nil
}

// buildTLSConfig construye un tls.Config con certificado cliente y CA.
func buildTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert/key pair: %w", err)
	}
	caData, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to append CA cert")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
