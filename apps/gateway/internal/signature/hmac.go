package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Valid compares signatures in constant time. Signature verification occurs before
// JSON parsing so untrusted payloads cannot enter the event stream. HMAC is used for
// the portfolio provider contract because it is simple, deterministic and mirrors
// common webhook-authentication patterns.
func Valid(secret string, body []byte, provided string) bool {
	decoded, err := hex.DecodeString(provided)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, decoded)
}
