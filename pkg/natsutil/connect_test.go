package natsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

// ─── test fixtures ────────────────────────────────────────────────────────────

// writeTestNKeySeed genera un NKey de usuario válido y escribe su seed a un
// archivo temporal, devolviendo el path.
func writeTestNKeySeed(t *testing.T) string {
	t.Helper()
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatalf("nkeys.CreateUser() error = %v", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatalf("kp.Seed() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "user.nk")
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}

// writeTestCert genera un certificado autofirmado (ECDSA P-256) y escribe el
// certificado y la clave privada a archivos PEM temporales.
func writeTestCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "posnet-test"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey() error = %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("os.WriteFile(key) error = %v", err)
	}
	return certPath, keyPath
}

// ─── nkeyOption ───────────────────────────────────────────────────────────────

func TestNkeyOption_Success(t *testing.T) {
	path := writeTestNKeySeed(t)

	opt, err := nkeyOption(path)
	if err != nil {
		t.Fatalf("nkeyOption() error = %v", err)
	}
	if opt == nil {
		t.Fatal("nkeyOption() returned nil option, want non-nil")
	}
}

func TestNkeyOption_FileNotFound(t *testing.T) {
	_, err := nkeyOption(filepath.Join(t.TempDir(), "missing.nk"))
	if err == nil || !strings.Contains(err.Error(), "read nkey seed file") {
		t.Fatalf("error = %v, want it to contain %q", err, "read nkey seed file")
	}
}

func TestNkeyOption_InvalidSeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.nk")
	if err := os.WriteFile(path, []byte("not-a-valid-nkey-seed"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := nkeyOption(path)
	if err == nil || !strings.Contains(err.Error(), "parse nkey seed") {
		t.Fatalf("error = %v, want it to contain %q", err, "parse nkey seed")
	}
}

// ─── buildTLSConfig ───────────────────────────────────────────────────────────

func TestBuildTLSConfig_Success(t *testing.T) {
	certPath, keyPath := writeTestCert(t)
	// Reutilizamos el mismo certificado autofirmado como CA — a
	// AppendCertsFromPEM le alcanza con un PEM de certificado válido.
	caPath := certPath

	cfg, err := buildTLSConfig(certPath, keyPath, caPath)
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("len(Certificates) = %d, want 1", len(cfg.Certificates))
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs is nil, want a populated cert pool")
	}
	if cfg.MinVersion != 0x0304 { // tls.VersionTLS13
		t.Errorf("MinVersion = %#x, want TLS 1.3 (%#x)", cfg.MinVersion, 0x0304)
	}
}

func TestBuildTLSConfig_CertNotFound(t *testing.T) {
	_, keyPath := writeTestCert(t)
	_, err := buildTLSConfig(filepath.Join(t.TempDir(), "missing.pem"), keyPath, keyPath)
	if err == nil || !strings.Contains(err.Error(), "load cert/key pair") {
		t.Fatalf("error = %v, want it to contain %q", err, "load cert/key pair")
	}
}

func TestBuildTLSConfig_CANotFound(t *testing.T) {
	certPath, keyPath := writeTestCert(t)
	_, err := buildTLSConfig(certPath, keyPath, filepath.Join(t.TempDir(), "missing-ca.pem"))
	if err == nil || !strings.Contains(err.Error(), "read CA file") {
		t.Fatalf("error = %v, want it to contain %q", err, "read CA file")
	}
}

func TestBuildTLSConfig_InvalidCAContent(t *testing.T) {
	certPath, keyPath := writeTestCert(t)
	caPath := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(caPath, []byte("not a valid PEM certificate"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := buildTLSConfig(certPath, keyPath, caPath)
	if err == nil || !strings.Contains(err.Error(), "failed to append CA cert") {
		t.Fatalf("error = %v, want it to contain %q", err, "failed to append CA cert")
	}
}

// ─── Connect ──────────────────────────────────────────────────────────────────

func TestConnect_NKeyLoadError(t *testing.T) {
	_, err := Connect(Config{
		URL:      "nats://127.0.0.1:1",
		NKeyPath: filepath.Join(t.TempDir(), "missing.nk"),
	})
	if err == nil || !strings.Contains(err.Error(), "natsutil: load nkey") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: load nkey")
	}
}

func TestConnect_TLSConfigError(t *testing.T) {
	_, err := Connect(Config{
		URL:         "nats://127.0.0.1:1",
		TLSCertPath: filepath.Join(t.TempDir(), "missing-cert.pem"),
	})
	if err == nil || !strings.Contains(err.Error(), "natsutil: build tls config") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: build tls config")
	}
}

func TestConnect_ValidNKey_StillFailsAtConnect(t *testing.T) {
	// El NKey carga correctamente, pero la conexión real sigue fallando —
	// cubre la rama de éxito de nkeyOption dentro de Connect.
	_, err := Connect(Config{
		URL:      "nats://127.0.0.1:1",
		NKeyPath: writeTestNKeySeed(t),
	})
	if err == nil || !strings.Contains(err.Error(), "natsutil: connect to") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: connect to")
	}
}

func TestConnect_ValidTLS_StillFailsAtConnect(t *testing.T) {
	// El tls.Config se construye correctamente, pero la conexión real sigue
	// fallando — cubre la rama de éxito de buildTLSConfig dentro de Connect.
	certPath, keyPath := writeTestCert(t)
	_, err := Connect(Config{
		URL:         "nats://127.0.0.1:1",
		TLSCertPath: certPath,
		TLSKeyPath:  keyPath,
		TLSCAPath:   certPath,
	})
	if err == nil || !strings.Contains(err.Error(), "natsutil: connect to") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: connect to")
	}
}

func TestConnect_ConnectionRefused(t *testing.T) {
	// Puerto cerrado en loopback — connection refused inmediato, sin
	// depender de un servidor NATS real disponible.
	_, err := Connect(Config{URL: "nats://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "natsutil: connect to nats://127.0.0.1:1") {
		t.Fatalf("error = %v, want it to contain %q", err, "natsutil: connect to nats://127.0.0.1:1")
	}
}
