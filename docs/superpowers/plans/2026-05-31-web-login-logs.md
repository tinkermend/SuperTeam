# Web Login Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add independent Web login log persistence and a real backend query interface without using the existing human-review `audit_events` table.

**Architecture:** Create a Web logging domain backed by `web_login_logs` and `web_operation_logs`. The auth service writes login/logout events through the auth repository, and the auth OpenAPI handler exposes a cookie-protected login log list endpoint.

**Tech Stack:** Go, chi/net/http, oapi-codegen, sqlc, Atlas migrations, PostgreSQL, Next.js/React, TypeScript API client, Vitest.

---

### Task 1: Lock Auth Service Log Behavior With Failing Tests

**Files:**
- Modify: `apps/control-plane/internal/auth/types.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Modify: `apps/control-plane/internal/auth/service_test.go`

- [ ] Add domain types for `LoginLog`, `CreateLoginLogParams`, and `ListLoginLogsFilter`.
- [ ] Extend the auth repository contract with `CreateLoginLog` and `ListLoginLogs`.
- [ ] Write service tests that require successful login, failed login, and logout to create separate login log events.
- [ ] Run `go test ./apps/control-plane/internal/auth -run 'TestLogin|TestLogout' -count=1` and verify the new tests fail before implementation.
- [ ] Implement the minimum service changes to record events and list logs.
- [ ] Re-run the auth package tests and verify they pass.

### Task 2: Add Database Schema And SQLC Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/004_create_web_logs.sql`
- Modify: `apps/control-plane/internal/storage/migrations/001_initial.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Create: `apps/control-plane/internal/storage/queries/web_logs.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/*`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`

- [ ] Add `web_login_logs` and `web_operation_logs` tables with explicit event/action/result fields, user/session metadata, request metadata, JSON details, timestamps, and indexes.
- [ ] Add a forward migration for already-migrated databases and mirror the schema in `001_initial.sql` for fresh databases.
- [ ] Add sqlc queries for creating and listing login logs.
- [ ] Run sqlc generation.
- [ ] Add/adjust migration tests to assert the new table and index names.
- [ ] Run storage migration/query tests.

### Task 3: Expose Cookie-Protected Login Log API

**Files:**
- Modify: `contracts/control-plane/auth.yaml`
- Modify generated: `apps/control-plane/internal/auth/generated.go`
- Modify: `apps/control-plane/internal/auth/handler.go`
- Modify: `apps/control-plane/internal/auth/pg_repository.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`

- [ ] Add `GET /api/auth/login-logs` with `limit` and `offset` query params to the auth OpenAPI contract.
- [ ] Generate auth server code.
- [ ] Implement `ListLoginLogs` in the HTTP handler; require a valid session cookie before returning data.
- [ ] Map DB/domain records to API response records.
- [ ] Add route tests for authenticated access and unauthenticated rejection.
- [ ] Run API/auth tests.

### Task 4: Add Web API Client Support

**Files:**
- Modify: `packages/api-client/src/auth-api.ts`
- Modify: `packages/api-client/src/auth-api.test.ts`
- Modify: `packages/api-client/src/index.ts`

- [ ] Add TypeScript types for login log records and list responses.
- [ ] Add `listLoginLogs`.
- [ ] Add a Vitest case proving the endpoint, query params, and credentials mode.
- [ ] Run the API client tests and typecheck.

### Task 5: Changelog And Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] Add a Simplified Chinese changelog entry describing Web login log tables, login/logout write path, and query API.
- [ ] Apply the new migration against the configured development database if `DATABASE_URL` is available.
- [ ] Run `go test ./apps/control-plane/... -count=1`.
- [ ] Run `pnpm -r --if-present test`.
- [ ] Run `pnpm -r --if-present typecheck`.
- [ ] Review `git diff --stat` and ensure no unrelated changes were reverted.
