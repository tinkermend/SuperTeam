#!/usr/bin/env bash
set -euo pipefail

# SuperTeam 本地开发服务启停脚本。
# 只停止由本脚本启动并写入 pid 文件的进程，避免误杀用户手动启动的服务。

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

PID_DIR="${SUPERTEAM_DEV_PID_DIR:-$PROJECT_ROOT/.scratch/dev-services/pids}"
LOG_DIR="${SUPERTEAM_DEV_LOG_DIR:-$PROJECT_ROOT/.scratch/dev-services/logs}"
WAIT_SECONDS="${SUPERTEAM_DEV_WAIT_SECONDS:-30}"
STOP_TIMEOUT_SECONDS="${SUPERTEAM_DEV_STOP_TIMEOUT_SECONDS:-10}"

CONTROL_PLANE_CMD="${SUPERTEAM_DEV_CONTROL_PLANE_CMD:-pnpm run dev:control-plane}"
CONTROL_PLANE_WAIT_URL="${SUPERTEAM_DEV_CONTROL_PLANE_WAIT_URL-http://127.0.0.1:8080/health}"
CONTROL_PLANE_DIR="${SUPERTEAM_DEV_CONTROL_PLANE_DIR:-$PROJECT_ROOT/apps/control-plane}"
CONTROL_PLANE_MIGRATIONS_DIR="${SUPERTEAM_DEV_CONTROL_PLANE_MIGRATIONS_DIR:-internal/storage/migrations}"
CONTROL_PLANE_CONFIG="${SUPERTEAM_DEV_CONTROL_PLANE_CONFIG:-$PROJECT_ROOT/apps/control-plane/config/config.yaml}"
# 迁移在 control-plane 启动前自动执行；置 1 可跳过（例如 CI 已单独迁移）。
SKIP_MIGRATIONS="${SUPERTEAM_DEV_SKIP_MIGRATIONS:-0}"
ATLAS_CMD="${SUPERTEAM_DEV_ATLAS_CMD:-atlas}"

TEMPORAL_CMD="${SUPERTEAM_DEV_TEMPORAL_CMD:-temporal server start-dev}"
TEMPORAL_WAIT_URL="${SUPERTEAM_DEV_TEMPORAL_WAIT_URL-http://127.0.0.1:8233/}"

WEB_CMD="${SUPERTEAM_DEV_WEB_CMD:-pnpm run dev:web}"
WEB_WAIT_URL="${SUPERTEAM_DEV_WEB_WAIT_URL-http://127.0.0.1:3000/}"

RUNTIME_AGENT_CMD="${SUPERTEAM_DEV_RUNTIME_AGENT_CMD:-pnpm run dev:runtime-agent}"
RUNTIME_AGENT_WAIT_URL="${SUPERTEAM_DEV_RUNTIME_AGENT_WAIT_URL-}"

# feishu-connector 进默认 all(排在 control-plane 之后);无 HTTP 面,不配 WAIT_URL。
FEISHU_CONNECTOR_CMD="${SUPERTEAM_DEV_FEISHU_CONNECTOR_CMD:-go run ./apps/feishu-connector}"
FEISHU_CONNECTOR_WAIT_URL="${SUPERTEAM_DEV_FEISHU_CONNECTOR_WAIT_URL-}"

# 凭据加密主密钥由 control-plane 配置文件承载(config.yaml security.credentialEncryptionKey,
# 环境变量 CONTROL_PLANE_CREDENTIAL_KEY 可覆盖),脚本不再注入。
# feishu-connector 服务凭据(经 POST /api/v1/admin/service-tokens 签发后存入该文件)。
FEISHU_CONNECTOR_TOKEN_FILE="${SUPERTEAM_DEV_FEISHU_CONNECTOR_TOKEN_FILE:-$PROJECT_ROOT/.scratch/dev-services/feishu-connector.token}"

# 自动装载 connector 服务凭据:环境变量优先;无则读文件;都没有仅告警(bootstrap 会失败)。
ensure_feishu_connector_token() {
    if [ -n "${FEISHU_CONNECTOR_TOKEN:-}" ]; then
        return 0
    fi
    if [ -f "$FEISHU_CONNECTOR_TOKEN_FILE" ]; then
        FEISHU_CONNECTOR_TOKEN="$(cat "$FEISHU_CONNECTOR_TOKEN_FILE")"
        export FEISHU_CONNECTOR_TOKEN
        return 0
    fi
    log_warn "FEISHU_CONNECTOR_TOKEN 未设置且 $FEISHU_CONNECTOR_TOKEN_FILE 不存在;经 POST /api/v1/admin/service-tokens 签发后写入该文件"
}

