# Build stage - use latest Go and install 1.25.1
FROM debian:bullseye AS builder

# Install build dependencies for Azure Speech SDK and Go
RUN apt-get update && apt-get install -y \
    build-essential \
    ca-certificates \
    libasound2-dev \
    libssl-dev \
    wget \
    curl \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.25.1 for ARM64
ENV GO_VERSION=1.25.1
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then \
        GOARCH="arm64"; \
    else \
        GOARCH="amd64"; \
    fi && \
    wget https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-${GOARCH}.tar.gz && \
    rm go${GO_VERSION}.linux-${GOARCH}.tar.gz

# Set Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Download and install Azure Speech SDK C++ library
ENV SPEECHSDK_ROOT="/usr/local/SpeechSDK"
RUN mkdir -p "$SPEECHSDK_ROOT" && \
    ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then \
        echo "ARM64 detected - using x64 Speech SDK as fallback" && \
        wget -O SpeechSDK-Linux.tar.gz https://aka.ms/csspeech/linuxbinary && \
        export LIBDIR="x64"; \
    else \
        wget -O SpeechSDK-Linux.tar.gz https://aka.ms/csspeech/linuxbinary && \
        export LIBDIR="x64"; \
    fi && \
    tar --strip 1 -xzf SpeechSDK-Linux.tar.gz -C "$SPEECHSDK_ROOT" && \
    rm SpeechSDK-Linux.tar.gz && \
    echo "SPEECHSDK_LIBDIR=$LIBDIR" >> /etc/environment

# Set environment variables for Speech SDK
ENV CGO_CFLAGS="-I$SPEECHSDK_ROOT/include/c_api"
ENV CGO_LDFLAGS="-L$SPEECHSDK_ROOT/lib/x64 -lMicrosoft.CognitiveServices.Speech.core"
ENV LD_LIBRARY_PATH="$SPEECHSDK_ROOT/lib/x64:$LD_LIBRARY_PATH"

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for Speech SDK
RUN CGO_ENABLED=1 GOOS=linux go build -o shopping-service ./cmd

# Runtime stage
FROM debian:bullseye-slim

# Install runtime dependencies for Azure Speech SDK
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libasound2 \
    libssl1.1 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Copy Speech SDK libraries from builder (x64 as fallback for ARM64)
COPY --from=builder /usr/local/SpeechSDK/lib/x64 /usr/local/lib/
RUN echo "/usr/local/lib" > /etc/ld.so.conf.d/speechsdk.conf && ldconfig

RUN mkdir /app

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/shopping-service .

# Create logs directory
RUN mkdir -p logs

# Create non-root user
RUN groupadd -g 1001 appgroup && \
    useradd -u 1001 -g appgroup -m -s /bin/bash appuser

# Change ownership
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3009/health || exit 1

# Expose port
EXPOSE 3009

# Run the application
ENTRYPOINT ["./shopping-service"]
