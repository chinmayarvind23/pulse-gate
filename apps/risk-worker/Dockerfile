FROM mirror.gcr.io/library/rust:1.90-slim-bookworm AS build
WORKDIR /src
COPY apps/risk-worker/Cargo.toml apps/risk-worker/Cargo.lock ./
COPY apps/risk-worker/src ./src
COPY apps/risk-worker/model ./model
RUN cargo build --release --locked

FROM debian:bookworm-slim
RUN useradd --system --uid 10001 hookguard
COPY --from=build /src/target/release/hookguard-risk-worker /usr/local/bin/hookguard-risk-worker
USER 10001
EXPOSE 8081
ENTRYPOINT ["hookguard-risk-worker"]