OPENFGA_COMPOSE_FILE="${SUPERTEAM_DEV_OPENFGA_COMPOSE_FILE:-$PROJECT_ROOT/docker-compose.dev.yml}"
OPENFGA_MODE="${SUPERTEAM_DEV_OPENFGA_MODE:-auto}"
OPENFGA_CMD="${SUPERTEAM_DEV_OPENFGA_CMD:-openfga}"
OPENFGA_DATA_DIR="${SUPERTEAM_DEV_OPENFGA_DATA_DIR:-$PROJECT_ROOT/.scratch/openfga}"
OPENFGA_DATASTORE_ENGINE="${SUPERTEAM_DEV_OPENFGA_DATASTORE_ENGINE:-sqlite}"
OPENFGA_DATASTORE_URI="${SUPERTEAM_DEV_OPENFGA_DATASTORE_URI:-file:$OPENFGA_DATA_DIR/openfga.db}"
OPENFGA_HTTP_ADDR="${SUPERTEAM_DEV_OPENFGA_HTTP_ADDR:-127.0.0.1:8088}"
OPENFGA_GRPC_ADDR="${SUPERTEAM_DEV_OPENFGA_GRPC_ADDR:-127.0.0.1:8089}"
OPENFGA_PLAYGROUND_ENABLED="${SUPERTEAM_DEV_OPENFGA_PLAYGROUND_ENABLED:-true}"
OPENFGA_PLAYGROUND_PORT="${SUPERTEAM_DEV_OPENFGA_PLAYGROUND_PORT:-3008}"
OPENFGA_PLAYGROUND_ADDR="${SUPERTEAM_DEV_OPENFGA_PLAYGROUND_ADDR:-127.0.0.1:$OPENFGA_PLAYGROUND_PORT}"
OPENFGA_WAIT_URL="${SUPERTEAM_DEV_OPENFGA_WAIT_URL-http://127.0.0.1:8088/healthz}"

SERVICES=(temporal control-plane web runtime-agent feishu-connector)
STOP_SERVICES=(feishu-connector runtime-agent web control-plane temporal)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_success() {
    echo -e "${BLUE}[OK]${NC} $1"
}

usage() {
    cat <<'USAGE'
Usage:
  scripts/dev-services.sh <start|stop|restart|status> [all|temporal|control-plane|web|runtime-agent|feishu-connector|openfga]

Examples:
  scripts/dev-services.sh start all
  scripts/dev-services.sh start openfga
  scripts/dev-services.sh status
  scripts/dev-services.sh restart web
  scripts/dev-services.sh stop runtime-agent

Environment overrides:
  SUPERTEAM_DEV_PID_DIR
  SUPERTEAM_DEV_LOG_DIR
  SUPERTEAM_DEV_WAIT_SECONDS
  SUPERTEAM_DEV_TEMPORAL_CMD
  SUPERTEAM_DEV_TEMPORAL_WAIT_URL
  SUPERTEAM_DEV_CONTROL_PLANE_CMD
  SUPERTEAM_DEV_CONTROL_PLANE_WAIT_URL
  SUPERTEAM_DEV_WEB_CMD
  SUPERTEAM_DEV_WEB_WAIT_URL
  SUPERTEAM_DEV_RUNTIME_AGENT_CMD
  SUPERTEAM_DEV_RUNTIME_AGENT_WAIT_URL
  SUPERTEAM_DEV_FEISHU_CONNECTOR_CMD
  SUPERTEAM_DEV_FEISHU_CONNECTOR_WAIT_URL
  SUPERTEAM_DEV_FEISHU_CONNECTOR_TOKEN_FILE
  SUPERTEAM_DEV_OPENFGA_MODE
  SUPERTEAM_DEV_OPENFGA_CMD
  SUPERTEAM_DEV_OPENFGA_COMPOSE_FILE
  SUPERTEAM_DEV_OPENFGA_DATA_DIR
  SUPERTEAM_DEV_OPENFGA_DATASTORE_ENGINE
  SUPERTEAM_DEV_OPENFGA_DATASTORE_URI
  SUPERTEAM_DEV_OPENFGA_HTTP_ADDR
  SUPERTEAM_DEV_OPENFGA_GRPC_ADDR
  SUPERTEAM_DEV_OPENFGA_PLAYGROUND_ENABLED
  SUPERTEAM_DEV_OPENFGA_PLAYGROUND_ADDR
  SUPERTEAM_DEV_OPENFGA_PLAYGROUND_PORT
  SUPERTEAM_DEV_OPENFGA_WAIT_URL
USAGE
}

