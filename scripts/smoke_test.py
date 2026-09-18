import os

import httpx

from scripts.common import encode_event, signed_headers

SECRET = os.getenv("PULSEGATE_HMAC_SECRET", "dev-secret-change-me")
URL = os.getenv("PULSEGATE_URL", "http://localhost:8080/v1/events")


def main() -> int:
    """Check provider acknowledgement and replay suppression through the real endpoint."""
    event = {
        "event_id": "smoke-1",
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
    payload = encode_event(event)
    with httpx.Client(timeout=2.0) as client:
        first = client.post(URL, content=payload, headers=signed_headers(SECRET, payload))
        second = client.post(URL, content=payload, headers=signed_headers(SECRET, payload))
    print(
        {
            "first": first.status_code,
            "second": second.status_code,
            "duplicate_header": second.headers.get("X-PulseGate-Duplicate"),
        }
    )
    return (
        0
        if first.status_code == 202
        and second.status_code == 202
        and second.headers.get("X-PulseGate-Duplicate") == "true"
        else 1
    )


if __name__ == "__main__":
    raise SystemExit(main())
