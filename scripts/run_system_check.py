"""Run a reproducible HTTP experiment with native container files and external artifacts."""

import argparse
import json
import os
import shlex
import subprocess
import sys
import time
import uuid
from pathlib import Path

from scripts.evidence import external_directory, provenance, sha256


def docker_json(*args):
    """Read structured Docker metadata without recording runtime secret environments."""
    return json.loads(subprocess.check_output(["docker", *args], text=True))


def main():
    """Capture the complete offered load, raw outcomes, runtime images and optional restart."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True)
    parser.add_argument("--vegeta", type=Path, required=True, help="Linux amd64 Vegeta executable")
    parser.add_argument("--network", default="pulse_gate_default")
    parser.add_argument("--url", default="http://gateway:8080/v1/events")
    parser.add_argument("--rate", type=int, default=12500)
    parser.add_argument("--seconds", type=int, default=60)
    parser.add_argument("--count", type=int)
    parser.add_argument("--unique", type=int)
    parser.add_argument("--restart-worker", action="store_true")
    parser.add_argument("--prefix", default="run-" + uuid.uuid4().hex[:12])
    args = parser.parse_args()
    count = args.count if args.count is not None else args.rate * max(1, args.seconds) + 1000
    if args.rate <= 0 or args.seconds < 0 or count <= 0:
        parser.error("rate/count must be positive and duration non-negative")
    if args.unique is not None and not 0 < args.unique <= count:
        parser.error("unique events must be positive and cannot exceed count")
    out = external_directory(args.output)
    executable = args.vegeta.resolve(strict=True)
    if not os.environ.get("PULSEGATE_HMAC_SECRET"):
        parser.error("PULSEGATE_HMAC_SECRET is required")
    targets = [
        sys.executable,
        "-m",
        "scripts.make_targets",
        "--output",
        str(out),
        "--url",
        args.url,
        "--count",
        str(count),
        "--prefix",
        args.prefix,
    ]
    if args.unique:
        targets += ["--unique", str(args.unique)]
    subprocess.run(targets, check=True)
    name = "pulsegate-check-" + uuid.uuid4().hex[:10]
    image = "mirror.gcr.io/library/rust:1.90-slim-bookworm"
    attack = [
        "/usr/local/bin/vegeta",
        "attack",
        f"-rate={args.rate}/s",
        f"-duration={args.seconds}s",
        "-workers=128",
        "-max-workers=512",
        "-timeout=2s",
        "-max-body=0",
        "-lazy",
        "-format=json",
        "-targets=/tmp/targets.jsonl",
        "-output=/tmp/results.bin",
    ]
    shell_script = (
        "set -eu\ncp /evidence/targets.jsonl /tmp/targets.jsonl\n"
        + shlex.join(attack)
        + "\n/usr/local/bin/vegeta report -type=json /tmp/results.bin > /tmp/report.json\ncp /tmp/results.bin /evidence/results.bin\ncp /tmp/report.json /evidence/report.json\n"
    )
    command = [
        "docker",
        "run",
        "--name",
        name,
        "--network",
        args.network,
        "-v",
        f"{executable}:/usr/local/bin/vegeta:ro",
        "-v",
        f"{out}:/evidence",
        image,
        "sh",
        "-c",
        shell_script,
    ]
    info = docker_json("info", "--format", "{{json .}}")
    manifest = {
        **provenance(),
        "command": sys.argv,
        "docker_command": command,
        "attack_command": attack,
        "driver_sha256": sha256(executable),
        "profile": vars(args) | {"vegeta": str(executable)},
        "docker": {
            key: info.get(key)
            for key in [
                "ServerVersion",
                "NCPU",
                "MemTotal",
                "KernelVersion",
                "OperatingSystem",
                "Architecture",
            ]
        },
        "images": {
            image: docker_json("image", "inspect", image)[0]["Id"]
            for image in [
                "pulsegate/gateway:local",
                "pulsegate/risk-worker:local",
                "redis:7.4-alpine",
            ]
        },
        "restart": None,
    }
    started = time.monotonic()
    with (out / "driver.log").open("w") as log:
        process = subprocess.Popen(command, stdout=log, stderr=subprocess.STDOUT)
        while process.poll() is None:
            elapsed = time.monotonic() - started
            if args.restart_worker and manifest["restart"] is None and elapsed >= 30:
                before = time.monotonic()
                restart = subprocess.run(
                    ["docker", "restart", "pulse_gate-risk-worker-1"],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                manifest["restart"] = {
                    "elapsed_since_driver_start": elapsed,
                    "timestamp_epoch": time.time(),
                    "duration_seconds": time.monotonic() - before,
                    "exit_code": restart.returncode,
                }
            time.sleep(1)
    manifest["exit_code"] = process.returncode
    manifest["elapsed_seconds"] = time.monotonic() - started
    if (out / "results.bin").exists():
        manifest["raw_sha256"] = sha256(out / "results.bin")
    (out / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    if process.returncode:
        raise RuntimeError(f"driver exited {process.returncode}; inspect {out / 'driver.log'}")
    print((out / "report.json").read_text())


if __name__ == "__main__":
    main()
