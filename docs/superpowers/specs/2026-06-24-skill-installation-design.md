# Skill Installation Design
> 复核状态：06-24技能安装落地

Date: 2026-06-24
Status: Confirmed design, pending implementation plan

## 1. Background

SuperTeam already has a skill marketplace, skill upload, skill detail pages, team and digital employee skill bindings, Runtime Agent skill materialization, and provider adapters for `opencode`, `codex`, and `claude-code`.

The missing product loop is active installation: from the skill marketplace, a user should install a skill into a team or a single digital employee workspace, see where it is installed, and receive a clear failure reason when installation is impossible.

The product direction is a single marketplace-centered flow. Team and employee pages may show helper views, but the primary action starts from the skill marketplace.

## 2. Goals

- Add an `Install` action to each skill in the skill marketplace.
- Support installing a skill into one team or one digital employee.
- For team installation, install into every current digital employee in that team.
- Keep first-version semantics simple: synchronous short timeout, all-or-nothing success.
- Do not create background install jobs, retry queues, or partial-success states in the first version.
- Do preflight before touching any Runtime. If any target cannot be installed, fail immediately and report why.
- Persist successful installation records so skill detail can show installed teams and actual employee workspace paths.
- Persist failed installation attempts as structured logs or audit events, not as successful installation rows.
- Keep Console, Control Plane, Runtime, and Provider responsibilities aligned with the project architecture.

## 3. Non-Goals

- No failed-target retry queue.
- No partial success recovery UI.
- No background install task dashboard.
- No automatic team-size enforcement in skill installation. Team maximum size will be handled by team management.
- No support for providers outside `opencode`, `codex`, and `claude-code`.
- No global user-level provider skill installation.
- No frontend-only simulated success.
- No full log management menu in this feature slice. The backend should record structured failures for the future log center.

## 4. Provider Scope And Installation Directories

SuperTeam exposes one product concept: install a skill into a team or digital employee.

Runtime Agent adapts that unified operation to each provider's official project-level skill directory:

| Provider | Directory under `agent_home_dir` |
| --- | --- |
| `opencode` | `.opencode/skills/<skill-slug>/` |
| `codex` | `.agents/skills/<skill-slug>/` |
| `claude-code` | `.claude/skills/<skill-slug>/` |

The first version fails preflight for any other `provider_type`.

Official references used for the directory choices:

- OpenCode Agent Skills: `https://opencode.ai/docs/skills/`
- Codex Agent Skills: `https://developers.openai.com/codex/skills`
- Claude Code Skills: `https://docs.anthropic.com/en/docs/claude-code/skills`

## 5. Architecture

Skill installation is a synchronous Control Plane orchestration, not an implicit side effect of task execution.

Control Plane:

- Validates the request and authorizes the user.
- Expands the target team or employee into install targets.
- Runs preflight for every target.
- Sends one synchronous Runtime command per needed Runtime node, or one command with all targets if they share a node.
- Waits for Runtime completion up to a short timeout.
- Writes bindings and installation records only after every Runtime target succeeds.
- Writes structured failure logs or audit events when preflight, runtime installation, or timeout fails.

Runtime Agent:

- Receives `install_skills` commands.
- Downloads the skill zip from object storage.
- Verifies checksum and archive safety.
- Resolves the provider-specific install directory.
- Extracts into a temporary directory.
- Atomically replaces the provider skill directory.
- Rolls back directories written by the current command if any target fails and `rollback_on_failure=true`.
- Returns installed paths and per-target results.

Provider adapters:

- Own directory mapping by provider type.
- Do not carry Control Plane business state.

Web Console:

- Starts installation from the skill marketplace.
- Shows a target picker.
- Shows synchronous pending, success, and failure states.
- Shows persisted installed targets and workspace paths in skill detail.

## 6. Installation Flow

1. User clicks `Install` on a skill row in the skill marketplace.
2. Console opens an install dialog.
3. User selects target scope:
   - Team
   - Digital employee
4. Console calls `POST /api/v1/skills/{skillId}/install`.
5. Control Plane expands targets:
   - Team target becomes all current digital employees in the team.
   - Employee target stays one employee.
6. Control Plane preflight checks every target.
7. If any preflight check fails, Control Plane returns failure and dispatches no Runtime command.
8. If all preflight checks pass, Control Plane dispatches `install_skills`.
9. Runtime installs the archive into every target workspace.
10. If every target succeeds before timeout, Control Plane writes successful binding and installation records.
11. If any target fails or times out, Control Plane writes a failure log, returns failure, and writes no successful binding.

## 7. Preflight Rules

Control Plane must fail before Runtime dispatch when any target has one of these blockers:

