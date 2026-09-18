# Evaluation strategy

## Principle

Evaluation is split into system correctness, system performance and model quality. Passing one category cannot hide a failure in another.

## Baseline

The ML baseline marks an event risky if it crosses a fixed amount, short-window velocity or prior-decline rule. The baseline is intentionally understandable. A model earns complexity only if it improves the chosen quality tradeoff on the held-out set.

## Dataset

The checked-in seed file is only a smoke fixture. `scripts/generate_dataset.py` creates a larger deterministic synthetic dataset with a fixed random seed. Synthetic data is useful for testing the harness, edge cases and metric plumbing. It is not evidence of real fraud-detection effectiveness.

## Release evals

1. Signature rejection.
2. Schema rejection.
3. First admission.
4. Duplicate replay suppression.
5. Concurrent duplicate replay.
6. Redis outage behavior.
7. Worker outage while ingress stays healthy.
8. Queue recovery after restart.
9. Scorer versus rules baseline.
10. Tail-latency measurement.

## Regression policy

CI runs deterministic unit tests on every change. Full 10k+ RPS and million-replay tests are release checks because hosted CI noise can invalidate latency evidence. A small load smoke test can run in CI to detect functional regressions.

## Failure interpretation

A failed performance gate is information. Record the bottleneck, profiler or metric evidence, the change tried and the new measurement. Do not remove the failing test or lower a threshold silently.
