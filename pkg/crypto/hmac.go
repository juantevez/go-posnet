package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignMessage genera una firma HMAC-SHA256 del payload con la clave dada.
// Retorna la firma como string hexadecimal.
// Uso: la firma va en el header X-Signature de los mensajes NATS
// para verificar que el publisher es legítimo.
func SignMessage(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyMessage verifica que la firma HMAC-SHA256 sea válida para el payload.
// Usa comparación en tiempo constante (hmac.Equal) para prevenir timing attacks.
func VerifyMessage(payload []byte, signature string, secret []byte) bool {
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}
