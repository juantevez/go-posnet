package crypto

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// LoadTLSConfig construye un tls.Config para servidores con mTLS habilitado.
// Requiere que el cliente presente un certificado firmado por la CA dada.
// Mínimo TLS 1.3.
func LoadTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("crypto/tls: load cert/key pair: %w", err)
	}

	pool, err := loadCertPool(caPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		RootCAs:      pool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// LoadClientTLSConfig construye un tls.Config para clientes mTLS.
// No exige certificado del servidor más allá de la CA (ClientAuth = NoClientCert).
func LoadClientTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("crypto/tls: load client cert/key pair: %w", err)
	}

	pool, err := loadCertPool(caPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// loadCertPool carga una CA desde archivo y retorna un CertPool.
func loadCertPool(caPath string) (*x509.CertPool, error) {
	caData, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("crypto/tls: read CA file %q: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("crypto/tls: failed to parse CA certificate from %q", caPath)
	}
	return pool, nil
}