- Skill archive metadata is incomplete:
  - missing `archive_object_ref`
  - missing `archive_checksum_sha256`
  - missing or invalid `archive_size_bytes`
  - missing or invalid `archive_file_count`
- Target employee is missing an execution instance.
- Target employee has no `runtime_node_id`.
- Target employee has no `agent_home_dir`.
- Target employee has no `provider_type`.
- `provider_type` is not one of `opencode`, `codex`, or `claude-code`.
- Runtime node is offline, disabled, archived, or not approved for the tenant/team.
- Runtime node cannot serve that team or employee according to existing Runtime scope rules.
- Provider capability for the target provider is unavailable or unhealthy on that node.

The response should identify the blocking employees and reasons. No filesystem writes happen when preflight fails.

## 8. Runtime Install Semantics

`install_skills` uses all-or-nothing semantics for the current request.

Runtime install steps per target:

1. Validate `provider_type`, `agent_home_dir`, `skill_key`, and archive metadata.
2. Download the skill archive from object storage.
3. Reject archives that exceed size or file count limits.
4. Verify checksum.
5. Reject unsafe zip entries:
   - absolute paths
   - `..`
   - path separators in skill key
   - symlink traversal if symlink support is ever added
6. Extract to a temp directory under `agent_home_dir`.
7. Atomically replace the provider-specific target directory.
8. Write a checksum marker for idempotent reinstall.
9. Return `installed_path`, checksum, and file count.

When `rollback_on_failure=true`, Runtime deletes directories written by the same command if any later target fails. It should not delete pre-existing directories that were not changed by the command.

## 9. Data Model

Reuse existing binding tables:

- `skill_team_bindings`
- `skill_agent_bindings`

Add a successful physical installation table:

```sql
CREATE TABLE skill_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    target_scope VARCHAR(40) NOT NULL,
    team_id UUID,
    digital_employee_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    provider_type VARCHAR(80) NOT NULL,
    installed_path TEXT NOT NULL,
    archive_checksum_sha256 VARCHAR(64) NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'installed',
    installed_by UUID,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
```

Recommended indexes:

- Unique active row by `(tenant_id, skill_id, digital_employee_id)` where `deleted_at IS NULL`
- List by skill: `(tenant_id, skill_id, installed_at DESC)` where `deleted_at IS NULL`
- List by employee: `(tenant_id, digital_employee_id, installed_at DESC)` where `deleted_at IS NULL`
- List by team: `(tenant_id, team_id, installed_at DESC)` where `deleted_at IS NULL`

`skill_installations` stores only successful physical installations. Failed attempts are recorded as structured logs or audit events, for example `skill.install.failed`.

Failure log fields:

- `tenant_id`
- `skill_id`
- `target_scope`
- `team_id`
- `digital_employee_id` when available
- `runtime_node_id` when available
- `provider_type` when available
- `phase`: `preflight`, `runtime_install`, or `timeout`
- `reason_code`
- `message`
- `command_id` when a Runtime command was dispatched
- structured per-target failure details

## 10. API Design

### `POST /api/v1/skills/{skillId}/install`

Request:

```json
{
  "target_scope": "team",
  "team_id": "00000000-0000-0000-0000-000000000000",
  "timeout_sec": 15
}
```

or:

```json
{
  "target_scope": "employee",
  "digital_employee_id": "00000000-0000-0000-0000-000000000000",
  "timeout_sec": 15
}
```

Success response:

```json
{
  "skill_id": "00000000-0000-0000-0000-000000000000",
  "target_scope": "team",
  "team_id": "00000000-0000-0000-0000-000000000000",
  "installed_count": 3,
  "installations": [
    {
      "digital_employee_id": "00000000-0000-0000-0000-000000000001",
      "provider_type": "codex",
      "runtime_node_id": "00000000-0000-0000-0000-000000000010",
      "installed_path": "/runtime/teams/.../employees/.../.agents/skills/review"
    }
  ]
}
```

Failure response:

```json
{
  "error": "skill_install_failed",
  "phase": "preflight",
  "message": "1 target cannot install this skill",
  "blocked_targets": [
    {
      "digital_employee_id": "00000000-0000-0000-0000-000000000001",
      "provider_type": "unknown-provider",
      "reason_code": "unsupported_provider",
      "message": "provider_type must be one of opencode, codex, claude-code"
    }
  ]
}
```

### `GET /api/v1/skills/{skillId}`

Extend the existing skill detail response with `installations` or an installation summary, depending on payload size.

### `GET /api/v1/skills/{skillId}/installations`

Returns installed targets for the skill detail page:

