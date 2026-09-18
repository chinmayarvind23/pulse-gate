.PHONY: test fmt up down smoke eval

test:
	go test ./apps/gateway/...
	cd apps/risk-worker && cargo test
	pytest -q

fmt:
	gofmt -w apps/gateway
	cd apps/risk-worker && cargo fmt
	ruff format scripts evals tests

up:
	docker compose up --build -d

down:
	docker compose down -v

smoke:
	python scripts/smoke_test.py

eval:
	python evals/risk_eval.py
