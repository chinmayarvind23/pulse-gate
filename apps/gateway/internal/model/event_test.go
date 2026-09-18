package model

import (
	"encoding/json"
	"testing"
)

const valid = `{"event_id":"evt_1","amount_cents":100,"currency":"USD","merchant_category":5812,"country":"US","card_present":false,"hour_utc":12,"velocity_5m":0,"velocity_1h":0,"prior_declines":0}`

// TestDecodeContract prevents permissive Go defaults from admitting worker poison.
func TestDecodeContract(t *testing.T) {
	if _, err := Decode([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(valid), &fields); err != nil {
		t.Fatal(err)
	}
	for field := range fields {
		t.Run("missing_"+field, func(t *testing.T) {
			clone := map[string]any{}
			for k, v := range fields {
				clone[k] = v
			}
			delete(clone, field)
			raw, _ := json.Marshal(clone)
			if _, err := Decode(raw); err == nil {
				t.Fatal("missing field accepted")
			}
		})
		t.Run("null_"+field, func(t *testing.T) {
			clone := map[string]any{}
			for k, v := range fields {
				clone[k] = v
			}
			clone[field] = nil
			raw, _ := json.Marshal(clone)
			if _, err := Decode(raw); err == nil {
				t.Fatal("null accepted")
			}
		})
	}
	for _, tc := range []struct {
		key   string
		value any
	}{{"event_id", ""}, {"event_id", "bad id"}, {"amount_cents", -1}, {"currency", "usd"}, {"country", "USA"}, {"hour_utc", 24}, {"velocity_5m", 2147483648}, {"velocity_1h", -1}, {"prior_declines", -1}, {"merchant_category", 10000}} {
		t.Run(tc.key, func(t *testing.T) {
			clone := map[string]any{}
			for k, v := range fields {
				clone[k] = v
			}
			clone[tc.key] = tc.value
			raw, _ := json.Marshal(clone)
			if _, err := Decode(raw); err == nil {
				t.Fatalf("accepted %v", tc.value)
			}
		})
	}
}
