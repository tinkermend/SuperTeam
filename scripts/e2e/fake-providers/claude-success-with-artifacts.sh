#!/usr/bin/env bash
# 假 Claude：成功终态 + 在工作区落下交付物，供 Runtime 经 CP presign 上传到对象存储。
# 若存在 .scratch/e2e/fake-produces.json（JSON 字符串数组），则按 produces 名写文件
# 并在 result_contract.deliverables 中同名声明，避免 handoff_deliverable_missing。
#
# 用法见 scripts/e2e/fake-providers/README.md 与 scripts/ops/smoke-runtime-artifact-rustfs.mjs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
PRODUCES_FILE="${SUPERTEAM_FAKE_PRODUCES_FILE:-$ROOT/.scratch/e2e/fake-produces.json}"
ACCEPT_FILE="${SUPERTEAM_FAKE_ACCEPT_FILE:-$ROOT/.scratch/e2e/fake-acceptance.json}"

# Runtime 每次派发前用 `<bin> --version` 探活（3s 超时）；不答会被判 provider_unavailable。
if [ "${1:-}" = "--version" ]; then
  echo "fake-provider 0.0.0 (superteam e2e artifacts)"
  exit 0
fi

# cwd 是任务工作区（claude provider current_dir=workspace_path）
# 声明式交付物写入本轮会话输出子目录（spec 2026-08-12 P0）；无会话目录时回退旧路径。
# 多会话并存时取最近修改的 sessions/*（勿用字典序首个，否则会串写入旧 command）。
DELIV_DIR="deliverables"
newest_session="$(
  python3 - <<'PY'
from pathlib import Path
root = Path(".superteam/sessions")
if not root.is_dir():
    raise SystemExit(0)
dirs = [p for p in root.iterdir() if p.is_dir()]
if not dirs:
    raise SystemExit(0)
print(max(dirs, key=lambda p: p.stat().st_mtime))
PY
)"
if [ -n "${newest_session}" ] && [ -d "${newest_session}" ]; then
  DELIV_DIR="${newest_session}/deliverables"
fi
mkdir -p "$DELIV_DIR"
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
marker="superteam-rustfs-runtime-artifact-${stamp}"

# 默认至少写一个声明式交付物 + 一个 execution_output 候选
printf '%s\n' "# RustFS runtime artifact smoke" "" "marker: ${marker}" "" "ok" \
  > "$DELIV_DIR/rustfs-smoke-report.md"
printf '%s\n' "execution_output candidate" "marker: ${marker}" \
  > "rustfs-smoke-notes.md"

# produces 名 → 文件 + result_contract.deliverables
deliv_json='[]'
if [ -f "$PRODUCES_FILE" ]; then
  # shellcheck disable=SC2016
  deliv_json="$(PRODUCES_FILE="$PRODUCES_FILE" MARKER="$marker" DELIV_DIR="$DELIV_DIR" python3 - <<'PY'
import json, os, pathlib
path = pathlib.Path(os.environ["PRODUCES_FILE"])
marker = os.environ["MARKER"]
root = pathlib.Path(os.environ["DELIV_DIR"])
try:
    names = json.loads(path.read_text())
except Exception:
    names = []
if not isinstance(names, list):
    names = []
deliverables = []
root.mkdir(parents=True, exist_ok=True)
for raw in names:
    name = str(raw).strip()
    if not name:
        continue
    # 文件名用 produce key，内容含 marker
    fname = f"{name}.md"
    (root / fname).write_text(
        f"# {name}\n\nmarker: {marker}\n\nok\n",
        encoding="utf-8",
    )
    deliverables.append({
        "name": name,
        "ref": f"{root.as_posix()}/{fname}",
        "value": "ok",
    })
print(json.dumps(deliverables, ensure_ascii=False))
PY
)"
fi

# result 字段是字符串；若本身是 JSON 对象文本，runtime 会 parse 出 result_contract
RESULT_OBJ="$(DELIV="$deliv_json" MARKER="$marker" ACCEPT_FILE="$ACCEPT_FILE" DELIV_DIR="$DELIV_DIR" python3 - <<'PY'
import json, os, pathlib
deliverables = json.loads(os.environ["DELIV"] or "[]")
marker = os.environ["MARKER"]
deliv_dir = os.environ.get("DELIV_DIR") or "deliverables"
accept_path = pathlib.Path(os.environ.get("ACCEPT_FILE") or "")
acceptance = []
if accept_path.is_file():
    try:
        raw = json.loads(accept_path.read_text())
        if isinstance(raw, list):
            for c in raw:
                text = str(c).strip()
                if text:
                    fallback = f"{deliv_dir}/rustfs-smoke-report.md"
                    acceptance.append({
                        "criterion": text,
                        "status": "passed",
                        "summary": f"satisfied by fake provider marker={marker}",
                        "evidence_refs": [deliverables[0]["ref"] if deliverables else fallback],
                    })
    except Exception:
        pass
payload = {
    "result_contract": {
        "status": "completed",
        "summary": f"wrote deliverables marker={marker}",
        "deliverables": deliverables,
        "acceptance_results": acceptance,
    }
}
print(json.dumps(payload, ensure_ascii=False))
PY
)"

# Claude stream-json：system 初始化 + 成功 result
printf '%s\n' '{"type":"system","subtype":"init","session_id":"ses_e2e_artifact"}'
# result 必须是 JSON 字符串值（内嵌整段 summary JSON）
RESULT_OBJ="$RESULT_OBJ" python3 -c 'import json,os; r=os.environ["RESULT_OBJ"]; print(json.dumps({"type":"result","result":r,"is_error":False}, ensure_ascii=False))'
exit 0
