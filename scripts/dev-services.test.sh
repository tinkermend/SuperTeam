#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCRIPT="$PROJECT_ROOT/scripts/dev-services.sh"
TMP_DIR="$(mktemp -d)"

cleanup() {
    if [ -x "$SCRIPT" ]; then
        SUPERTEAM_DEV_PID_DIR="$TMP_DIR/pids" \
        SUPERTEAM_DEV_LOG_DIR="$TMP_DIR/logs" \
        bash "$SCRIPT" stop all >/dev/null 2>&1 || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

export SUPERTEAM_DEV_PID_DIR="$TMP_DIR/pids"
export SUPERTEAM_DEV_LOG_DIR="$TMP_DIR/logs"
export SUPERTEAM_DEV_WAIT_SECONDS=2
export SUPERTEAM_DEV_CONTROL_PLANE_CMD="sleep 60"
export SUPERTEAM_DEV_CONTROL_PLANE_WAIT_URL=""
export SUPERTEAM_DEV_WEB_CMD="sleep 60"
export SUPERTEAM_DEV_WEB_WAIT_URL=""
export SUPERTEAM_DEV_RUNTIME_AGENT_CMD="sleep 60"

run_script() {
    bash "$SCRIPT" "$@"
}

assert_contains() {
    local file="$1"
    local expected="$2"
    if ! grep -Fq "$expected" "$file"; then
        echo "expected $file to contain: $expected" >&2
        echo "actual:" >&2
        cat "$file" >&2
        exit 1
    fi
}

assert_pid_running() {
    local service="$1"
    local pid_file="$SUPERTEAM_DEV_PID_DIR/$service.pid"
    if [ ! -f "$pid_file" ]; then
        echo "missing pid file for $service" >&2
        exit 1
    fi
    local pid
    pid="$(cat "$pid_file")"
    if ! kill -0 "$pid" >/dev/null 2>&1; then
        echo "expected $service pid $pid to be running" >&2
        exit 1
    fi
}

run_script start all >"$TMP_DIR/start.out"
assert_pid_running control-plane
assert_pid_running web
assert_pid_running runtime-agent

run_script status all >"$TMP_DIR/status-running.out"
assert_contains "$TMP_DIR/status-running.out" "control-plane: running"
assert_contains "$TMP_DIR/status-running.out" "web: running"
assert_contains "$TMP_DIR/status-running.out" "runtime-agent: running"

old_web_pid="$(cat "$SUPERTEAM_DEV_PID_DIR/web.pid")"
run_script restart web >"$TMP_DIR/restart-web.out"
new_web_pid="$(cat "$SUPERTEAM_DEV_PID_DIR/web.pid")"
if [ "$old_web_pid" = "$new_web_pid" ]; then
    echo "expected restart to replace web pid" >&2
    exit 1
fi
if kill -0 "$old_web_pid" >/dev/null 2>&1; then
    echo "expected old web pid $old_web_pid to be stopped" >&2
    exit 1
fi
assert_pid_running web

run_script stop all >"$TMP_DIR/stop.out"
run_script status all >"$TMP_DIR/status-stopped.out"
assert_contains "$TMP_DIR/status-stopped.out" "control-plane: stopped"
assert_contains "$TMP_DIR/status-stopped.out" "web: stopped"
assert_contains "$TMP_DIR/status-stopped.out" "runtime-agent: stopped"