ensure_dirs() {
    mkdir -p "$PID_DIR" "$LOG_DIR"
}

is_known_service() {
    case "$1" in
        temporal|control-plane|web|runtime-agent|openfga|feishu-connector) return 0 ;;
        *) return 1 ;;
    esac
}

service_command() {
    case "$1" in
        temporal) printf '%s\n' "$TEMPORAL_CMD" ;;
        control-plane) printf '%s\n' "$CONTROL_PLANE_CMD" ;;
        web) printf '%s\n' "$WEB_CMD" ;;
        runtime-agent) printf '%s\n' "$RUNTIME_AGENT_CMD" ;;
        feishu-connector) printf '%s\n' "$FEISHU_CONNECTOR_CMD" ;;
    esac
}

service_wait_url() {
    case "$1" in
        temporal) printf '%s\n' "$TEMPORAL_WAIT_URL" ;;
        control-plane) printf '%s\n' "$CONTROL_PLANE_WAIT_URL" ;;
        web) printf '%s\n' "$WEB_WAIT_URL" ;;
        runtime-agent) printf '%s\n' "$RUNTIME_AGENT_WAIT_URL" ;;
        openfga) printf '%s\n' "$OPENFGA_WAIT_URL" ;;
        feishu-connector) printf '%s\n' "$FEISHU_CONNECTOR_WAIT_URL" ;;
    esac
}

pid_file() {
    printf '%s/%s.pid\n' "$PID_DIR" "$1"
}

log_file() {
    printf '%s/%s.log\n' "$LOG_DIR" "$1"
}

read_pid() {
    local file
    file="$(pid_file "$1")"
    if [ -f "$file" ]; then
        tr -d '[:space:]' <"$file"
    fi
}

