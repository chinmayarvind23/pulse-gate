package model

import (
	"encoding/json"
	"fmt"
)

type PaymentEvent struct {
	EventID          string `json:"event_id"`
	AmountCents      int64  `json:"amount_cents"`
	Currency         string `json:"currency"`
	MerchantCategory int    `json:"merchant_category"`
	Country          string `json:"country"`
	CardPresent      bool   `json:"card_present"`
	HourUTC          int    `json:"hour_utc"`
	Velocity5m       int    `json:"velocity_5m"`
	Velocity1h       int    `json:"velocity_1h"`
	PriorDeclines    int    `json:"prior_declines"`
}

// Decode validates only deterministic request invariants needed before admission.
// Risk semantics belong in the worker. This split keeps the hot path cheap while
// still rejecting malformed data that would otherwise poison the stream.
func Decode(raw []byte) (PaymentEvent, error) {
	var event PaymentEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return event, fmt.Errorf("decode JSON: %w", err)
	}
	if event.EventID == "" {
		return event, fmt.Errorf("event_id is required")
	}
	if event.AmountCents < 0 {
		return event, fmt.Errorf("amount_cents must be non-negative")
	}
	if event.Currency == "" {
		return event, fmt.Errorf("currency is required")
	}
	if event.HourUTC < 0 || event.HourUTC > 23 {
		return event, fmt.Errorf("hour_utc must be between 0 and 23")
	}
	if event.Velocity5m < 0 || event.Velocity1h < 0 || event.PriorDeclines < 0 {
		return event, fmt.Errorf("velocity and decline counts must be non-negative")
	}
	return event, nil
}
