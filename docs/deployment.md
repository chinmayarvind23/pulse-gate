# Deployment

## Docker Compose

`docker compose up --build` starts Redis, gateway, risk worker and Prometheus. Compose is the reproducible local integration environment.

## Kubernetes

The manifests under `infra/k8s/` demonstrate:

- multiple gateway and worker replicas
- readiness and liveness probes
- resource requests and limits
- service discovery
- CPU-based HPA configuration

For a local cluster, use kind or k3d and load locally built images.

## Production differences

- managed Redis with TLS, authentication and high availability
- external load balancer and provider IP/authentication controls where supported
- secret manager integration
- pod disruption budgets
- network policies
- queue-lag custom metric for worker autoscaling
- pending-message reclamation and DLQ
- multi-AZ failure testing
