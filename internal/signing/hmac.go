package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign computes HMAC-SHA256 of payload using the given secret and returns the hex-encoded signature.
func Sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that the given signature matches the HMAC-SHA256 of payload with the given secret.
func Verify(payload []byte, secret, signature string) bool {
	expected := Sign(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}
