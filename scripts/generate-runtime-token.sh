#!/usr/bin/env bash
set -euo pipefail

# Runtime Token 生成脚本
# 生成 bcrypt hash 并插入到数据库

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

log_success() {
    echo -e "${BLUE}[SUCCESS]${NC} $1"
}

# 检查参数
if [ $# -lt 1 ]; then
    log_error "Usage: $0 <node-id> [token] [ttl]"
    log_info "Example: $0 node-001"
    log_info "Example: $0 node-001 my-custom-token"
    log_info "Example: $0 node-001 my-custom-token '30 days'"
    exit 1
fi

NODE_ID="$1"
TOKEN="${2:-$(openssl rand -hex 32)}"
TTL="${3:-30 days}"

# 检查环境变量
if [ -z "${DATABASE_URL:-}" ]; then
    log_error "DATABASE_URL environment variable is not set"
    log_info "Example: export DATABASE_URL='postgres://user:pass@host:port/dbname?sslmode=disable'"
    exit 1
fi

# 检查 psql 是否安装
if ! command -v psql &> /dev/null; then
    log_error "psql is not installed"
    log_info "Install: brew install postgresql"
    exit 1
fi

log_info "Generating runtime token for node: $NODE_ID"

# 使用 Go 生成 bcrypt hash
cd "$PROJECT_ROOT/apps/control-plane"

# 创建临时 Go 程序生成 bcrypt hash
TEMP_GO=$(mktemp /tmp/bcrypt.XXXXXX.go)
cat > "$TEMP_GO" <<'EOF'
package main

import (
    "fmt"
    "os"
    "golang.org/x/crypto/bcrypt"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "Usage: bcrypt <password>")
        os.Exit(1)
    }
    password := os.Args[1]
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        os.Exit(1)
    }
    fmt.Print(string(hash))
}
EOF

# 生成 hash
HASH=$(go run "$TEMP_GO" "$TOKEN")
rm "$TEMP_GO"

if [ -z "$HASH" ]; then
    log_error "Failed to generate bcrypt hash"
    exit 1
fi

log_info "Generated bcrypt hash"

# 插入到数据库
SQL="INSERT INTO auth_runtime_tokens (node_id, token_hash, expires_at)
     VALUES (:'node_id', :'token_hash', NOW() + :'ttl'::interval)
     ON CONFLICT (node_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at;"

if psql "$DATABASE_URL" -v node_id="$NODE_ID" -v token_hash="$HASH" -v ttl="$TTL" > /dev/null 2>&1 <<SQL
$SQL
SQL
then
    log_success "Token saved to database"
    echo ""
    echo "=========================================="
    echo "Node ID:   $NODE_ID"
    echo "Token:     $TOKEN"
    echo "TTL:       $TTL"
    echo "=========================================="
    echo ""
    log_warn "Save this token securely. It will not be shown again."
    echo ""
    log_info "Start Runtime Agent with:"
    echo "  RUNTIME_AGENT_NODE_ID=$NODE_ID RUNTIME_AGENT_AUTH_TOKEN=$TOKEN cargo run --manifest-path apps/runtime-agent/Cargo.toml"
else
    log_error "Failed to insert token into database"
    exit 1
fi
