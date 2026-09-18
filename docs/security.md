# Security and data handling

The gateway verifies HMAC-SHA256 over raw bytes before parsing event data. It uses constant-time comparison, bounds request bodies and rejects malformed, missing, null, extra and ambiguous fields. Canonicalization happens only after authentication. Reusing an event identifier with a changed validated payload is a conflict.

The local operator page takes the signing secret into tab memory. It does not store it in browser storage or send it as an ordinary request field. Recent decisions and queue inspection require a signature over the requested path. The page renders event content as text, not injected HTML.

Keep `.env` out of version control. Kubernetes deployments read the signing key from a Secret. Replace example values and restrict access to both the source environment and cluster credentials. Rotating the signing key requires coordinated provider and gateway configuration.

The local deployment uses synthetic transaction fields. It does not collect card numbers, account credentials or personal identity records. Application logs report processing failures and identifiers without logging signing secrets or complete request bodies.

Redis and worker diagnostics are internal services. Compose binds diagnostic ports to loopback. Before placing any endpoint on an untrusted network, configure transport encryption, access control and request quotas appropriate to that environment. The supplied local configuration does not make a production payments security claim.

Replay protection depends on retained Redis state and the configured identifier lifetime. A valid signature alone does not establish transaction freshness. Actual external side effects need their own durable idempotency boundary; a recorded risk decision is not a money transfer.
