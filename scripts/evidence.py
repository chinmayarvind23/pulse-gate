"""Shared artifact-path and provenance rules for local validation tools."""

import hashlib
import os
import platform
import subprocess
from datetime import UTC, datetime
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]


def external_directory(path: str) -> Path:
    """Prevent measured outputs from entering the source repository."""
    destination = Path(path).expanduser().resolve()
    if destination == REPO or destination.is_relative_to(REPO):
        raise ValueError("Output directory must be outside the source repository")
    destination.mkdir(parents=True, exist_ok=True)
    return destination


def sha256(path: Path) -> str:
    """Bind evidence to exact dataset and model bytes."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def provenance() -> dict:
    """Capture source identity and host context without exposing process secrets."""
    return {
        "timestamp": datetime.now(UTC).isoformat(),
        "git_sha": subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=REPO, text=True
        ).strip(),
        "git_dirty": bool(
            subprocess.check_output(["git", "status", "--porcelain"], cwd=REPO, text=True).strip()
        ),
        "machine": {
            "platform": platform.platform(),
            "processor": platform.processor(),
            "cpus": os.cpu_count(),
            "python": platform.python_version(),
        },
    }
