#!/usr/bin/env bash
# 读取 Control Plane 配置（yaml + S3_* 覆盖），幂等创建 objectStore.bucket 并写入控制台 CORS。
# 控制面进程启动不会自动建桶；新环境在起 CP 前后都应跑一次。
#
#   ./scripts/ops/init-object-store.sh
#   ./scripts/ops/init-object-store.sh --check
#   ./scripts/ops/init-object-store.sh --skip-cors
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="${SUPERTEAM_DEV_CONTROL_PLANE_CONFIG:-$PROJECT_ROOT/apps/control-plane/config/config.yaml}"

cd "$PROJECT_ROOT"
exec go run ./apps/control-plane/cmd/object-store-init --config "$CONFIG" "$@"
