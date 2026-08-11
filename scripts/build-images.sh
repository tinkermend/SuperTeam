#!/usr/bin/env bash
# Build SuperTeam production container images (control-plane + web).
# Uses docker if available, otherwise podman (this machine aliases docker→podman).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

CLI="${SUPERTEAM_CONTAINER_CLI:-}"
if [ -z "$CLI" ]; then
  if command -v docker >/dev/null 2>&1; then
    CLI=docker
  elif command -v podman >/dev/null 2>&1; then
    CLI=podman
  else
    echo "neither docker nor podman found" >&2
    exit 1
  fi
fi

TAG="${SUPERTEAM_IMAGE_TAG:-local}"
CP_IMAGE="${SUPERTEAM_CP_IMAGE:-superteam-control-plane:${TAG}}"
WEB_IMAGE="${SUPERTEAM_WEB_IMAGE:-superteam-web:${TAG}}"
# Bake browser API base URL into the static build (empty = hostname:8080 fallback).
VITE_CP_URL="${VITE_CONTROL_PLANE_URL:-http://127.0.0.1:8080}"

echo "[build-images] cli=$CLI tag=$TAG"
echo "[build-images] control-plane → $CP_IMAGE"
"$CLI" build \
  -f apps/control-plane/Dockerfile \
  -t "$CP_IMAGE" \
  .

echo "[build-images] web → $WEB_IMAGE (VITE_CONTROL_PLANE_URL=$VITE_CP_URL)"
"$CLI" build \
  -f apps/web/Dockerfile \
  --build-arg "VITE_CONTROL_PLANE_URL=$VITE_CP_URL" \
  -t "$WEB_IMAGE" \
  .

echo "[build-images] done"
echo "  $CP_IMAGE"
echo "  $WEB_IMAGE"
echo "See docs/ops/docker-images.md for run examples."
