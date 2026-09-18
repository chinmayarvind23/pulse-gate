# Evaluation

Generate the deterministic synthetic dataset with `python scripts/generate_dataset.py`, start the worker, then run `python evals/risk_eval.py`.

The evaluator compares the Rust scorer with a simple deterministic baseline and reports precision, recall, F1, false-positive rate and request-level scoring latency. The synthetic label generator is intentionally separate from the Rust model weights. These numbers demonstrate the evaluation mechanism and serving discipline, not real fraud-detection performance.
