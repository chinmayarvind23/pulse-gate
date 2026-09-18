"""Train, calibrate and export the exact feature contract used by the worker."""

import argparse
import json
import random
import sys

import numpy as np
import sklearn
from sklearn.calibration import CalibratedClassifierCV
from sklearn.frozen import FrozenEstimator
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import f1_score

from scripts.evidence import external_directory, provenance, sha256
from scripts.generate_dataset import make


def features(event: dict) -> list[float]:
    """Mirror the Rust transform, including order, scaling and saturation."""
    return [
        min(max(event["amount_cents"] / 100000, 0), 10),
        float(event["hour_utc"] <= 5 or event["hour_utc"] >= 23),
        float(not event["card_present"]),
        float(event["country"] != "US"),
        min(max(event["velocity_5m"] / 10, 0), 10),
        min(max(event["velocity_1h"] / 30, 0), 10),
        min(max(event["prior_declines"] / 5, 0), 10),
    ]


def main() -> None:
    """Keep fitting, calibration and threshold selection on disjoint generated splits."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    out = external_directory(args.output)
    splits = {}
    metadata = {}
    for name, seed, count in [
        ("train", 1729, 12000),
        ("calibration", 1730, 4000),
        ("threshold", 1731, 4000),
    ]:
        random.seed(seed)
        rows = [dict(make(i), event_id=f"{name}-{i}") for i in range(count)]
        path = out / f"{name}.jsonl"
        path.write_bytes(
            "".join(json.dumps(row, separators=(",", ":")) + "\n" for row in rows).encode()
        )
        splits[name] = (
            np.array([features(row) for row in rows]),
            np.array([row["label"] for row in rows]),
        )
        metadata[name] = {"seed": seed, "count": count, "sha256": sha256(path)}
    base = LogisticRegression(C=1.0, max_iter=2000, random_state=1729).fit(*splits["train"])
    calibrated = CalibratedClassifierCV(FrozenEstimator(base), method="sigmoid").fit(
        *splits["calibration"]
    )
    probabilities = calibrated.predict_proba(splits["threshold"][0])[:, 1]
    threshold = float(
        max(
            np.linspace(0.05, 0.95, 901),
            key=lambda value: (
                f1_score(splits["threshold"][1], probabilities >= value),
                -abs(value - 0.5),
            ),
        )
    )
    calibrator = calibrated.calibrated_classifiers_[0].calibrators[0]
    # sklearn's sigmoid stores the opposite sign to the standard sigmoid(logit).
    artifact = {
        "schema_version": 1,
        "model_version": "logreg-calibrated-v2",
        "feature_version": "payment-risk-v1",
        "feature_names": [
            "amount_100k",
            "night",
            "card_not_present",
            "non_us",
            "velocity_5m_10",
            "velocity_1h_30",
            "declines_5",
        ],
        "weights": base.coef_[0].tolist(),
        "intercept": float(base.intercept_[0]),
        "calibration_slope": float(-calibrator.a_),
        "calibration_intercept": float(-calibrator.b_),
        "threshold": threshold,
        "training": {
            "sklearn_version": sklearn.__version__,
            "estimator": "LogisticRegression(C=1.0, max_iter=2000)",
            "calibration": "FrozenEstimator + sigmoid",
            "splits": metadata,
        },
    }
    (out / "model.json").write_text(json.dumps(artifact, indent=2) + "\n")
    (out / "training-manifest.json").write_text(
        json.dumps(
            {
                **provenance(),
                "command": sys.argv,
                "splits": metadata,
                "model_sha256": sha256(out / "model.json"),
            },
            indent=2,
        )
        + "\n"
    )
    print(out / "model.json")


if __name__ == "__main__":
    main()
