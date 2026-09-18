# API

Use the gateway to send payment events and check their progress. It listens on port 8080, with the operator page at `/`. The separate worker listens on port 8081 for direct scoring checks.

Sending an event and getting its decision are separate steps. `202 Accepted` means the event is in the queue or was already accepted. A worker finishes the decision afterward.

## Submit an event

Send `POST /v1/events` with `Content-Type: application/json` and `X-PulseGate-Signature`, the lowercase hexadecimal HMAC-SHA256 of the exact request body using `PULSEGATE_HMAC_SECRET`.

The signature lets the gateway check that the sender knows the shared secret and that the message bytes have not changed. Sign the exact bytes you send, including spaces and line breaks. The sample client below handles this for you.

```json
{"event_id":"order-123-paid","amount_cents":2500,"currency":"USD","merchant_category":5812,"country":"US","card_present":true,"hour_utc":12,"velocity_5m":1,"velocity_1h":2,"prior_declines":0}
```

Every field is required. Fields cannot be `null`, and extra fields or repeated JSON keys are rejected.

| Field | Meaning and allowed values |
| --- | --- |
| `event_id` | Stable ID for the event. Reuse it when retrying the same event. One to 128 ASCII letters, digits, underscores, hyphens, periods or colons. |
| `amount_cents` | Amount in cents, from zero through the maximum signed 64-bit integer. |
| `currency`, `country` | Uppercase letter codes: three letters for currency and two for country. |
| `merchant_category` | Merchant category number, from zero to 9999. |
| `card_present` | Boolean indicating whether the card was physically present. |
| `hour_utc` | Hour of the transaction in UTC, from zero to 23. |
| `velocity_5m`, `velocity_1h` | Transaction counts over the preceding five minutes and hour. Non-negative signed 32-bit integers. |
| `prior_declines` | Count of earlier declines. A non-negative signed 32-bit integer. |

The sender supplies these transaction details; PulseGate validates the fields but does not look up the underlying payment history.

| Status | Meaning |
| --- | --- |
| 202 | Queued, or the same event was already accepted |
| 400 | Invalid JSON or event fields |
| 401 | Missing or invalid signature |
| 409 | The identifier was already used with different data |
| 413 | Body exceeds the configured limit |
| 503 | Redis is unavailable or the queue is full; retry later |

A repeated event receives `202` with `X-PulseGate-Duplicate: true`. If the same retained ID arrives with different field values, the response is `409`. Changing only JSON spacing or field order still counts as the same event: after checking the original signature, the gateway puts the fields into a consistent format before comparing them.

A `503` response includes `Retry-After: 1`, asking the sender to wait before retrying. A `202` response has an empty body; read the decisions endpoint to see the worker's result.

The included client signs requests consistently:

```sh
export PULSEGATE_HMAC_SECRET='<your secret>'
python -m scripts.smoke_test
```

## Inspect state

`GET /v1/status` returns `outstanding`, the number of unfinished events. A successful status read shows that the gateway can read Redis; a full queue can still reject new events. `GET /v1/decisions` returns at most the 50 most recent decisions.

Both reads need the same signature header, computed over the UTF-8 URL path itself, such as `/v1/decisions`. They expose fixed views of the data, rather than accepting arbitrary Redis commands or key names.

These responses use `Cache-Control: no-store` so browsers and intermediaries should not cache the authenticated views or their error responses.

A decision contains `event_id`, `score`, `risky` and `model_version`. The score is a model output between zero and one. `risky` is true when the score reaches the model's saved threshold; the operator page displays this as Review. The model uses synthetic data, so this is a sample decision rather than an established real-world fraud probability.

## Worker diagnostics

`POST /score` accepts the same event and returns a decision without writing it to Redis. Keep this endpoint on the internal network or local loopback. Its request body is bounded independently of the gateway.

## Operational endpoints

Both services expose these endpoints for local monitoring:

- `/healthz`: can the running process answer a request?
- `/readyz`: can the service reach Redis and do its work? The worker must also complete a successful processing-loop cycle.
- `/metrics`: measurements that Prometheus collects for the dashboard.
