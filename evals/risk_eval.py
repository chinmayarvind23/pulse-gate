"""Compare actual worker predictions with a fixed rules baseline on held-out data."""

import argparse
import json
import math
import sys
import time
from pathlib import Path

import httpx
import numpy as np
from sklearn.metrics import brier_score_loss, confusion_matrix, log_loss

from scripts.evidence import external_directory, provenance, sha256
from scripts.train_model import features


def measurements(labels, scores, threshold):
    """Expose confusion counts as well as ratios so class imbalance remains visible."""
    tn, fp, fn, tp = map(
        int, confusion_matrix(labels, np.asarray(scores) >= threshold, labels=[0, 1]).ravel()
    )
    return {
        "tp": tp,
        "fp": fp,
        "tn": tn,
        "fn": fn,
        "precision": tp / max(1, tp + fp),
        "recall": tp / max(1, tp + fn),
        "f1": 2 * tp / max(1, 2 * tp + fp + fn),
        "false_positive_rate": fp / max(1, fp + tn),
        "brier_score": float(brier_score_loss(labels, scores)),
        "log_loss": float(log_loss(labels, scores, labels=[0, 1])),
    }


def main():
    """Measure HTTP separately and verify every returned probability against the export."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worker-url", default="http://localhost:8081")
    parser.add_argument(
        "--dataset", type=Path, default=Path(__file__).parent / "data/risk_eval.jsonl"
    )
    parser.add_argument(
        "--model",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "apps/risk-worker/model/model.json",
    )
    parser.add_argument("--predictions", type=Path)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    out = external_directory(args.output)
    rows = [json.loads(line) for line in args.dataset.read_text().splitlines()]
    model = json.loads(args.model.read_text())
    latencies, scores, predictions, labels, baseline, parity = [], [], [], [], [], []
    loaded = (
        [json.loads(line) for line in args.predictions.read_text().splitlines()]
        if args.predictions
        else None
    )
    if loaded is not None and len(loaded) != len(rows):
        raise ValueError("Prediction count differs from dataset")
    with httpx.Client(timeout=5) as client:
        for i, row in enumerate(rows):
            event = {key: value for key, value in row.items() if key != "label"}
            if loaded is None:
                started = time.perf_counter_ns()
                response = client.post(args.worker_url + "/score", json=event)
                response.raise_for_status()
                latencies.append((time.perf_counter_ns() - started) / 1e6)
                prediction = response.json()
            else:
                prediction = loaded[i]
            if (
                prediction["event_id"] != row["event_id"]
                or prediction["model_version"] != model["model_version"]
            ):
                raise ValueError("Prediction identity mismatch")
            z = model["intercept"] + sum(
                x * w for x, w in zip(features(event), model["weights"], strict=True)
            )
            expected = 1 / (
                1 + math.exp(-(z * model["calibration_slope"] + model["calibration_intercept"]))
            )
            parity.append(abs(expected - prediction["score"]))
            if prediction["risky"] != (expected >= model["threshold"]):
                raise ValueError("Threshold decision mismatch")
            scores.append(prediction["score"])
            labels.append(row["label"])
            baseline.append(
                float(
                    row["amount_cents"] > 400000
                    or row["velocity_5m"] >= 9
                    or row["prior_declines"] >= 4
                )
            )
            predictions.append(prediction)
    if max(parity) > 1e-12:
        raise ValueError("Rust/Python probability mismatch")
    result = {
        **provenance(),
        "command": sys.argv,
        "synthetic_only": True,
        "dataset_sha256": sha256(args.dataset),
        "model_sha256": sha256(args.model),
        "count": len(rows),
        "model": measurements(labels, scores, model["threshold"]),
        "rules_baseline": measurements(labels, baseline, 0.5),
        "max_export_error": max(parity),
        "http_p99_ms": float(np.percentile(latencies, 99)) if latencies else None,
        "latency_scope": "HTTP serialization, network and inference; in-process timing is a separate worker command",
    }
    (out / "evaluation.json").write_text(json.dumps(result, indent=2) + "\n")
    (out / "predictions.jsonl").write_text("".join(json.dumps(p) + "\n" for p in predictions))
    (out / "http-latencies-ms.json").write_text(json.dumps(latencies) + "\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
