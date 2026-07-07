# Task 4 Report: Identity Step Without Employee Type Or Description

## TDD RED

- Command: `corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "without employee type or description"`
- Result: FAILED as expected.
- Evidence: `configures blank custom without employee type or description fields` found the disabled `员工类型` select on the identity step.

## TDD GREEN

- Removed user-visible `员工类型` and `描述` controls from the identity step.
- Kept internal `employee_type` submission behavior:
  - blank custom still submits `custom_agent`
  - template creation still submits the selected template type
- Removed `description` from the create-page draft and submit body only; backend/API types were not changed.

## Changed Files

- `apps/web/src/features/employees/create.tsx`
- `apps/web/src/features/employees/create.test.tsx`

## Test Results

- PASS: `corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "without employee type or description|submits blank-custom|creates a ready digital employee"`
  - 3 passed, 37 skipped.
- PASS/no matching test titles: `corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "描述|员工类型"`
  - 40 skipped.
- PASS: `git diff --check`

## Commit

- Commit hash: `3babd7eb`
- Message: `fix(web): hide internal employee type in create flow`

## Notes

- Scope intentionally excludes Task 5 risk/Provider extra tests and Task 6 team blocker/capability split.
- Real-chain browser/API smoke was not run because this task was explicitly scoped to focused web test files and the requested verification commands.
