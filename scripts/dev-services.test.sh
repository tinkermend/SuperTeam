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
        SUPERTEAM_DEV_OPENFGA_WAIT_URL="" \
        bash "$SCRIPT" stop openfga >/dev/null 2>&1 || true
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
export SUPERTEAM_DEV_FEISHU_CONNECTOR_CMD="sleep 60"

# 对象存储就绪检查用桩替掉真 go 命令：脚本测试不该依赖本机 RustFS/凭据/网络，
# 但要能演练"存储不可用 → 起服中止"这条真实故障路径。
FAKE_OS_BIN="$TMP_DIR/osbin"
mkdir -p "$FAKE_OS_BIN"
cat >"$FAKE_OS_BIN/object-store-init" <<'FAKE_OBJECT_STORE'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"$SUPERTEAM_FAKE_OBJECT_STORE_LOG"
echo "endpoint=http://127.0.0.1:9000 bucket=superteam-artifacts region=us-east-1 forcePathStyle=true"
if [ "${FAKE_OBJECT_STORE_DOWN:-0}" = "1" ]; then
    echo 'object-store-init: head bucket "superteam-artifacts": dial tcp 127.0.0.1:9000: connect: connection refused' >&2
    exit 1
fi
echo "bucket: exists"
FAKE_OBJECT_STORE
chmod +x "$FAKE_OS_BIN/object-store-init"
export SUPERTEAM_DEV_OBJECT_STORE_INIT_CMD="$FAKE_OS_BIN/object-store-init"
export SUPERTEAM_FAKE_OBJECT_STORE_LOG="$TMP_DIR/object-store.log"

run_script() {
    bash "$SCRIPT" "$@"
}

assert_contains() {
    local file="$1"
    local expected="$2"
    if ! grep -Fq -- "$expected" "$file"; then
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
assert_pid_running feishu-connector

run_script status all >"$TMP_DIR/status-running.out"
assert_contains "$TMP_DIR/status-running.out" "control-plane: running"
assert_contains "$TMP_DIR/status-running.out" "web: running"
assert_contains "$TMP_DIR/status-running.out" "runtime-agent: running"
assert_contains "$TMP_DIR/status-running.out" "feishu-connector: running"

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
assert_contains "$TMP_DIR/status-stopped.out" "feishu-connector: stopped"

# 起 control-plane 前必须复核对象存储，且只为读（--check）：起服不该顺手改桶状态。
assert_contains "$SUPERTEAM_FAKE_OBJECT_STORE_LOG" "--check --skip-cors"

# 存储不可用时必须中止启动，而不是起一个 /health 恒 503 的 CP。
export FAKE_OBJECT_STORE_DOWN=1
if run_script start control-plane >"$TMP_DIR/start-cp-store-down.out" 2>&1; then
    echo "expected 'start control-plane' to fail while object store is unreachable" >&2
    cat "$TMP_DIR/start-cp-store-down.out" >&2
    exit 1
fi
assert_contains "$TMP_DIR/start-cp-store-down.out" "对象存储未就绪"
if [ -f "$SUPERTEAM_DEV_PID_DIR/control-plane.pid" ]; then
    echo "expected no control-plane pid file after failed object store check" >&2
    exit 1
fi

# 逃生舱：存储确认无误但仍要起服时，跳过检查必须真的放行。
export SUPERTEAM_DEV_SKIP_OBJECT_STORE_CHECK=1
run_script start control-plane >"$TMP_DIR/start-cp-store-skipped.out" 2>&1
assert_pid_running control-plane
assert_contains "$TMP_DIR/start-cp-store-skipped.out" "跳过对象存储就绪检查"
unset SUPERTEAM_DEV_SKIP_OBJECT_STORE_CHECK FAKE_OBJECT_STORE_DOWN
run_script stop control-plane >"$TMP_DIR/stop-cp.out"

FAKE_BIN="$TMP_DIR/bin"
mkdir -p "$FAKE_BIN"
cat >"$FAKE_BIN/openfga" <<'FAKE_OPENFGA'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
    migrate)
        echo "migrate $*" >>"$SUPERTEAM_FAKE_OPENFGA_LOG"
        exit 0
        ;;
    run)
        echo "run $*" >>"$SUPERTEAM_FAKE_OPENFGA_LOG"
        sleep 60
        ;;
    *)
        echo "unexpected openfga command: $*" >&2
        exit 1
        ;;
esac
FAKE_OPENFGA
chmod +x "$FAKE_BIN/openfga"

export PATH="$FAKE_BIN:$PATH"
export SUPERTEAM_FAKE_OPENFGA_LOG="$TMP_DIR/openfga-fake.log"
export SUPERTEAM_DEV_OPENFGA_WAIT_URL=""
export SUPERTEAM_DEV_OPENFGA_DATASTORE_URI="file:$TMP_DIR/openfga.db"

run_script start openfga >"$TMP_DIR/start-openfga.out"
assert_pid_running openfga
assert_contains "$SUPERTEAM_FAKE_OPENFGA_LOG" "migrate migrate --datastore-engine sqlite --datastore-uri file:$TMP_DIR/openfga.db"
assert_contains "$SUPERTEAM_FAKE_OPENFGA_LOG" "run run --datastore-engine sqlite --datastore-uri file:$TMP_DIR/openfga.db"

run_script status openfga >"$TMP_DIR/status-openfga-running.out"
assert_contains "$TMP_DIR/status-openfga-running.out" "openfga: running"

run_script stop openfga >"$TMP_DIR/stop-openfga.out"
run_script status openfga >"$TMP_DIR/status-openfga-stopped.out"
assert_contains "$TMP_DIR/status-openfga-stopped.out" "openfga: stopped"

# 非法服务参数必须以非零码失败，而不是静默 exit 0。
if run_script status bogus-service >"$TMP_DIR/status-unknown.out" 2>&1; then
    echo "expected 'status bogus-service' to exit non-zero" >&2
    cat "$TMP_DIR/status-unknown.out" >&2
    exit 1
fi
assert_contains "$TMP_DIR/status-unknown.out" "unknown service: bogus-service"

# start/restart/stop 同样要拒绝非法服务名。
if run_script restart bogus-service >"$TMP_DIR/restart-unknown.out" 2>&1; then
    echo "expected 'restart bogus-service' to exit non-zero" >&2
    cat "$TMP_DIR/restart-unknown.out" >&2
    exit 1
fi
assert_contains "$TMP_DIR/restart-unknown.out" "unknown service: bogus-service"

# 非法动作必须以非零码失败。
if run_script frobnicate all >"$TMP_DIR/action-unknown.out" 2>&1; then
    echo "expected unknown action to exit non-zero" >&2
    cat "$TMP_DIR/action-unknown.out" >&2
    exit 1
fi
assert_contains "$TMP_DIR/action-unknown.out" "unknown action: frobnicate"

# 合法参数仍须以零码成功（回归保护，避免校验误伤正常路径）。
run_script status all >"$TMP_DIR/status-all.out"

echo "dev-services.test.sh: all assertions passed"
