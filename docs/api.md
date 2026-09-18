# API

The gateway listens on port 8080 by default. The worker's diagnostic API listens on port 8081. The operator page is served at the gateway root.

## Submit an event

Send `POST /v1/events` with `Content-Type: application/json` and `X-PulseGate-Signature`, the lowercase hexadecimal HMAC-SHA256 of the exact request body using `PULSEGATE_HMAC_SECRET`.

```json
{"event_id":"order-123-paid","amount_cents":2500,"currency":"USD","merchant_category":5812,"country":"US","card_present":true,"hour_utc":12,"velocity_5m":1,"velocity_1h":2,"prior_declines":0}
```

All fields are required and non-null. Extra fields and duplicate JSON keys are rejected. The event identifier contains one to 128 ASCII letters, digits, underscores, hyphens, periods or colons. Currency and country are uppercase codes of three and two letters. Amount is a non-negative signed 64-bit integer. Merchant category is between zero and 9999, hour is between zero and 23, and counts are non-negative signed 32-bit integers.

| Status | Meaning |
| --- | --- |
| 202 | Queued, or the same event was already admitted |
| 400 | Invalid JSON or event fields |
| 401 | Missing or invalid signature |
| 409 | The identifier was already used with different data |
| 413 | Body exceeds the configured limit |
| 503 | Shared state unavailable or outstanding queue full; retry later |

A duplicate response includes `X-PulseGate-Duplicate: true`. A 503 response includes `Retry-After: 1`. An empty 202 body confirms admission, not a completed decision. After signature verification, the gateway canonicalizes the validated fields, so formatting and property order do not change replay identity.

The included client signs requests consistently:

```sh
export PULSEGATE_HMAC_SECRET='<your secret>'
python -m scripts.smoke_test
```

## Inspect state

`GET /v1/status` returns `outstanding` and admission availability. `GET /v1/decisions` returns at most the 50 most recent decisions. These endpoints require the same signature header, computed over the UTF-8 URL path itself, such as `/v1/decisions`. They do not accept arbitrary Redis commands or key names.

A decision contains `event_id`, `score`, `risky` and `model_version`. The score is a model probability; `risky` applies the frozen threshold embedded with the model.

## Worker diagnostics

`POST /score` accepts the same event and returns a decision without writing it to Redis. Keep this endpoint on the internal network or local loopback. Its request body is bounded independently of the gateway.

## Operational endpoints

Both services expose `/healthz`, `/readyz` and `/metrics`. Liveness means the process can answer; readiness also depends on shared state. The worker must complete a consumer cycle before becoming ready. Telemetry endpoints are intended for the local operations network.
