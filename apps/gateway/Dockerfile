FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY apps/gateway ./apps/gateway
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gateway ./apps/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/gateway"]