- team name
- employee name
- provider type
- Runtime node name/id
- installed path
- installed at
- installed by

## 11. Runtime Command Design

Command type: `install_skills`

Payload:

```json
{
  "command_id": "cmd-123",
  "tenant_id": "00000000-0000-0000-0000-000000000000",
  "skill": {
    "skill_id": "00000000-0000-0000-0000-000000000000",
    "skill_key": "review",
    "revision_id": null,
    "archive_object_ref": "s3://bucket/skills/tenant/review/hash.zip",
    "archive_checksum_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "archive_size_bytes": 12345,
    "archive_file_count": 12
  },
  "targets": [
    {
      "team_id": "00000000-0000-0000-0000-000000000000",
      "digital_employee_id": "00000000-0000-0000-0000-000000000001",
      "provider_type": "codex",
      "agent_home_dir": "/runtime/teams/.../employees/..."
    }
  ],
  "rollback_on_failure": true
}
```

Success result:

```json
{
  "status": "installed",
  "installed": [
    {
      "digital_employee_id": "00000000-0000-0000-0000-000000000001",
      "provider_type": "codex",
      "installed_path": "/runtime/teams/.../employees/.../.agents/skills/review",
      "archive_checksum_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "archive_file_count": 12
    }
  ]
}
```

Failure result:

```json
{
  "status": "failed",
  "rolled_back": true,
  "failed_target": "00000000-0000-0000-0000-000000000001",
  "reason_code": "checksum_mismatch",
  "message": "skill archive checksum mismatch"
}
```

## 12. Web Experience

Skill marketplace:

- Add `Install` action per skill row/card.
- Keep `View detail` as a separate action.
- Install button opens a compact dialog.

Install dialog:

- Target type segmented control: `Team` / `Digital employee`.
- Target selector.
- Shows selected target count for team installation.
- Submit button enters pending state while the synchronous request is in flight.
- Success message: installed count and target label.
- Failure message: phase, reason, and blocking employee list.

Skill detail:

- Installed teams section.
- Digital employee installations section:
  - employee
  - team
  - provider
  - Runtime
  - installed path
  - installed at
- If a team binding exists but expected employee installation rows are missing, show a consistency warning for troubleshooting. This is not a normal first-version state.

Log management:

- Backend writes structured failure logs now.
- Full log management menu can be implemented later.
- Skill detail may link to future logs but does not need to ship the full log center.

## 13. Error Handling

User-facing errors should be direct and actionable:

- "Digital employee has no active Runtime."
- "Runtime is offline."
- "Provider type is unsupported."
- "Skill archive is missing."
- "Runtime timed out while installing."
- "Runtime failed to verify skill archive checksum."

Do not expose secret values or raw object store credentials in errors.

## 14. Testing And Verification

Backend unit tests:

- Preflight fails and dispatches no command when archive metadata is missing.
- Preflight fails and dispatches no command when employee lacks Runtime.
- Preflight fails and dispatches no command when `agent_home_dir` is missing.
- Preflight fails and dispatches no command for unsupported provider.
- Successful team installation writes team binding and one installation per employee.
- Successful employee installation writes employee binding and one installation.
- Runtime failure writes no binding and records failure log.
- Timeout writes no binding and records timeout log.

Runtime tests:

- Provider directory mapping for `opencode`, `codex`, and `claude-code`.
- Reject unsupported provider.
- Reject unsafe skill key.
- Reject zip path traversal.
- Reject checksum mismatch.
- Idempotent reinstall with matching checksum marker.
- Atomic replacement of existing skill directory.
- `rollback_on_failure=true` removes directories written by the current command.

Web tests:

- Install button opens target dialog.
- Team target shows selected employee count.
- Success refreshes skill list/detail data.
- Preflight failure shows blocking employee reason.
- Runtime failure shows failure phase and message.

Real smoke:

- Use a real local Runtime Agent and at least one digital employee.
- Upload a real zip with `SKILL.md`.
- Install it through the API or Web.
- Verify the skill files exist under the provider-specific directory.
- Verify skill detail API and page show the installation row.

## 15. Implementation Planning Defaults

- Default timeout is 15 seconds unless the implementation plan finds an existing project-wide Runtime command timeout convention that should be reused.
- A team install may span multiple Runtime nodes. Control Plane should group targets by `runtime_node_id`, dispatch one `install_skills` command per node, and treat the full request as failed if any node command fails or times out.
- Structured failure logs should use existing audit/runtime-command recording first. Add a focused skill install event table only if existing logs cannot support skill detail and future log-center queries.
- Skill uninstall is out of scope for the first implementation plan. Install status can be replaced by reinstalling the same skill checksum or a newer archive revision.
