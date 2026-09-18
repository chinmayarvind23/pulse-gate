# Reliability

## Retry model

Provider delivery should be assumed at least once. Duplicate deliveries are normal behavior, not exceptional behavior. The gateway therefore makes idempotency part of admission rather than a downstream cleanup task.

## Atomic admission

The idempotency key and stream entry are created in one Redis Lua operation. This avoids the crash gap between a separate dedup write and enqueue.

## Backpressure

The gateway does not keep an unbounded process-local work queue. Redis is the admission dependency. If Redis cannot confirm admission inside the dependency timeout, the gateway returns 503 and lets the provider retry according to its policy.

## Worker recovery

Consumer groups keep unacknowledged entries pending. The MVP documents pending-entry reclamation as a production follow-up. Before claiming resilient at-least-once processing across worker death, implement `XAUTOCLAIM` or an equivalent recovery loop and add a failure test.

## Circuit breakers

A circuit breaker is not required between the gateway and Redis in the MVP because Redis is the required admission store. Failing fast with a short timeout already provides the desired semantics. If Redis is unavailable, acknowledging requests would be less safe than returning an error.

## Graceful degradation

Risk scoring may lag while ingress stays available. The system must expose this degraded state through queue lag. Any downstream decision consumer must know whether stale risk decisions are acceptable for its business action.
