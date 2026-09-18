package config

import "testing"

// TestLoadBounds rejects settings that invalidate stream retention or queue bounds.
func TestLoadBounds(t *testing.T) {
	t.Setenv("HOOKGUARD_HMAC_SECRET", "test-secret")
	for _, tc := range []struct{ key, value string }{{"HOOKGUARD_IDEMPOTENCY_TTL_SECONDS", "0"}, {"HOOKGUARD_IDEMPOTENCY_TTL_SECONDS", "31536001"}, {"HOOKGUARD_MAX_BODY_BYTES", "0"}, {"HOOKGUARD_MAX_QUEUE", "-1"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	t.Setenv("HOOKGUARD_STREAM", "same")
	t.Setenv("HOOKGUARD_RESULT_STREAM", "same")
	if _, err := Load(); err == nil {
		t.Fatal("shared input/output accepted")
	}
}
