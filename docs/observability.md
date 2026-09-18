# Observability

## Questions the telemetry must answer

- Are providers receiving fast acknowledgements?
- Are duplicates increasing because a provider or network path is retrying?
- Is Redis slowing the admission path?
- Is worker capacity lower than admitted traffic?
- Are risk decisions being produced and acknowledged?
- Did a deployment increase bad signatures, 5xx responses or queue lag?

## Gateway metrics

`pulsegate_http_requests_total{code}`

`pulsegate_ack_duration_seconds`

`pulsegate_duplicates_total`

`pulsegate_admission_errors_total`

Add Redis dependency latency and stream-admission outcome metrics before production deployment.

## Logging

Use structured logs. Include request id or event id only where operationally required and avoid logging full payment payloads. Never log the HMAC secret or raw authorization material.

## Alerts for a production extension

- p99 acknowledgement above provider safety budget
- 5xx rate above 1 percent for 5 minutes
- sustained queue lag growth
- no worker decisions while ingress remains active
- signature failure rate anomaly
- Redis memory or availability degradation
