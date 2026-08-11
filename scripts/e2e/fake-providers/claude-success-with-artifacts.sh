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
mkdir -p deliverables
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
marker="superteam-rustfs-runtime-artifact-${stamp}"

# 默认至少写一个声明式交付物 + 一个 execution_output 候选
printf '%s\n' "# RustFS runtime artifact smoke" "" "marker: ${marker}" "" "ok" \
  > "deliverables/rustfs-smoke-report.md"
printf '%s\n' "execution_output candidate" "marker: ${marker}" \
  > "rustfs-smoke-notes.md"

# produces 名 → 文件 + result_contract.deliverables
deliv_json='[]'
if [ -f "$PRODUCES_FILE" ]; then
  # shellcheck disable=SC2016
  deliv_json="$(PRODUCES_FILE="$PRODUCES_FILE" MARKER="$marker" python3 - <<'PY'
import json, os, pathlib
path = pathlib.Path(os.environ["PRODUCES_FILE"])
marker = os.environ["MARKER"]
try:
    names = json.loads(path.read_text())
except Exception:
    names = []
if not isinstance(names, list):
    names = []
deliverables = []
root = pathlib.Path("deliverables")
root.mkdir(exist_ok=True)
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
        "ref": f"deliverables/{fname}",
        "value": "ok",
    })
print(json.dumps(deliverables, ensure_ascii=False))
PY
)"
fi

# result 字段是字符串；若本身是 JSON 对象文本，runtime 会 parse 出 result_contract
RESULT_OBJ="$(DELIV="$deliv_json" MARKER="$marker" ACCEPT_FILE="$ACCEPT_FILE" python3 - <<'PY'
import json, os, pathlib
deliverables = json.loads(os.environ["DELIV"] or "[]")
marker = os.environ["MARKER"]
accept_path = pathlib.Path(os.environ.get("ACCEPT_FILE") or "")
acceptance = []
if accept_path.is_file():
    try:
        raw = json.loads(accept_path.read_text())
        if isinstance(raw, list):
            for c in raw:
                text = str(c).strip()
                if text:
                    acceptance.append({
                        "criterion": text,
                        "status": "passed",
                        "summary": f"satisfied by fake provider marker={marker}",
                        "evidence_refs": [f"deliverables/{deliverables[0]['name']}.md" if deliverables else "deliverables/rustfs-smoke-report.md"],
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
