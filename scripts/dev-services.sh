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
CONTROL_PLANE_WAIT_URL="${SUPERTEAM_DEV_CONTROL_PLANE_WAIT_URL-http://127.0.0.1:8081/health}"

TEMPORAL_CMD="${SUPERTEAM_DEV_TEMPORAL_CMD:-temporal server start-dev}"
TEMPORAL_WAIT_URL="${SUPERTEAM_DEV_TEMPORAL_WAIT_URL-http://127.0.0.1:8233/}"

WEB_CMD="${SUPERTEAM_DEV_WEB_CMD:-pnpm run dev:web}"
WEB_WAIT_URL="${SUPERTEAM_DEV_WEB_WAIT_URL-http://127.0.0.1:3000/}"

RUNTIME_AGENT_CMD="${SUPERTEAM_DEV_RUNTIME_AGENT_CMD:-pnpm run dev:runtime-agent}"
RUNTIME_AGENT_WAIT_URL="${SUPERTEAM_DEV_RUNTIME_AGENT_WAIT_URL-}"

SERVICES=(temporal control-plane web runtime-agent)
STOP_SERVICES=(runtime-agent web control-plane temporal)

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
  scripts/dev-services.sh <start|stop|restart|status> [all|temporal|control-plane|web|runtime-agent]

Examples:
  scripts/dev-services.sh start all
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
USAGE
}

ensure_dirs() {
    mkdir -p "$PID_DIR" "$LOG_DIR"
}

is_known_service() {
    case "$1" in
        temporal|control-plane|web|runtime-agent) return 0 ;;
        *) return 1 ;;
    esac
}

service_command() {
    case "$1" in
        temporal) printf '%s\n' "$TEMPORAL_CMD" ;;
        control-plane) printf '%s\n' "$CONTROL_PLANE_CMD" ;;
        web) printf '%s\n' "$WEB_CMD" ;;
        runtime-agent) printf '%s\n' "$RUNTIME_AGENT_CMD" ;;
    esac
}

service_wait_url() {
    case "$1" in
        temporal) printf '%s\n' "$TEMPORAL_WAIT_URL" ;;
        control-plane) printf '%s\n' "$CONTROL_PLANE_WAIT_URL" ;;
        web) printf '%s\n' "$WEB_WAIT_URL" ;;
        runtime-agent) printf '%s\n' "$RUNTIME_AGENT_WAIT_URL" ;;
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

start_service() {
    local service="$1"
    ensure_dirs

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

stop_service() {
    local service="$1"
    ensure_dirs

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

status_service() {
    local service="$1"
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
        *)
            log_error "unknown action: $action"
            usage >&2
            return 1
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
