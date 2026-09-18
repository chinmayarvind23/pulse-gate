# Security and data handling

Each event carries an HMAC-SHA256 signature made with a secret shared by the sender and gateway. The gateway checks it against the exact message bytes before reading the event fields. This checks that the sender knows the secret and that the message has not changed. The comparison uses constant-time code to avoid revealing matching parts through comparison timing.

The gateway also limits message size and rejects missing, invalid, repeated or unexpected fields. After checking the signature, it puts valid fields into a consistent format for comparison. Reusing a remembered event ID with different data is rejected.

The operator page keeps the signing secret in the current tab's memory. It does not save it in browser storage or include it as a request field. Reading decisions or queue status also requires a signature, made from the requested URL path. Event data is displayed as text so it cannot be treated as page code.

Keep `.env` out of version control. Kubernetes reads the signing key from a Secret. Replace the example values and limit who can read the key and cluster credentials. When changing the key, update the sender and gateway together so they continue to agree.

The examples use synthetic transaction data. The event format does not include card numbers, account passwords or identity records. Use event IDs that do not contain personal information. Logs include event identifiers and processing failures, but omit signing secrets and full request bodies.

Redis and the worker's direct scoring API are internal services. Compose exposes their ports only on your own machine. An installation on an untrusted network needs encrypted connections, access controls and request limits appropriate to that environment. The supplied setup is for local use.

A valid signature can be copied along with an old message, so it does not prove that an event is new. Redis recognizes repeats using saved event IDs for a limited time. If a separate system uses a decision to move money, that system needs its own durable records and duplicate protection.
