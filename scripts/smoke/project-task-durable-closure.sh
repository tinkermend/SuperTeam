#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

scripts/dev-services.sh status

: "${SUPERTEAM_API_BASE:=http://127.0.0.1:8080}"
: "${SUPERTEAM_USERNAME:=admin}"
: "${SUPERTEAM_PASSWORD:=admin}"

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "missing required command: $name" >&2
    exit 1
  fi
}

require_command curl
require_command jq

COOKIE_JAR="${SUPERTEAM_COOKIE_JAR:-$ROOT_DIR/.scratch/smoke/project-task-durable-closure.cookie}"
mkdir -p "$(dirname "$COOKIE_JAR")"

login_if_needed() {
  if [[ -n "${SUPERTEAM_AUTH_TOKEN:-}" ]]; then
    return
  fi
  rm -f "$COOKIE_JAR"
  curl -fsS -c "$COOKIE_JAR" -b "$COOKIE_JAR" \
    -H "Content-Type: application/json" \
    --data "$(jq -nc --arg username "$SUPERTEAM_USERNAME" --arg password "$SUPERTEAM_PASSWORD" '{username: $username, password: $password}')" \
    "$SUPERTEAM_API_BASE/api/auth/login" >/dev/null
}

api() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local args=(-fsS -X "$method" "$SUPERTEAM_API_BASE$path")
  if [[ -n "${SUPERTEAM_AUTH_TOKEN:-}" ]]; then
    args+=(-H "Authorization: Bearer $SUPERTEAM_AUTH_TOKEN")
  else
    args+=(-c "$COOKIE_JAR" -b "$COOKIE_JAR")
  fi
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data "$body")
  fi
  curl "${args[@]}"
}

discover_fixture() {
  local projects_json project_id project_name tasks_json task_id demand_id task_status liveness_json graph_json
  projects_json="$(api GET "/api/v1/projects?limit=100")"
  while IFS=$'\t' read -r project_id project_name; do
    tasks_json="$(api GET "/api/v1/projects/$project_id/tasks?limit=1000")"
    while IFS=$'\t' read -r task_id demand_id task_status; do
      [[ "$task_status" == "completed" && -n "$demand_id" ]] || continue
      liveness_json="$(api GET "/api/v1/projects/$project_id/tasks/$task_id/liveness")"
      if ! echo "$liveness_json" | jq -e '.liveness == "terminal" and .current_attempt_id != null and .attempt.status != null' >/dev/null; then
        continue
      fi
      graph_json="$(api GET "/api/v1/projects/$project_id/task-graph?demand_id=$demand_id")"
      if ! echo "$graph_json" | jq -e --arg task_id "$task_id" '.execution_summaries[]? | select(.project_task_id == $task_id)' >/dev/null; then
        continue
      fi
      SUPERTEAM_PROJECT_ID="$project_id"
      SUPERTEAM_DEMAND_ID="$demand_id"
      SUPERTEAM_TASK_ID="$task_id"
      SUPERTEAM_ATTEMPT_ID="$(echo "$liveness_json" | jq -r '.current_attempt_id')"
      echo "discovered project-task smoke fixture: project=$SUPERTEAM_PROJECT_ID task=$SUPERTEAM_TASK_ID attempt=$SUPERTEAM_ATTEMPT_ID project_name=$project_name"
      return 0
    done < <(echo "$tasks_json" | jq -r '.[] | [.id, (.demand_id // ""), .status] | @tsv')
  done < <(echo "$projects_json" | jq -r '.[] | [.id, .name] | @tsv')

  echo "no completed attempt-backed ProjectTask fixture found; create or run a new durable-closure task, then rerun this smoke" >&2
  return 1
}

login_if_needed

if [[ -z "${SUPERTEAM_PROJECT_ID:-}" || -z "${SUPERTEAM_DEMAND_ID:-}" || -z "${SUPERTEAM_TASK_ID:-}" || -z "${SUPERTEAM_ATTEMPT_ID:-}" ]]; then
  discover_fixture
fi

tasks_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/tasks?limit=1000")"
task_json="$(echo "$tasks_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  '.[] | select(.id == $task_id)')"
echo "$task_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  '.id == $task_id and .status == "completed"' >/dev/null

liveness_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/tasks/$SUPERTEAM_TASK_ID/liveness")"
echo "$liveness_json" | jq -e \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  --arg attempt_id "$SUPERTEAM_ATTEMPT_ID" \
  '.project_task_id == $task_id
    and .current_attempt_id == $attempt_id
    and .liveness == "terminal"
    and .is_terminal == true
    and .next_action.source != null
    and .attempt.id == $attempt_id
    and (.attempt.status | IN("succeeded", "completed"))' >/dev/null

graph_json="$(api GET "/api/v1/projects/$SUPERTEAM_PROJECT_ID/task-graph?demand_id=$SUPERTEAM_DEMAND_ID")"
summary_id="$(echo "$graph_json" | jq -er \
  --arg task_id "$SUPERTEAM_TASK_ID" \
  '.execution_summaries[] | select(.project_task_id == $task_id) | .id' | head -n 1)"
if [[ -z "$summary_id" ]]; then
  echo "missing execution summary for project task $SUPERTEAM_TASK_ID" >&2
  exit 1
fi

task_status="$(echo "$task_json" | jq -r '.status')"
liveness="$(echo "$liveness_json" | jq -r '.liveness')"
echo "project-task durable closure smoke passed: task=$SUPERTEAM_TASK_ID attempt=$SUPERTEAM_ATTEMPT_ID status=$task_status liveness=$liveness execution_summary_id=$summary_id"
