# Operations

## Docker Compose

Copy `.env.example` to `.env` and replace the placeholders. Run `docker compose up --build -d`. Compose starts Redis, gateway, worker, Prometheus and Grafana. Redis uses a named volume and append-only persistence. `docker compose down` preserves the volume; deleting the volume deletes local state.

| Local endpoint | Purpose |
| --- | --- |
| http://localhost:8080 | Operator page and webhook API |
| http://localhost:8081 | Worker diagnostics |
| http://localhost:9090 | Prometheus |
| http://localhost:3000 | Grafana |
| localhost:16380 | Redis for local inspection |

Docker binds these ports to loopback. Grafana's anonymous role is read-only. Do not expose this local configuration directly to an untrusted network.

```sh
docker compose ps
docker compose logs --tail=100 gateway risk-worker
docker compose exec redis redis-cli XLEN payment_events
docker compose exec redis redis-cli XPENDING payment_events risk-workers
```

`XLEN` shows outstanding work because completion removes the input entry after acknowledgement. `XPENDING` shows deliveries already assigned to workers. The dashboard includes admission activity, errors, outstanding work and worker recovery counters.

## Configuration

| Variable | Default and purpose |
| --- | --- |
| PULSEGATE_HMAC_SECRET | Required signing secret |
| PULSEGATE_REDIS_ADDR | Redis host and port |
| PULSEGATE_STREAM | `payment_events`, input stream |
| PULSEGATE_RESULT_STREAM | `risk_decisions`, output stream; must differ from input |
| PULSEGATE_IDEMPOTENCY_TTL_SECONDS | `86400`, replay retention |
| PULSEGATE_MAX_BODY_BYTES | `65536`, gateway body limit |
| PULSEGATE_MAX_QUEUE | `1000000`, outstanding-entry limit |
| PULSEGATE_WORKER_GROUP | `risk-workers`, the single consuming workflow |
| PULSEGATE_WORKER_CONSUMER | Unique host/process identity unless explicitly supplied |
| PULSEGATE_WORKER_BATCH | `256`, capped at 1024 |
| PULSEGATE_RECLAIM_IDLE_MS | `5000`, idle interval before reclaiming pending work |

Compose sets service addresses explicitly. Supply additional variables to both gateway and worker when changing stream or retention settings. Do not give replicas the same explicit consumer name.

## Recovery and storage

Admission compares a canonical payload digest with the identifier's retained digest. If the append fails, its new marker is removed. Completion writes a decision, keeps a completion marker, acknowledges and removes input work under Redis isolation. Reclaimed deliveries cannot append a second retained decision. Invalid stream payloads go to the result stream's `:dead` companion.

Transport deadlines and explicit connection replacement allow workers to reconnect after the Redis endpoint changes. A worker shutdown leaves unfinished deliveries pending for another worker. Liveness is separate from readiness so a Redis interruption does not require restarting every application process.

The input stream supports one business consumer group. Completion deletes entries, so a second independent consuming workflow must use its own stream. Decisions have a bounded recent-history window. Idempotency markers expire. Very late replays and deleting Redis data are outside the retained-state guarantee.

AOF synchronizes every second. Host failure can lose recent acknowledged writes even though normal process and pod replacements preserve them. Redis is a single local dependency, not a replicated ledger.

## Kubernetes

Build `pulsegate/gateway:local` and `pulsegate/risk-worker:local` using Compose. Make those images available to your cluster. For a local kind cluster:

```sh
kind create cluster --name pulsegate
kind load docker-image --name pulsegate pulsegate/gateway:local pulsegate/risk-worker:local
kubectl apply -f infra/k8s/namespace.yaml
kubectl -n pulsegate create secret generic pulsegate-secrets --from-literal=hmac-secret="$PULSEGATE_HMAC_SECRET"
kubectl apply -f infra/k8s/redis.yaml -f infra/k8s/gateway.yaml -f infra/k8s/worker.yaml -f infra/k8s/hpa.yaml
kubectl -n pulsegate rollout status deployment/gateway
kubectl -n pulsegate rollout status deployment/risk-worker
kubectl -n pulsegate port-forward service/gateway 8080:80
```

A default dynamic StorageClass is required for the Redis PVC. Redis uses a Recreate strategy because two processes must not write the same append-only directory. The application containers run as non-root with dropped capabilities and read-only root filesystems. Startup, readiness and liveness probes have separate purposes.

The CPU-based HPAs require metrics-server. Confirm resource readings with `kubectl top pods -n pulsegate` and inspect `kubectl get hpa -n pulsegate`. CPU autoscaling is a capacity aid; use queue and pending-work inspection when diagnosing stalled processing.

Apply the observability resources to provision Prometheus pod discovery and the Grafana dashboard:

```sh
kubectl apply -f infra/k8s/observability.json
kubectl -n pulsegate rollout status deployment/prometheus
kubectl -n pulsegate rollout status deployment/grafana
```

Prometheus discovers each gateway and worker replica through a namespace-scoped service account. Run these forwards in separate terminals:

```sh
kubectl -n pulsegate port-forward service/prometheus 29090:9090 --address 127.0.0.1
kubectl -n pulsegate port-forward service/grafana 23000:3000 --address 127.0.0.1
```

Open Prometheus at `http://127.0.0.1:29090` and the dashboard at `http://127.0.0.1:23000/d/pulsegate-operations/pulsegate-operations`. Grafana permits anonymous Viewer access in this local configuration. For a separate cluster configuration, add `--kubeconfig /path/to/kubeconfig` to every kubectl command rather than changing your default context.
