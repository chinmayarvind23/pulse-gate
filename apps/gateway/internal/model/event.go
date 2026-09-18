package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type PaymentEvent struct {
	EventID          string `json:"event_id"`
	AmountCents      int64  `json:"amount_cents"`
	Currency         string `json:"currency"`
	MerchantCategory int32  `json:"merchant_category"`
	Country          string `json:"country"`
	CardPresent      bool   `json:"card_present"`
	HourUTC          int32  `json:"hour_utc"`
	Velocity5m       int32  `json:"velocity_5m"`
	Velocity1h       int32  `json:"velocity_1h"`
	PriorDeclines    int32  `json:"prior_declines"`
}

// Decode rejects values that cannot cross the Go/Rust wire contract safely.
func Decode(raw []byte) (PaymentEvent, error) {
	var event PaymentEvent
	fields, err := uniqueFields(raw)
	if err != nil {
		return event, fmt.Errorf("invalid JSON")
	}
	for _, key := range []string{"event_id", "amount_cents", "currency", "merchant_category", "country", "card_present", "hour_utc", "velocity_5m", "velocity_1h", "prior_declines"} {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return event, fmt.Errorf("%s is required", key)
		}
	}
	if len(fields) != 10 {
		return event, fmt.Errorf("unexpected event fields")
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return event, fmt.Errorf("invalid field type or range")
	}
	if len(event.EventID) == 0 || len(event.EventID) > 128 {
		return event, fmt.Errorf("event_id must have 1 to 128 characters")
	}
	for _, c := range event.EventID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.' || c == ':') {
			return event, fmt.Errorf("invalid event_id")
		}
	}
	if event.AmountCents < 0 {
		return event, fmt.Errorf("amount_cents must be non-negative")
	}
	if !upperASCII(event.Currency, 3) || !upperASCII(event.Country, 2) {
		return event, fmt.Errorf("currency and country require uppercase codes")
	}
	if event.MerchantCategory < 0 || event.MerchantCategory > 9999 {
		return event, fmt.Errorf("invalid merchant_category")
	}
	if event.HourUTC < 0 || event.HourUTC > 23 {
		return event, fmt.Errorf("hour_utc must be between 0 and 23")
	}
	if event.Velocity5m < 0 || event.Velocity1h < 0 || event.PriorDeclines < 0 {
		return event, fmt.Errorf("counts must be non-negative")
	}
	return event, nil
}

// uniqueFields rejects duplicate keys rather than relying on language-specific
// first/last-value behavior across a signed cross-language boundary.
func uniqueFields(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("object required")
	}
	fields := make(map[string]json.RawMessage, 10)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid key")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate key")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	return fields, nil
}

// upperASCII keeps country and currency normalization identical across services.
func upperASCII(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
