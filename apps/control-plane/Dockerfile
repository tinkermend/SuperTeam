# SuperTeam Control Plane — production image
#
# Build from monorepo root (recommended):
#   docker build -f apps/control-plane/Dockerfile -t superteam-control-plane:local .
#
# Run (mount config; migrations live at /migrations for atlas operators):
#   docker run --rm -p 8080:8080 \
#     -v $PWD/apps/control-plane/config/config.yaml:/etc/superteam/config.yaml:ro \
#     superteam-control-plane:local
#
# See docs/ops/docker-images.md

# syntax=docker/dockerfile:1.6

FROM golang:1.25-bookworm AS build

WORKDIR /src

# Module download layer (cache-friendly)
COPY apps/control-plane/go.mod apps/control-plane/go.sum ./
RUN go mod download

COPY apps/control-plane/ ./

# Static binary; linux target matches container runtime (podman/docker).
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/control-plane ./cmd/control-plane

# Minimal runtime with CA certs (TLS to Postgres/S3/Temporal) and wget for healthcheck.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget \
  && adduser -D -u 65532 -g nonroot nonroot \
  && mkdir -p /etc/superteam /migrations \
  && chown -R nonroot:nonroot /etc/superteam

COPY --from=build /out/control-plane /usr/local/bin/control-plane
# Operators run atlas against this tree; CP process itself does not auto-migrate.
COPY --from=build /src/internal/storage/migrations /migrations

USER nonroot:nonroot
EXPOSE 8080

# Config path is required today (see internal/config.ParseConfigPath).
ENTRYPOINT ["/usr/local/bin/control-plane"]
CMD ["--config", "/etc/superteam/config.yaml"]

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1
