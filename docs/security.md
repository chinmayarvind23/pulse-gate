# Security

## Trust boundary

The public boundary is the webhook endpoint. Payloads are untrusted until their signature is verified. Redis and the worker should live on a private network.

## Controls

- HMAC-SHA256 over exact raw request bytes.
- Constant-time comparison.
- Request body size limit.
- Strict field validation before stream admission.
- Secrets supplied through environment or Kubernetes Secret, never committed.
- Non-root runtime containers where practical.
- No raw payment payloads in normal logs.
- Diagnostic `/score` endpoint restricted to internal/evaluation networks in production.

## Threats considered

Replay: mitigated within the idempotency retention window.

Payload tampering: HMAC failure.

Memory exhaustion: bounded request body and server timeouts.

Retry amplification: duplicate suppression plus dependency timeouts.

Secret leakage: keep secret material out of source, command history artifacts and telemetry.

Redis exposure: bind privately and require managed-service auth/TLS in production.

## Not present

There is no agent or LLM tool execution path, so prompt injection and tool authorization are not applicable to this project. Adding them would create risk without serving the product requirement.
