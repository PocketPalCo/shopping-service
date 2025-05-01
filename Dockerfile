FROM golang:1.24 as builder

WORKDIR /app
COPY . .
COPY pkg/config/config.sample.yaml ./pkg/config/config.yaml
RUN go mod download &&  go build -o ./shopping-service ./cmd

# health check for grpc
RUN GRPC_HEALTH_PROBE_VERSION=v0.4.4 && \
    wget -qO/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /grpc_health_probe

FROM golang:1.24
COPY --from=builder /app/shopping-service /app/shopping-service
COPY --from=builder /app/pkg/config/config.yaml /app/pkg/config/config.yaml
COPY --from=builder /grpc_health_probe /bin/grpc_health_probe
WORKDIR /app

ENTRYPOINT ["./shopping-service"]
