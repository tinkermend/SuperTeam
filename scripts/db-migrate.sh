#!/usr/bin/env bash
set -euo pipefail

# 数据库迁移脚本
# 使用 Atlas 运行数据库迁移

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTROL_PLANE_DIR="$PROJECT_ROOT/apps/control-plane"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查环境变量
if [ -z "${DATABASE_URL:-}" ]; then
    log_error "DATABASE_URL environment variable is not set"
    log_info "Example: export DATABASE_URL='postgres://user:pass@host:port/dbname?sslmode=disable'"
    exit 1
fi

# 检查 Atlas 是否安装
if ! command -v atlas &> /dev/null; then
    log_error "Atlas is not installed"
    log_info "Install: brew install ariga/tap/atlas"
    log_info "Or visit: https://atlasgo.io/getting-started#installation"
    exit 1
fi

log_info "Running database migrations..."
log_info "Database: $DATABASE_URL"

cd "$CONTROL_PLANE_DIR"

# 运行迁移
if atlas migrate apply --env local; then
    log_info "Migration completed successfully"
else
    log_error "Migration failed"
    exit 1
fi

# 验证迁移状态
log_info "Checking migration status..."
atlas migrate status --env local
