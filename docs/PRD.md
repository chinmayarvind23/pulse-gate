# Product requirements document

## Product

PulseGate is a payment-event reliability and risk gateway for fintech platform teams that must accept provider webhooks quickly without allowing retries, duplicates or downstream scoring failures to create duplicate financial side effects.

## Problem statement

A payment platform receives signed webhook events from external providers. Delivery is at least once from the platform's perspective because providers retry when acknowledgement is delayed or lost. Traffic can burst. Downstream risk logic can be slower or temporarily unavailable. The platform needs a bounded ingress path that validates authenticity and required fields, suppresses duplicate event admission, durably hands work to an asynchronous processor, and makes failure visible.

## Primary user

A backend or platform engineer responsible for payment-provider integrations and transaction processing reliability.

## Inputs

- Raw JSON webhook payload.
- HMAC signature over the exact raw bytes.
- Provider event identifier used as the idempotency key.
- Transaction fields needed by the risk scorer.

## Outputs

- `202 Accepted` when an authentic, valid event is admitted or already known.
- `401` for a bad signature.
- `400` for malformed event data.
- `413` for an oversized body.
- `503` when shared admission state is unavailable.
- Asynchronous risk decision containing event id, score, threshold result and model version.
- Logs and metrics sufficient to explain latency, failures, duplicates, queue behavior and scoring quality.

## User value

The provider receives a fast acknowledgement that does not wait for risk scoring. The platform can retry and replay safely. Operators can distinguish ingress failure from downstream backlog. Model behavior has an explicit baseline and evaluation harness.

## Functional requirements

- FR1: Verify HMAC before admitting data.
- FR2: Reject request bodies larger than the configured limit.
- FR3: Validate the minimum event schema before enqueue.
- FR4: Atomically suppress duplicate event admission and add first-seen events to the stream.
- FR5: Return quickly without waiting for scoring.
- FR6: Consume events through a Redis consumer group.
- FR7: Produce one risk decision per successfully processed stream entry.
- FR8: Expose deterministic `/score` diagnostics for the eval harness.
- FR9: Expose health/readiness endpoints and ingress metrics.
- FR10: Provide load, replay, eval and failure-injection commands.

## Non-functional requirements

- NFR1: No secret values in logs or repository history.
- NFR2: Request handler has explicit body, read, write and dependency timeouts.
- NFR3: Shared idempotency works across gateway replicas.
- NFR4: Request acknowledgement is independent from risk-worker health after Redis admission succeeds.
- NFR5: Metrics use bounded-cardinality labels.
- NFR6: Components run as non-root containers where practical.
- NFR7: CI runs deterministic unit tests and static checks for all three languages.
- NFR8: The portfolio version remains understandable enough to explain every component without framework hand-waving.

## Edge cases

- Same event id, same payload: acknowledge duplicate without another stream entry.
- Same event id, different payload: treat as a contract violation in the production extension and alert. The MVP suppresses it and records duplicate count.
- Signature over parsed JSON rather than raw bytes: reject because signatures bind exact bytes.
- Redis unavailable: return 503 rather than acknowledge work that was not admitted.
- Worker unavailable: ingress continues while Redis is healthy; queue depth grows and triggers operator action.
- Poison payload that passed ingress but fails worker deserialization: move to a dead-letter path in the production extension. MVP logs and leaves the entry pending for inspection.
- Expired idempotency key followed by a very late replay: documented limitation. Production TTL must match provider replay and financial-reconciliation policy.
