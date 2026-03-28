FROM --platform=$BUILDPLATFORM golang:1.16.7-alpine3.14 AS builder
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/app ./cmd

FROM --platform=$TARGETPLATFORM alpine:3.19 AS runtime
RUN apk --no-cache add ca-certificates tzdata

RUN addgroup -S app && adduser -S -G app app

WORKDIR /src

COPY --from=builder /out/app /bin/app
COPY --from=builder /src/views /src/views
COPY --from=builder /src/assets /src/assets

EXPOSE 8080
USER app
ENTRYPOINT ["/bin/app"]