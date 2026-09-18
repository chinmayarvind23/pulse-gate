# Low-level design

## Gateway modules

`config`: parse and validate process configuration once.

`signature`: compute and constant-time compare HMAC-SHA256 over raw payload bytes.

`model`: deserialize and validate request invariants that must hold before admission.

`admission`: own atomic idempotency and stream append. The Redis Lua script is the concurrency-critical unit.

`telemetry`: define bounded-cardinality Prometheus metrics.

`server`: HTTP transport, deadlines, status mapping and dependency wiring.

## Worker modules

`model.rs`: typed payment event, explicit logistic score and decision type.

`main.rs`: Redis consumer group loop, persistence of decisions, input acknowledgement and diagnostic `/score` endpoint.

## Data structures

The ingress dedup structure is a Redis key per event id with TTL. Lookup is expected O(1). Redis Streams provide ordered IDs and consumer-group delivery. The scoring model is O(f) for `f` numeric features and allocates only the returned decision plus deserialization state.

## API contract

`POST /v1/events`

Required header: `X-PulseGate-Signature: <hex hmac-sha256>`.

Success: `202` for both first-seen and duplicate valid events. Duplicates add `X-PulseGate-Duplicate: true` so test clients can verify behavior without changing provider retry semantics.

`POST /score`

Diagnostic worker endpoint used by the evaluation harness. It returns `{event_id, score, risky, model_version}` and must not become the payment-provider endpoint.

## Scoring math

The worker computes a linear logit:

`z = b + sum(w_i * x_i)`

and probability:

`p = 1 / (1 + exp(-z))`

with decision `risky = p >= 0.58`.

Features are scaled to bounded ranges before multiplication. This keeps extreme synthetic values from dominating without requiring a preprocessing library. A production model should serialize the exact training-time transform and calibration metadata rather than hard-code weights.

## Complexity

- HMAC: O(payload bytes).
- JSON parsing: O(payload bytes).
- Redis idempotency lookup and key write: expected O(1), plus XADD.
- Model score: O(number of features), effectively constant for this schema.

## Review hotspots

1. Lua atomicity and error interpretation.
2. Signature calculated on exact raw bytes.
3. Request body and timeouts.
4. Worker acknowledgement ordering.
5. Pending-entry recovery after worker death.
6. Idempotency TTL policy.
7. Any new metric label that can grow with user input.
