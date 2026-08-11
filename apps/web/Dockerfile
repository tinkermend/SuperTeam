# SuperTeam Web Console — production static image (nginx)
#
# Build from monorepo root:
#   docker build -f apps/web/Dockerfile \
#     --build-arg VITE_CONTROL_PLANE_URL=http://127.0.0.1:8080 \
#     -t superteam-web:local .
#
# Run:
#   docker run --rm -p 8088:80 superteam-web:local
#
# Browser will call VITE_CONTROL_PLANE_URL baked at build time (or hostname:8080
# fallback if left empty — see apps/web/src/lib/config/control-plane-url.ts).
# Prefer an explicit URL or a later reverse-proxy same-origin setup.
#
# See docs/ops/docker-images.md

# syntax=docker/dockerfile:1.6

FROM node:24-bookworm AS build

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.32.1 --activate

# Workspace manifests first for install cache
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY tsconfig.base.json ./
COPY apps/web/package.json apps/web/package.json
# pnpm-workspace globs apps/* — provide sibling package.json so lockfile resolves
COPY apps/desktop/package.json apps/desktop/package.json

# Contracts path is required by vite alias @human-task-kind-labels
COPY contracts/control-plane/human-task-kind-labels.json \
  contracts/control-plane/human-task-kind-labels.json

COPY apps/web apps/web

# Install web graph only (devDependencies still needed for vite/tsc build)
RUN corepack pnpm install --filter @superteam/web... --frozen-lockfile

# Empty string = runtime resolve via location.hostname:8080 (dev-like default).
ARG VITE_CONTROL_PLANE_URL=
ENV VITE_CONTROL_PLANE_URL=${VITE_CONTROL_PLANE_URL}

RUN corepack pnpm --filter @superteam/web build

FROM nginx:1.27-alpine

COPY deploy/docker/web-nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/apps/web/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
