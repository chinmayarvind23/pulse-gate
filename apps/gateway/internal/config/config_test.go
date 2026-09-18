package config

import "testing"

// TestLoadBounds rejects settings that invalidate stream retention or queue bounds.
func TestLoadBounds(t *testing.T) {
	t.Setenv("PULSEGATE_HMAC_SECRET", "test-secret")
	for _, tc := range []struct{ key, value string }{{"PULSEGATE_IDEMPOTENCY_TTL_SECONDS", "0"}, {"PULSEGATE_IDEMPOTENCY_TTL_SECONDS", "31536001"}, {"PULSEGATE_MAX_BODY_BYTES", "0"}, {"PULSEGATE_MAX_QUEUE", "-1"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	t.Setenv("PULSEGATE_STREAM", "same")
	t.Setenv("PULSEGATE_RESULT_STREAM", "same")
	if _, err := Load(); err == nil {
		t.Fatal("shared input/output accepted")
	}
}
