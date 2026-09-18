# Operations

The gateway receives messages, Redis keeps the shared queue and event records, and workers process waiting events. Docker Compose starts these services together. The Kubernetes section provides another way to run them with separate service replicas.

## Docker Compose

Copy `.env.example` to `.env` and replace the placeholders. Run `docker compose up --build -d`. Compose starts Redis, the gateway, the worker, Prometheus and Grafana.

Redis saves its data in a Docker volume and records changes in an append-only file. `docker compose down` stops the services but keeps that data for the next start. Deleting the volume deletes the saved event records and decisions.

| Local endpoint | Purpose |
| --- | --- |
| http://localhost:8080 | Operator page and webhook API |
| http://localhost:8081 | Worker diagnostics |
| http://localhost:9090 | Prometheus |
| http://localhost:3000 | Grafana |
| localhost:16380 | Redis for local inspection |

These ports listen only on your own machine. Grafana allows visitors to view the dashboard without logging in, but they cannot edit it. Keep this local setup off untrusted networks.

Use these commands to check running services, recent logs and waiting work:

```sh
docker compose ps
docker compose logs --tail=100 gateway risk-worker
docker compose exec redis redis-cli XLEN payment_events
docker compose exec redis redis-cli XPENDING payment_events risk-workers
```

`XLEN` counts unfinished events. `XPENDING` counts events that a worker has picked up but has not finished. After processing catches up, both should return zero. During traffic or a restart, they can temporarily increase.

The dashboard shows incoming requests, errors, waiting work and events recovered from stopped workers. If the queue keeps growing, workers are falling behind or cannot finish their work; check their logs and readiness.

The operator page refreshes queue status and recent decisions every two seconds while a signing secret is entered. It shows the last successful refresh time and marks displayed values as potentially out of date after a failure. A request that receives no response stops waiting after five seconds. Use **Retry last** for an event with an uncertain outcome; **Send event** creates a new event ID.

## Configuration

| Variable | Default and purpose |
| --- | --- |
| HOOKGUARD_HMAC_SECRET | Required signing secret |
| HOOKGUARD_REDIS_ADDR | Redis host and port |
| HOOKGUARD_STREAM | `payment_events`, input stream |
| HOOKGUARD_RESULT_STREAM | `risk_decisions`, output stream; must differ from input |
| HOOKGUARD_IDEMPOTENCY_TTL_SECONDS | `86400`, how long Redis remembers IDs to recognize repeats |
| HOOKGUARD_MAX_BODY_BYTES | `65536`, gateway body limit |
| HOOKGUARD_MAX_QUEUE | `1000000`, maximum number of unfinished events |
| HOOKGUARD_WORKER_GROUP | `risk-workers`, the workers sharing this queue |
| HOOKGUARD_WORKER_CONSUMER | Unique host/process identity unless explicitly supplied |
| HOOKGUARD_WORKER_BATCH | `256`, capped at 1024 |
| HOOKGUARD_RECLAIM_IDLE_MS | `5000`, how long assigned work can sit idle before another worker can pick it up |

Compose sets service addresses explicitly. Supply additional variables to both gateway and worker when changing stream or retention settings. Do not give replicas the same explicit consumer name.

## Recovery and storage

For each new event, Redis keeps a record of its ID and a fingerprint of its fields. A retry with the same fields is recognized as a repeat. Different fields under the same retained ID produce a conflict. If queueing fails after a new record is created, that record is removed so the sender can try again.

A worker saves the decision and a completion record, then marks the queued event finished and removes it. Redis runs these steps without another client interrupting them. If the event is picked up again while its completion record is retained, the worker does not publish another decision. Invalid data found in the queue goes to a separate `:dead` stream for inspection.

When a worker stops, its unfinished events remain assigned but incomplete, or *pending*. Another worker can pick them up after the idle interval. Workers also replace failed Redis connections so they can reconnect when Redis moves to a new address.

Health checks separate a running process from one that is ready to work. A Redis interruption can make a service unready without requiring that process to restart.

The queue supports one group of workers sharing the same job. Finished entries are deleted, so a second independent workflow needs its own stream. Only recent decisions are kept, and records used to recognize repeats expire. Very late retries or deleting Redis data can therefore cause an event to be treated as new.

Redis requests a disk sync once per second. A host failure can lose recent accepted events even though ordinary process and pod replacements preserve saved data. This local setup uses one Redis instance and does not provide a replicated financial ledger.

## Kubernetes

Build `hookguard/gateway:local` and `hookguard/risk-worker:local` using Compose. Make those images available to your cluster. For a local kind cluster:

```sh
kind create cluster --name hookguard
kind load docker-image --name hookguard hookguard/gateway:local hookguard/risk-worker:local
kubectl apply -f infra/k8s/namespace.yaml
kubectl -n hookguard create secret generic hookguard-secrets --from-literal=hmac-secret="$HOOKGUARD_HMAC_SECRET"
kubectl apply -f infra/k8s/redis.yaml -f infra/k8s/gateway.yaml -f infra/k8s/worker.yaml -f infra/k8s/hpa.yaml
kubectl -n hookguard rollout status deployment/gateway
kubectl -n hookguard rollout status deployment/risk-worker
kubectl -n hookguard port-forward service/gateway 8080:80
```

A default dynamic StorageClass must be available so Kubernetes can allocate storage for Redis's persistent volume claim (PVC). This lets Redis reuse its saved files after a pod is replaced. Redis uses the Recreate strategy to stop the old process before starting another one on the same files.

Application containers run without root privileges, with reduced permissions and a read-only root filesystem. Startup probes allow time to start, readiness probes decide whether a service can receive work, and liveness probes detect a process that no longer answers.

Horizontal Pod Autoscalers (HPAs) adjust the number of application replicas using CPU readings. They require metrics-server. Confirm the readings with `kubectl top pods -n hookguard` and inspect the policies with `kubectl get hpa -n hookguard`. CPU readings alone do not explain a stuck queue; also check unfinished and pending events.

Apply these resources to let Prometheus find the application pods and load the Grafana dashboard:

```sh
kubectl apply -f infra/k8s/observability.json
kubectl -n hookguard rollout status deployment/prometheus
kubectl -n hookguard rollout status deployment/grafana
```

Prometheus discovers each gateway and worker replica through a namespace-scoped service account. Run these forwards in separate terminals:

```sh
kubectl -n hookguard port-forward service/prometheus 29090:9090 --address 127.0.0.1
kubectl -n hookguard port-forward service/grafana 23000:3000 --address 127.0.0.1
```

Open Prometheus at `http://127.0.0.1:29090` and the dashboard at `http://127.0.0.1:23000/d/hookguard-operations/hookguard-operations`. Grafana permits anonymous Viewer access in this local configuration. For a separate cluster configuration, add `--kubeconfig /path/to/kubeconfig` to every kubectl command rather than changing your default context.