pid_running() {
    local pid="$1"
    [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1
}

http_ok() {
    local url="$1"
    [ -n "$url" ] || return 1
    command -v curl >/dev/null 2>&1 || return 1
    curl -fsS --max-time 2 "$url" >/dev/null 2>&1
}

docker_compose() {
    docker compose -f "$OPENFGA_COMPOSE_FILE" "$@"
}

shell_join() {
    local joined=""
    local quoted
    local arg
    for arg in "$@"; do
        printf -v quoted '%q' "$arg"
        joined="${joined}${joined:+ }$quoted"
    done
    printf '%s\n' "$joined"
}

openfga_local_available() {
    command -v "$OPENFGA_CMD" >/dev/null 2>&1
}

openfga_migrate_command() {
    local openfga_bin="${1:-$OPENFGA_CMD}"
    shell_join \
        "$openfga_bin" migrate \
        --datastore-engine "$OPENFGA_DATASTORE_ENGINE" \
        --datastore-uri "$OPENFGA_DATASTORE_URI"
}

openfga_run_command() {
    local openfga_bin="${1:-$OPENFGA_CMD}"
    local args=(
        "$openfga_bin" run
        --datastore-engine "$OPENFGA_DATASTORE_ENGINE"
        --datastore-uri "$OPENFGA_DATASTORE_URI"
        --http-addr "$OPENFGA_HTTP_ADDR"
        --grpc-addr "$OPENFGA_GRPC_ADDR"
    )

    if [ "$OPENFGA_PLAYGROUND_ENABLED" = "true" ]; then
        args+=(--playground-enabled --playground-addr "$OPENFGA_PLAYGROUND_ADDR")
    fi

    shell_join "${args[@]}"
}

tail_openfga_log() {
    if command -v docker >/dev/null 2>&1; then
        docker_compose logs --tail 80 openfga openfga-migrate >&2 || true
    fi
}

tail_service_log() {
    local service="$1"
    local file
    file="$(log_file "$service")"
    if [ -f "$file" ]; then
        echo "---- tail $file ----" >&2
        tail -n 40 "$file" >&2 || true
        echo "--------------------" >&2
    fi
}

child_pids() {
    local pid="$1"
    if command -v pgrep >/dev/null 2>&1; then
        pgrep -P "$pid" 2>/dev/null || true
        return 0
    fi
    ps -axo ppid=,pid= | awk -v parent="$pid" '$1 == parent { print $2 }'
}

kill_tree() {
    local signal="$1"
    local pid="$2"
    local child
    for child in $(child_pids "$pid"); do
        kill_tree "$signal" "$child"
    done
    kill "-$signal" "$pid" >/dev/null 2>&1 || true
}

wait_for_stop() {
    local pid="$1"
    local waited=0
    while pid_running "$pid"; do
        if [ "$waited" -ge "$STOP_TIMEOUT_SECONDS" ]; then
            return 1
        fi
        sleep 1
        waited=$((waited + 1))
    done
    return 0
}

wait_for_start() {
    local service="$1"
    local pid="$2"
    local url
    url="$(service_wait_url "$service")"

    if [ -z "$url" ]; then
        local waited=0
        while [ "$waited" -lt "$WAIT_SECONDS" ]; do
            if ! pid_running "$pid"; then
                log_error "$service exited during startup"
                tail_service_log "$service"
                return 1
            fi
            sleep 1
            waited=$((waited + 1))
        done
        log_success "$service stayed running for ${WAIT_SECONDS}s pid=$pid log=$(log_file "$service")"
        return 0
    fi

    local waited=0
    while [ "$waited" -lt "$WAIT_SECONDS" ]; do
        if ! pid_running "$pid"; then
            log_error "$service exited before becoming healthy"
            tail_service_log "$service"
            return 1
        fi
        if http_ok "$url"; then
            log_success "$service healthy at $url pid=$pid log=$(log_file "$service")"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
    done

    log_error "$service did not become healthy at $url within ${WAIT_SECONDS}s"
    tail_service_log "$service"
    return 1
}

launch_service_process() {
    local cmd="$1"
    local log="$2"

    (
        cd "$PROJECT_ROOT"
        if command -v python3 >/dev/null 2>&1; then
            exec nohup python3 -c 'import os, sys; os.setsid(); os.execvp(sys.argv[1], sys.argv[1:])' bash -lc "$cmd" >>"$log" 2>&1 < /dev/null
        fi
        if command -v perl >/dev/null 2>&1; then
            exec nohup perl -MPOSIX=setsid -e 'setsid() or die "setsid failed: $!"; exec @ARGV' bash -lc "$cmd" >>"$log" 2>&1 < /dev/null
        fi
        exec nohup bash -lc "$cmd" >>"$log" 2>&1 < /dev/null
    ) &
    printf '%s\n' "$!"
}

# 解析 control-plane 实际连接的数据库 URL。
# 优先级：SUPERTEAM_DEV_DATABASE_URL > DATABASE_URL > config.yaml 的 postgres.url。
resolve_control_plane_db_url() {
    if [ -n "${SUPERTEAM_DEV_DATABASE_URL:-}" ]; then
        printf '%s\n' "$SUPERTEAM_DEV_DATABASE_URL"
        return 0
    fi
    if [ -n "${DATABASE_URL:-}" ]; then
        printf '%s\n' "$DATABASE_URL"
        return 0
    fi
    if [ -f "$CONTROL_PLANE_CONFIG" ]; then
        awk '
            /^[A-Za-z]/ { section=$1 }
            section=="postgres:" && $1=="url:" {
                line=$0; sub(/^[^"]*"/,"",line); sub(/".*$/,"",line); print line; exit
            }
        ' "$CONTROL_PLANE_CONFIG"
        return 0
    fi
    return 0
}

# 在启动 control-plane 前执行数据库迁移，避免 schema 漂移。
run_control_plane_migrations() {
    if [ "$SKIP_MIGRATIONS" = "1" ]; then
        log_warn "SUPERTEAM_DEV_SKIP_MIGRATIONS=1，跳过 control-plane 数据库迁移"
        return 0
    fi

    if ! command -v "$ATLAS_CMD" >/dev/null 2>&1; then
        log_error "未找到 atlas（$ATLAS_CMD），无法执行迁移；安装 atlas 或设 SUPERTEAM_DEV_SKIP_MIGRATIONS=1 跳过"
        return 1
    fi

    local db_url
    db_url="$(resolve_control_plane_db_url)"
    if [ -z "$db_url" ]; then
        log_error "无法解析数据库 URL（检查 DATABASE_URL 或 $CONTROL_PLANE_CONFIG 的 postgres.url）"
        return 1
    fi

    local log
    log="$(log_file "control-plane")"
    {
        echo ""
        echo "===== $(date '+%Y-%m-%d %H:%M:%S') migrate control-plane ====="
        echo "dir: $CONTROL_PLANE_MIGRATIONS_DIR"
    } >>"$log"

    log_info "applying control-plane migrations (atlas migrate apply)"
    if ! (cd "$CONTROL_PLANE_DIR" && "$ATLAS_CMD" migrate apply \
            --dir "file://$CONTROL_PLANE_MIGRATIONS_DIR" \
            --url "$db_url" \
            --revisions-schema atlas_schema_revisions >>"$log" 2>&1); then
        log_error "control-plane 迁移失败，查看日志: $log"
        tail_service_log "control-plane"
        return 1
    fi
    log_success "control-plane migrations up to date"
    return 0
}

start_service() {
    local service="$1"
    ensure_dirs

    case "$service" in
        feishu-connector) ensure_feishu_connector_token ;;
    esac

    if [ "$service" = "openfga" ]; then
        start_openfga_service
        return 0
    fi

    local pid
    pid="$(read_pid "$service")"
    if pid_running "$pid"; then
        log_info "$service already running pid=$pid"
        return 0
    fi

    local file
    file="$(pid_file "$service")"
    if [ -f "$file" ]; then
        log_warn "$service has stale pid file; removing $file"
        rm -f "$file"
    fi

    local url
    url="$(service_wait_url "$service")"
    if http_ok "$url"; then
        log_warn "$service already responds at $url but is not managed by this script; skipping start"
        return 0
    fi

    if [ "$service" = "control-plane" ]; then
        if ! run_control_plane_migrations; then
            return 1
        fi
    fi

    local cmd
    cmd="$(service_command "$service")"
    local log
    log="$(log_file "$service")"
    {
        echo ""
        echo "===== $(date '+%Y-%m-%d %H:%M:%S') start $service ====="
        echo "cwd: $PROJECT_ROOT"
        echo "cmd: $cmd"
    } >>"$log"

    log_info "starting $service: $cmd"
    pid="$(launch_service_process "$cmd" "$log")"
    echo "$pid" >"$file"
    if ! wait_for_start "$service" "$pid"; then
        rm -f "$file"
        return 1
    fi
}

start_openfga_service() {
    ensure_dirs

    if http_ok "$OPENFGA_WAIT_URL"; then
        log_info "openfga already healthy at $OPENFGA_WAIT_URL"
        return 0
    fi

    case "$OPENFGA_MODE" in
        local)
            start_openfga_local_service
            ;;
        compose)
            start_openfga_compose_service
            ;;
        auto)
            if openfga_local_available; then
                start_openfga_local_service
            elif command -v docker >/dev/null 2>&1; then
                start_openfga_compose_service
            else
                log_error "openfga binary is not available and docker is not available; install OpenFGA or set SUPERTEAM_DEV_OPENFGA_MODE=compose with Docker"
                return 1
            fi
            ;;
        *)
            log_error "unknown SUPERTEAM_DEV_OPENFGA_MODE: $OPENFGA_MODE"
            return 1
            ;;
    esac
}

