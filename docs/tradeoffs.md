# Engineering tradeoffs

## Go versus FastAPI for ingress

FastAPI could implement the contract quickly, but this project is specifically proving high-concurrency service behavior and systems reasoning. Go makes the hot path and concurrency model central rather than incidental.

## Rust versus Go for the worker

Go could score this model. Rust is retained because the worker is a clean independently scalable compute boundary and gives the project a typed native inference path. If team ownership, profiling or deployment cost did not justify the split, consolidating to Go would be simpler operationally.

## Redis Streams versus Kafka

Redis makes the MVP small and gives shared idempotency plus async delivery with one dependency. Kafka wins when retention, partitioned throughput, replay history and independent consumer ecosystems dominate. Redis is a conscious scope choice, not a claim that it is the universal payments bus.

## Logistic regression versus a larger model

The simple model is explainable and cheap enough to reason about from math to serving. A larger model only earns its place if offline and online evidence shows a useful quality gain under the latency and false-positive budget.

## Kubernetes versus Compose only

Compose is enough to run the product. Kubernetes exists to demonstrate replica correctness, probes, resource isolation and failure recovery because those behaviors matter to the role signal. The project does not build a platform around Kubernetes.
