package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestValid protects the security boundary against accidental replacement with a
// normal string comparison or a signature over a transformed payload.
func TestValid(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event_id":"evt_1"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !Valid(secret, body, sig) {
		t.Fatal("expected valid signature")
	}
	if Valid(secret, body, sig+"00") {
		t.Fatal("expected invalid signature")
	}
}