start_openfga_local_service() {
    if ! openfga_local_available; then
        log_error "openfga command not found: $OPENFGA_CMD"
        return 1
    fi

    local openfga_bin
    openfga_bin="$(command -v "$OPENFGA_CMD")"

    local pid
    pid="$(read_pid openfga)"
    if pid_running "$pid"; then
        log_info "openfga already running pid=$pid"
        return 0
    fi

    local file
    file="$(pid_file openfga)"
    if [ -f "$file" ]; then
        log_warn "openfga has stale pid file; removing $file"
        rm -f "$file"
    fi

    mkdir -p "$OPENFGA_DATA_DIR"

    local migrate_cmd
    local run_cmd
    local log
    migrate_cmd="$(openfga_migrate_command "$openfga_bin")"
    run_cmd="$(openfga_run_command "$openfga_bin")"
    log="$(log_file openfga)"
    {
        echo ""
        echo "===== $(date '+%Y-%m-%d %H:%M:%S') start openfga ====="
        echo "cwd: $PROJECT_ROOT"
        echo "mode: local"
        echo "migrate: $migrate_cmd"
        echo "cmd: $run_cmd"
    } >>"$log"

    log_info "migrating openfga datastore: $OPENFGA_DATASTORE_URI"
    if ! (cd "$PROJECT_ROOT" && bash -lc "$migrate_cmd" >>"$log" 2>&1); then
        log_error "openfga datastore migration failed"
        tail_service_log openfga
        return 1
    fi

    log_info "starting openfga via local CLI: $OPENFGA_CMD"
    pid="$(launch_service_process "$run_cmd" "$log")"
    echo "$pid" >"$file"
    if ! wait_for_start openfga "$pid"; then
        rm -f "$file"
        return 1
    fi
}

