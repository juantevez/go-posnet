// Package crypto contiene utilidades criptográficas del Shared Kernel.
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// ─── HMAC ─────────────────────────────────────────────────────────────────────

// SignMessage genera una firma HMAC-SHA256 del payload con la clave dada.
// Retorna la firma como string hexadecimal.
// Uso: firma va en el header X-Signature de los mensajes NATS.
func SignMessage(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyMessage verifica que la firma HMAC-SHA256 sea válida para el payload.
// Usa comparación de tiempo constante para prevenir timing attacks.
func VerifyMessage(payload []byte, signature string, secret []byte) bool {
	expected := SignMessage(payload, secret)
	// Comparación en tiempo constante (hmac.Equal opera sobre bytes).
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(sigBytes, expectedBytes)
}

// ─── TLS ──────────────────────────────────────────────────────────────────────

// LoadTLSConfig construye un tls.Config para clientes y servidores.
// Soporta TLS 1.3 mínimo y autenticación mTLS con certificado cliente.
func LoadTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("crypto: load cert/key pair: %w", err)
	}

	caData, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("crypto: read CA file %q: %w", caPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("crypto: failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// LoadClientTLSConfig construye un tls.Config para clientes mTLS (sin ClientAuth).
func LoadClientTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cfg, err := LoadTLSConfig(certPath, keyPath, caPath)
	if err != nil {
		return nil, err
	}
	cfg.ClientAuth = tls.NoClientCert
	return cfg, nil
}

// ─── NKey ─────────────────────────────────────────────────────────────────────

// LoadNKeyOption carga un archivo .nk de NKey de NATS y retorna
// la opción de autenticación para nats.Connect().
func LoadNKeyOption(nkeyPath string) (nats.Option, error) {
	seed, err := os.ReadFile(nkeyPath)
	if err != nil {
		return nil, fmt.Errorf("crypto: read nkey file %q: %w", nkeyPath, err)
	}

	kp, err := nkeys.FromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("crypto: parse nkey seed: %w", err)
	}

	pub, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("crypto: get nkey public key: %w", err)
	}

	return nats.Nkey(pub, func(nonce []byte) ([]byte, error) {
		sig, err := kp.Sign(nonce)
		if err != nil {
			return nil, fmt.Errorf("crypto: sign nkey nonce: %w", err)
		}
		return sig, nil
	}), nil
}
