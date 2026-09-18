import hashlib
import hmac
import json


def signed_headers(secret: str, payload: bytes) -> dict[str, str]:
    """Return the exact HMAC contract used by the Go gateway."""
    signature = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return {"Content-Type": "application/json", "X-HookGuard-Signature": signature}


def encode_event(event: dict) -> bytes:
    """Use compact deterministic JSON so signature generation and replay are reproducible."""
    return json.dumps(event, separators=(",", ":"), sort_keys=True).encode()
