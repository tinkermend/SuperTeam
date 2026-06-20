#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

OPENFGA_API_URL="${OPENFGA_API_URL:-http://127.0.0.1:8088}"
OPENFGA_STORE_NAME="${OPENFGA_STORE_NAME:-superteam-dev}"
OPENFGA_MODEL_FILE="${OPENFGA_MODEL_FILE:-$PROJECT_ROOT/apps/control-plane/internal/authz/openfga/model.json}"
OPENFGA_SMOKE_TENANT="${OPENFGA_SMOKE_TENANT:-bootstrap-tenant}"
OPENFGA_SMOKE_USER="${OPENFGA_SMOKE_USER:-bootstrap-admin}"

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "$1 is required" >&2
        exit 1
    fi
}

json_value() {
    local key="$1"
    python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$key"
}

json_string() {
    python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$1"
}

curl_json() {
    local method="$1"
    local path="$2"
    shift 2
    local args=(-fsS -X "$method" "$OPENFGA_API_URL$path" -H "content-type: application/json")
    if [ -n "${OPENFGA_API_TOKEN:-}" ]; then
        args+=(-H "Authorization: Bearer $OPENFGA_API_TOKEN")
    fi
    curl "${args[@]}" "$@"
}

require_cmd curl
require_cmd python3

if [ ! -f "$OPENFGA_MODEL_FILE" ]; then
    echo "OpenFGA model file not found: $OPENFGA_MODEL_FILE" >&2
    exit 1
fi

store_payload="{\"name\":$(json_string "$OPENFGA_STORE_NAME")}"
store_response="$(curl_json POST /stores -d "$store_payload")"
store_id="$(printf '%s' "$store_response" | json_value id)"

model_response="$(curl_json POST "/stores/$store_id/authorization-models" --data-binary "@$OPENFGA_MODEL_FILE")"
model_id="$(printf '%s' "$model_response" | json_value authorization_model_id)"

smoke_tuple="{\"user\":\"user:$OPENFGA_SMOKE_USER\",\"relation\":\"admin\",\"object\":\"tenant:$OPENFGA_SMOKE_TENANT\"}"
curl_json POST "/stores/$store_id/write" -d "{\"writes\":{\"tuple_keys\":[$smoke_tuple]},\"authorization_model_id\":\"$model_id\"}" >/dev/null

check_response="$(curl_json POST "/stores/$store_id/check" -d "{\"tuple_key\":$smoke_tuple,\"authorization_model_id\":\"$model_id\"}")"
allowed="$(printf '%s' "$check_response" | json_value allowed)"
if [ "$allowed" != "True" ] && [ "$allowed" != "true" ]; then
    echo "OpenFGA smoke check failed: $check_response" >&2
    exit 1
fi

cat <<EOF
AUTHZ_ENGINE=openfga_shadow
OPENFGA_API_URL=$OPENFGA_API_URL
OPENFGA_STORE_ID=$store_id
OPENFGA_MODEL_ID=$model_id
EOF
