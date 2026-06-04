package crypto

import (
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// LoadNKeyOption carga un archivo .nk de NKey de NATS y retorna
// la opción de autenticación lista para pasar a nats.Connect().
//
// NKey usa criptografía Ed25519: el servidor NATS envía un nonce,
// el cliente lo firma con su clave privada, y el servidor verifica
// la firma con la clave pública registrada. Nunca viaja la clave privada.
func LoadNKeyOption(nkeyPath string) (nats.Option, error) {
	seed, err := os.ReadFile(nkeyPath)
	if err != nil {
		return nil, fmt.Errorf("crypto/nkey: read seed file %q: %w", nkeyPath, err)
	}

	kp, err := nkeys.FromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("crypto/nkey: parse seed: %w", err)
	}

	pub, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("crypto/nkey: get public key: %w", err)
	}

	return nats.Nkey(pub, func(nonce []byte) ([]byte, error) {
		sig, err := kp.Sign(nonce)
		if err != nil {
			return nil, fmt.Errorf("crypto/nkey: sign nonce: %w", err)
		}
		return sig, nil
	}), nil
}
