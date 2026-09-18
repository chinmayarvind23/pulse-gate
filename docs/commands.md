# Commands

```bash
cp .env.example .env
docker compose up --build -d
python -m venv .venv
. .venv/bin/activate
pip install -e '.[dev]'
python scripts/generate_dataset.py
python scripts/smoke_test.py
python evals/risk_eval.py --worker-url http://localhost:8081
python scripts/load_test.py --rps 1000 --seconds 10
```

Language checks:

```bash
go test ./apps/gateway/...
cd apps/risk-worker && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
pytest -q
ruff check scripts evals tests
```

For final performance evidence use a dedicated load tool such as k6 or vegeta from a separate process or host.