start_openfga_compose_service() {
    if ! command -v docker >/dev/null 2>&1; then
        log_error "docker is required to start openfga in compose mode"
        return 1
    fi
    log_info "starting openfga via docker compose: $OPENFGA_COMPOSE_FILE"
    docker_compose up -d openfga

    local waited=0
    while [ "$waited" -lt "$WAIT_SECONDS" ]; do
        if http_ok "$OPENFGA_WAIT_URL"; then
            log_success "openfga healthy at $OPENFGA_WAIT_URL"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
    done

    log_error "openfga did not become healthy at $OPENFGA_WAIT_URL within ${WAIT_SECONDS}s"
    tail_openfga_log
    return 1
}

stop_service() {
    local service="$1"
    ensure_dirs

    if [ "$service" = "openfga" ]; then
        stop_openfga_service
        return 0
    fi

    local pid
    pid="$(read_pid "$service")"
    local file
    file="$(pid_file "$service")"

    if ! pid_running "$pid"; then
        rm -f "$file"
        local url
        url="$(service_wait_url "$service")"
        if http_ok "$url"; then
            log_warn "$service is available at $url but was not started by this script; leaving it running"
        else
            log_info "$service stopped"
        fi
        return 0
    fi

    log_info "stopping $service pid=$pid"
    kill_tree TERM "$pid"
    if ! wait_for_stop "$pid"; then
        log_warn "$service did not stop after ${STOP_TIMEOUT_SECONDS}s; sending SIGKILL"
        kill_tree KILL "$pid"
        wait_for_stop "$pid" || true
    fi
    rm -f "$file"
    log_success "$service stopped"
}

stop_openfga_service() {
    ensure_dirs

    local pid
    pid="$(read_pid openfga)"
    local file
    file="$(pid_file openfga)"

    if pid_running "$pid"; then
        log_info "stopping openfga pid=$pid"
        kill_tree TERM "$pid"
        if ! wait_for_stop "$pid"; then
            log_warn "openfga did not stop after ${STOP_TIMEOUT_SECONDS}s; sending SIGKILL"
            kill_tree KILL "$pid"
            wait_for_stop "$pid" || true
        fi
        rm -f "$file"
        log_success "openfga stopped"
        return 0
    fi

    rm -f "$file"
    if http_ok "$OPENFGA_WAIT_URL"; then
        log_warn "openfga is available at $OPENFGA_WAIT_URL but was not started by this script; leaving it running"
        return 0
    fi

    if [ "$OPENFGA_MODE" != "local" ] && command -v docker >/dev/null 2>&1; then
        if docker_compose ps --status running --services 2>/dev/null | grep -Eq '^(openfga|openfga-migrate)$'; then
            log_info "stopping openfga via docker compose"
            docker_compose stop openfga openfga-migrate >/dev/null 2>&1 || true
            log_success "openfga stopped"
            return 0
        fi
    fi

    log_info "openfga stopped"
}

