"""Generate signed JSON targets for an external HTTP load driver."""

import argparse
import base64
import json
import os
import sys
import uuid

from scripts.common import encode_event, signed_headers
from scripts.evidence import external_directory, provenance, sha256


def main():
    """Freeze payload identity before replay and retain an exact target-file hash."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://gateway:8080/v1/events")
    parser.add_argument("--count", type=int, required=True)
    parser.add_argument("--unique", type=int)
    parser.add_argument("--output", required=True)
    parser.add_argument("--prefix", default="load-" + uuid.uuid4().hex[:12])
    args = parser.parse_args()
    if args.count <= 0 or (args.unique is not None and args.unique <= 0):
        parser.error("counts must be positive")
    if args.unique is not None and args.unique > args.count:
        parser.error("unique events cannot exceed delivery count")
    secret = os.environ["PULSEGATE_HMAC_SECRET"]
    out = external_directory(args.output)
    unique = args.unique or args.count
    path = out / "targets.jsonl"
    with path.open("w", newline="\n") as output:
        for delivery in range(args.count):
            index = delivery % unique
            event = {
                "event_id": f"{args.prefix}-{index}",
                "amount_cents": 2500 + index % 800000,
                "currency": "USD",
                "merchant_category": 5812,
                "country": "US",
                "card_present": bool(index % 2),
                "hour_utc": index % 24,
                "velocity_5m": index % 12,
                "velocity_1h": index % 35,
                "prior_declines": index % 6,
            }
            body = encode_event(event)
            target = {
                "method": "POST",
                "url": args.url,
                "body": base64.b64encode(body).decode(),
                "header": {key: [value] for key, value in signed_headers(secret, body).items()},
            }
            output.write(json.dumps(target, separators=(",", ":")) + "\n")
    (out / "target-manifest.json").write_text(
        json.dumps(
            {
                **provenance(),
                "command": sys.argv,
                "count": args.count,
                "unique": unique,
                "prefix": args.prefix,
                "sha256": sha256(path),
            },
            indent=2,
        )
        + "\n"
    )
    print(path)


if __name__ == "__main__":
    main()
