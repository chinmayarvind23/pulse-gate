.PHONY: test fmt up down smoke

test:
	go test ./apps/gateway/...
	cd apps/risk-worker && cargo test --locked
	python -m pytest -q -p no:cacheprovider

fmt:
	gofmt -w apps/gateway
	cd apps/risk-worker && cargo fmt
	ruff format scripts evals tests

up:
	docker compose up --build -d

down:
	docker compose down

smoke:
	python -m scripts.smoke_test
