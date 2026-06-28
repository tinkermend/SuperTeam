## Task 4: Control Plane Handlers

### What was implemented
- Added `PromptTemplate`, `PromptTemplateVariable`, and `CreatePromptTemplateRequest` to `contracts/control-plane/openapi.yaml`.
- Registered paths `GET /api/v1/templates`, `POST /api/v1/templates`, and `POST /api/v1/templates/{id}/apply` in `openapi.yaml`.
- Generated server types by running `cd apps/control-plane && go generate ./internal/api/`.
- Validated contracts by running `node scripts/verify-foundation-contracts.mjs`.
- Created `apps/control-plane/internal/prompttemplate/handler.go` which handles the HTTP requests, parses the context (using auth cookies and the `AuthService`), invokes the corresponding domain service methods, and maps between the domain and DTO representations.
- Wired the handler up in `apps/control-plane/internal/api/server.go` and the `apps/control-plane/internal/app/app.go` application composition root.

### Test Results
- Ran `cd apps/control-plane && go test ./internal/prompttemplate/... ./internal/app/... ./internal/api/...`.
- `github.com/superteam/control-plane/internal/prompttemplate`, `app`, and `api` tests successfully passed.
- (Note: `internal/api/handlers` tests had a pre-existing compilation issue with `claimAuthorizer` not implementing the recently added `CheckBulkTeamActions` method on `authz.Authorizer`, but this is unrelated to the prompt templates task).

### Files Changed
- Modified `contracts/control-plane/openapi.yaml`
- Created `apps/control-plane/internal/prompttemplate/handler.go`
- Modified `apps/control-plane/internal/api/server.go`
- Modified `apps/control-plane/internal/app/app.go`
- Generated files in `apps/control-plane/internal/api`

### Self-Review
- Checked that handlers adhere to the codebase patterns of extracting context and enforcing authorization.
- Used `AuthService` inside the handler layer to securely obtain the full `CurrentUserContext` which `Service.ListTemplates` and `Service.ApplyTemplate` expected.
- Hardcoded `IsAdmin: false` for template creation requests via the API since the web UI shouldn't explicitly bypass team membership checks via true role elevations during template creations directly, or if it should, an Authorizer check for a specific "tenant.admin" action would be required. Setting it to false restricts team-template creations safely to just team members.

### Issues / Concerns
- The generated HTTP handler has `IsAdmin` set to `false`. If tenant admins are intended to create templates scoped for teams they do NOT belong to through this API endpoint, then the handler will need to integrate with `authz.Authorizer` to evaluate a `tenant.admin` check to conditionally set `IsAdmin: true`. Currently, it securely defaults to `false`.

### Fix Report
- Fixed `handler.go` to use OpenAPI generated types (`gen.PromptTemplate`, `gen.CreatePromptTemplateRequest`, `gen.PromptTemplateVariable`).
- Added `apps/control-plane/internal/prompttemplate/handler_test.go` to cover auth failure, missing fields, invalid UUIDs, and success mappings.
- Output for `cd apps/control-plane && go test ./internal/prompttemplate/... -v`:
```
=== RUN   TestHandler_AuthFailure
--- PASS: TestHandler_AuthFailure (0.00s)
=== RUN   TestHandler_ListSuccess
--- PASS: TestHandler_ListSuccess (0.00s)
=== RUN   TestHandler_CreateTemplate
=== RUN   TestHandler_CreateTemplate/missing_fields/invalid_json
=== RUN   TestHandler_CreateTemplate/success
--- PASS: TestHandler_CreateTemplate (0.00s)
    --- PASS: TestHandler_CreateTemplate/missing_fields/invalid_json (0.00s)
    --- PASS: TestHandler_CreateTemplate/success (0.00s)
=== RUN   TestHandler_ApplyTemplate
=== RUN   TestHandler_ApplyTemplate/invalid_id
=== RUN   TestHandler_ApplyTemplate/success
--- PASS: TestHandler_ApplyTemplate (0.00s)
    --- PASS: TestHandler_ApplyTemplate/invalid_id (0.00s)
    --- PASS: TestHandler_ApplyTemplate/success (0.00s)
=== RUN   TestService_ListTemplates
--- PASS: TestService_ListTemplates (0.00s)
... [snip] ...
PASS
ok  	github.com/superteam/control-plane/internal/prompttemplate	0.574s
```
- Commits created: 1 commit.
