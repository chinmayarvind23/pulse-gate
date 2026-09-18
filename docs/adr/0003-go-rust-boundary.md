# ADR 0003: Go ingress, Rust scoring worker

Status: accepted with an explicit simplification option.

Decision: Go owns I/O-bound webhook admission. Rust owns compute-bound typed scoring.

Reason: the boundary maps to independent latency and scaling concerns.

Tradeoff: two language toolchains increase maintenance. If profiling and team ownership do not justify the split, consolidate to Go.
