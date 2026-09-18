from scripts.common import encode_event, signed_headers


def test_encoding_is_deterministic():
    """Reordered fields must produce the same signed payload bytes."""
    a = encode_event({"b": 2, "a": 1})
    b = encode_event({"a": 1, "b": 2})
    assert a == b


def test_signature_header_present():
    """The client must send a complete SHA-256 signature."""
    assert len(signed_headers("secret", b"payload")["X-PulseGate-Signature"]) == 64
