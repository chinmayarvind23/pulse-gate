package model

import "testing"

func TestDecodeRejectsMissingEventID(t *testing.T) {
	_, err := Decode([]byte(`{"amount_cents":100,"currency":"USD","hour_utc":12}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDecodeAcceptsMinimalValidEvent(t *testing.T) {
	_, err := Decode([]byte(`{"event_id":"evt_1","amount_cents":100,"currency":"USD","hour_utc":12}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
