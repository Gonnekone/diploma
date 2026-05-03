# Builder: Debian (glibc) — нужен для CGO, opus и onnxruntime
FROM golang:1.25-bookworm AS builder
WORKDIR /src

ARG TARGETARCH

# Зависимости для сборки
# libopusfile-dev нужен для github.com/hraban/opus
RUN apt-get update && apt-get install -y --no-install-recommends \
    libopus-dev \
    libopusfile-dev \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Скачиваем onnxruntime под нужную архитектуру
RUN set -eux; \
    if [ "$TARGETARCH" = "arm64" ]; then ARCH="aarch64"; else ARCH="x64"; fi; \
    ORT_DIR="onnxruntime-linux-${ARCH}-1.19.2"; \
    wget -q "https://github.com/microsoft/onnxruntime/releases/download/v1.19.2/${ORT_DIR}.tgz" -O ort.tgz; \
    tar xzf ort.tgz; \
    cp ${ORT_DIR}/lib/libonnxruntime.so* /usr/lib/; \
    ldconfig; \
    rm -rf ort.tgz ${ORT_DIR}

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /out/app ./cmd

# Runtime: slim Debian
FROM debian:bookworm-slim AS runtime

# libopusfile0 — runtime-зависимость для opus
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    libopus0 \
    libopusfile0 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/lib/libonnxruntime.so* /usr/lib/
RUN ldconfig

RUN groupadd -r app && useradd -r -g app app

WORKDIR /src

COPY --from=builder /out/app    /bin/app
COPY --from=builder /src/views  /src/views
COPY --from=builder /src/assets /src/assets

RUN mkdir -p /src/noisecanceletionmodel/modeldata
COPY noisecanceletionmodel/modeldata/noise_suppressor.onnx \
     /src/noisecanceletionmodel/modeldata/noise_suppressor.onnx

EXPOSE 8080
USER app
ENTRYPOINT ["/bin/app"]