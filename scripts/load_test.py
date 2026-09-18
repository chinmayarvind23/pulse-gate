"""Run a bounded HTTP diagnostic load; use an external driver for release experiments."""

import argparse
import asyncio
import json
import os
import time
import uuid

import httpx

from scripts.common import encode_event, signed_headers
from scripts.evidence import external_directory, provenance


async def run(url: str, secret: str, rps: int, seconds: int) -> dict:
    """Keep in-flight tasks bounded and report scheduler drops instead of hiding them."""
    latencies, codes = [], {}
    active = set()
    dropped = 0
    prefix = uuid.uuid4().hex
    async with httpx.AsyncClient(timeout=2, limits=httpx.Limits(max_connections=256)) as client:

        async def one(index):
            """Measure one full acknowledgement attempt, including transport failure."""
            event = {
                "event_id": f"load-{prefix}-{index}",
                "amount_cents": 2500,
                "currency": "USD",
                "merchant_category": 5812,
                "country": "US",
                "card_present": True,
                "hour_utc": 12,
                "velocity_5m": 1,
                "velocity_1h": 2,
                "prior_declines": 0,
            }
            body = encode_event(event)
            started = time.perf_counter()
            try:
                response = await client.post(
                    url, content=body, headers=signed_headers(secret, body)
                )
                code = response.status_code
            except httpx.HTTPError:
                code = 0
            latencies.append((time.perf_counter() - started) * 1000)
            codes[code] = codes.get(code, 0) + 1

        started = time.perf_counter()
        for index in range(rps * seconds):
            delay = started + index / rps - time.perf_counter()
            if delay > 0:
                await asyncio.sleep(delay)
            if len(active) >= 512:
                dropped += 1
                continue
            task = asyncio.create_task(one(index))
            active.add(task)
            task.add_done_callback(active.discard)
        await asyncio.gather(*active)
        elapsed = time.perf_counter() - started
    ordered = sorted(latencies)
    return {
        "requests": len(latencies),
        "scheduler_drops": dropped,
        "elapsed_s": elapsed,
        "observed_rps": len(latencies) / elapsed,
        "p99_ms": ordered[min(len(ordered) - 1, int(len(ordered) * 0.99))] if ordered else None,
        "codes": codes,
        "latencies_ms": latencies,
    }


def main():
    """Require an external output directory and a finite diagnostic request budget."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8080/v1/events")
    parser.add_argument("--rps", type=int, default=1000)
    parser.add_argument("--seconds", type=int, default=10)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.rps <= 0 or args.seconds <= 0 or args.rps * args.seconds > 1000000:
        parser.error("use positive rate/duration with at most 1000000 attempts")
    out = external_directory(args.output)
    result = asyncio.run(run(args.url, os.environ["HOOKGUARD_HMAC_SECRET"], args.rps, args.seconds))
    (out / "load.json").write_text(json.dumps({**provenance(), **result}) + "\n")
    print(out / "load.json")


if __name__ == "__main__":
    main()
