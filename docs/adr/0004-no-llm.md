# ADR 0004: Do not add an LLM

Status: accepted.

Decision: use deterministic scoring and standard software controls.

Reason: transaction-risk classification does not require language generation. Adding an LLM would worsen latency, cost, reproducibility and attack surface without solving a product requirement.
