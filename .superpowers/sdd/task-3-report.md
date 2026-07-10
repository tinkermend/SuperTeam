Task 3 Report

Status
- Completed: ValidateRouteDecisionPlan no longer rejects plans for empty required_capabilities, MissingCapabilities, or hard-failure score checks.
- Deleted the skipped hard/missing capability rejection test from Task 1.
- Updated planner coverage so under-capable selections keep capability gaps as display evidence without forcing human review.

Files Changed
- apps/control-plane/internal/workflow/projectcoordination/graph_validation.go
- apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go
- apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go

RED Evidence
- Command:
  `GOWORK="/Users/tinker/src/singe/SuperTeam/.worktrees/plan-phase-02-retire-capability-control-flow/go.work" go test ./internal/workflow/projectcoordination/ -run TestValidateRouteDecisionPlanAcceptsEmptyRequiredCapabilities -v`
- Result:
  failed with `invalid route decision: task "root": required_capabilities is empty and the task is not flagged for human review`.

GREEN Evidence
- Command:
  `GOWORK="/Users/tinker/src/singe/SuperTeam/.worktrees/plan-phase-02-retire-capability-control-flow/go.work" go build ./internal/... && GOWORK="/Users/tinker/src/singe/SuperTeam/.worktrees/plan-phase-02-retire-capability-control-flow/go.work" go test ./internal/workflow/projectcoordination/`
- Result:
  `ok github.com/superteam/control-plane/internal/workflow/projectcoordination 0.486s`

Residual Check
- `rg "RequiredCapabilities|MissingCapabilities|HardFailures|required_capabilities|missing required|hard-failure" graph_validation.go` only finds requirement shape validation, display assignment, hard-failure approval in ApplyPlanningProfileScores, and planningTaskRequirements mapping.
- `rg "Rejects.*Capability|under-capable.*must require|must require human review|t\\.Skip\\(" *_test.go` found no stale rejection/skip assertions.

Concerns
- None for Task 3 scope.
- Verification is local package/build verification only; no service/API real-chain path was involved.