status_openfga_service() {
    local pid
    pid="$(read_pid openfga)"

    if pid_running "$pid"; then
        if [ -n "$OPENFGA_WAIT_URL" ]; then
            if http_ok "$OPENFGA_WAIT_URL"; then
                echo "openfga: running pid=$pid healthy=$OPENFGA_WAIT_URL log=$(log_file openfga)"
            else
                echo "openfga: running pid=$pid health=pending url=$OPENFGA_WAIT_URL log=$(log_file openfga)"
            fi
        else
            echo "openfga: running pid=$pid log=$(log_file openfga)"
        fi
        return 0
    fi

    if [ -n "$pid" ] && http_ok "$OPENFGA_WAIT_URL"; then
        echo "openfga: running-external stale_pid=$pid healthy=$OPENFGA_WAIT_URL"
        return 0
    fi

    if [ -n "$pid" ]; then
        echo "openfga: stale pid=$pid"
        return 0
    fi

    if http_ok "$OPENFGA_WAIT_URL"; then
        echo "openfga: running-external healthy=$OPENFGA_WAIT_URL"
        return 0
    fi

    if command -v docker >/dev/null 2>&1 && docker_compose ps --status running --services 2>/dev/null | grep -Fxq openfga; then
        echo "openfga: running health=pending url=$OPENFGA_WAIT_URL compose=$OPENFGA_COMPOSE_FILE"
        return 0
    fi
    echo "openfga: stopped"
}

status_service() {
    local service="$1"
    if [ "$service" = "openfga" ]; then
        status_openfga_service
        return 0
    fi

    local pid
    pid="$(read_pid "$service")"
    local url
    url="$(service_wait_url "$service")"

    if pid_running "$pid"; then
        if [ -n "$url" ]; then
            if http_ok "$url"; then
                echo "$service: running pid=$pid healthy=$url log=$(log_file "$service")"
            else
                echo "$service: running pid=$pid health=pending url=$url log=$(log_file "$service")"
            fi
        else
            echo "$service: running pid=$pid log=$(log_file "$service")"
        fi
        return 0
    fi

    if [ -n "$pid" ] && http_ok "$url"; then
        echo "$service: running-external stale_pid=$pid healthy=$url"
        return 0
    fi

    if [ -n "$pid" ]; then
        echo "$service: stale pid=$pid"
        return 0
    fi

    if http_ok "$url"; then
        echo "$service: running-external healthy=$url"
        return 0
    fi

    echo "$service: stopped"
}

services_for_target() {
    local target="$1"
    if [ "$target" = "all" ]; then
        printf '%s\n' "${SERVICES[@]}"
        return 0
    fi
    if is_known_service "$target"; then
        printf '%s\n' "$target"
        return 0
    fi
    log_error "unknown service: $target"
    usage >&2
    return 1
}

stop_services_for_target() {
    local target="$1"
    if [ "$target" = "all" ]; then
        printf '%s\n' "${STOP_SERVICES[@]}"
        return 0
    fi
    services_for_target "$target"
}

run_action() {
    local action="$1"
    local target="$2"
    local service

    case "$action" in
        start|stop|restart|status)
            ;;
        *)
            log_error "unknown action: $action"
            usage >&2
            return 1
            ;;
    esac

    # 先校验目标服务，非法参数直接以非零码失败。
    # 下面的循环用进程替换 < <(services_for_target ...) 读取服务列表，
    # 而进程替换的退出码在 set -e 下不会被观察到——没有这道校验，
    # 拼错的服务名会打印错误却仍以 exit 0 结束，掩盖调用方（CI / restart <service>）的失误。
    if ! services_for_target "$target" >/dev/null; then
        return 1
    fi

    case "$action" in
        start)
            while IFS= read -r service; do
                start_service "$service"
            done < <(services_for_target "$target")
            ;;
        stop)
            while IFS= read -r service; do
                stop_service "$service"
            done < <(stop_services_for_target "$target")
            ;;
        restart)
            while IFS= read -r service; do
                stop_service "$service"
            done < <(stop_services_for_target "$target")
            while IFS= read -r service; do
                start_service "$service"
            done < <(services_for_target "$target")
            ;;
        status)
            while IFS= read -r service; do
                status_service "$service"
            done < <(services_for_target "$target")
            ;;
    esac
}

main() {
    local action="${1:-status}"
    local target="${2:-all}"

    case "$action" in
        -h|--help|help)
            usage
            return 0
            ;;
    esac

    run_action "$action" "$target"
}

main "$@"
