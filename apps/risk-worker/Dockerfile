FROM rust:1.82-slim AS build
WORKDIR /src
COPY apps/risk-worker/Cargo.toml ./Cargo.toml
COPY apps/risk-worker/src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN useradd --system --uid 10001 pulsegate
COPY --from=build /src/target/release/pulsegate-risk-worker /usr/local/bin/pulsegate-risk-worker
USER 10001
EXPOSE 8081
ENTRYPOINT ["pulsegate-risk-worker"]
