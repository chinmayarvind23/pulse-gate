# ADR 0002: Use Redis for idempotency and the MVP event stream

Status: accepted for portfolio scope.

Decision: use a Redis Lua operation for idempotency plus XADD and consumer groups for workers.

Reason: one dependency supplies the shared correctness boundary and async queue while staying small enough for a 3 to 4 hour implementation.

Rejected: process memory because it loses replica correctness; Kafka because its operational surface is unnecessary for this scope.
