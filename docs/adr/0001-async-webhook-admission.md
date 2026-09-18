# ADR 0001: Acknowledge after safe admission, not after business processing

Status: accepted.

Decision: verify authenticity and schema, atomically admit the event, then return 202. Risk scoring is asynchronous.

Reason: downstream latency and outages must not create webhook retry amplification.

Consequence: operators need queue-lag telemetry because processing failures are no longer visible in the provider response.
