import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = process.cwd();

const requiredOpenApiOperations = new Set([
  "GET /health",
  "GET /api/v1/tasks",
  "POST /api/v1/tasks",
  "GET /api/v1/tasks/{taskId}",
  "PUT /api/v1/tasks/{taskId}/status",
  "POST /api/v1/tasks/{taskId}/cancel",
  "POST /api/v1/runtime/register",
  "POST /api/v1/runtime/heartbeat",
  "POST /api/v1/runtime/tasks/claim",
  "POST /api/v1/runtime/tasks/{taskId}/events",
  "POST /api/v1/runtime/tasks/{taskId}/complete",
  "POST /api/v1/runtime/tasks/{taskId}/fail",
  "POST /api/v1/runtime/tasks/{taskId}/lease",
  "POST /api/v1/runtime/commands/{commandId}/events",
  "POST /api/v1/runtime/commands/{commandId}/complete",
  "POST /api/v1/runtime/commands/{commandId}/fail",
  "POST /api/v1/runtime/commands/{commandId}/cancelled",
  "POST /api/v1/runtime/commands/{commandId}/timed-out",
  "GET /api/v1/runtime/nodes",
  "GET /api/v1/runtime/nodes/{nodeId}",
  "GET /api/v1/teams",
  "POST /api/v1/teams",
  "GET /api/v1/teams/{teamId}",
  "POST /api/v1/teams/{teamId}/config-revisions",
  "GET /api/v1/teams/{teamId}/config-revisions/current",
  "POST /api/v1/digital-employees/{employeeId}/config-revisions",
  "POST /api/v1/digital-employees/{employeeId}/effective-configs/preview",
  "POST /api/v1/digital-employees/{employeeId}/effective-configs/approve",
  "GET /api/v1/digital-employees/{employeeId}/runs",
  "POST /api/v1/digital-employees/{employeeId}/runs",
  "GET /api/v1/digital-employees/{employeeId}/runs/{runId}",
  "GET /api/v1/digital-employees/{employeeId}/runs/{runId}/events",
  "POST /api/v1/digital-employees/{employeeId}/runs/{runId}/stop",
]);

const requiredRustClientPaths = new Set([
  "/api/v1/runtime/enrollments/hello",
  "/api/v1/runtime/register",
  "/api/v1/runtime/heartbeat",
  "/api/v1/runtime/tasks/claim",
  "/api/v1/runtime/tasks/{taskId}/events",
  "/api/v1/tasks/{taskId}/status",
  "/api/v1/runtime/tasks/{taskId}/complete",
  "/api/v1/runtime/tasks/{taskId}/fail",
  "/api/v1/runtime/tasks/{taskId}/lease",
  "/api/v1/runtime/commands/{commandId}/events",
  "/api/v1/runtime/commands/{commandId}/complete",
  "/api/v1/runtime/commands/{commandId}/fail",
  "/api/v1/runtime/sessions/{sessionId}/renew",
  "/api/v1/runtime/nodes/{nodeId}/capabilities",
]);

const requiredTypeScriptClientPaths = new Set([
  "/health",
  "/api/v1/tasks",
  "/api/v1/tasks/{taskId}",
  "/api/v1/tasks/{taskId}/status",
  "/api/v1/tasks/{taskId}/cancel",
  "/api/v1/runtime/nodes",
  "/api/v1/runtime/nodes/{nodeId}",
  "/api/v1/runtime/enrollments",
  "/api/v1/runtime/enrollments/{enrollmentId}/approve",
  "/api/v1/teams",
  "/api/v1/teams/{teamId}/config-revisions",
  "/api/v1/teams/{teamId}/config-revisions/current",
  "/api/v1/digital-employees",
  "/api/v1/digital-employees/{employeeId}/execution-instance",
  "/api/v1/digital-employees/{employeeId}/config-revisions",
  "/api/v1/digital-employees/{employeeId}/effective-configs/preview",
  "/api/v1/digital-employees/{employeeId}/effective-configs/approve",
  "/api/v1/digital-employees/{employeeId}/runs",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}/events",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}/stop",
]);

function readText(path) {
  return readFileSync(resolve(root, path), "utf8");
}

function normalizePath(path) {
  return path
    .split("?")[0]
    .replace(/\/+$/, "")
    .replace(/^$/, "/")
    .replace(/\{\}/g, "{taskId}")
    .replace(/\{command_id\}/g, "{commandId}")
    .replace(/\/api\/v1\/runtime\/commands\/\{taskId\}/g, "/api/v1/runtime/commands/{commandId}")
    .replace(/\/api\/v1\/runtime\/commands\/\{id\}/g, "/api/v1/runtime/commands/{commandId}")
    .replace(/\/api\/v1\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/runtime/tasks/{taskId}")
    .replace(/\/api\/v1\/tasks\/\{id\}/g, "/api/v1/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/tasks\/\{id\}/g, "/api/v1/runtime/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/sessions\/\{taskId\}/g, "/api/v1/runtime/sessions/{sessionId}")
    .replace(/\/api\/v1\/runtime\/nodes\/\{taskId\}/g, "/api/v1/runtime/nodes/{nodeId}")
    .replace(/\/api\/v1\/runtime\/sessions\/\{id\}/g, "/api/v1/runtime/sessions/{sessionId}")
    .replace(/\/api\/v1\/runtime\/nodes\/\{id\}/g, "/api/v1/runtime/nodes/{nodeId}")
    .replace(/\{nodeId\}/g, "{nodeId}");
}

function normalizeOperation(method, path) {
  return `${method.toUpperCase()} ${normalizePath(path)}`;
}

function joinPaths(prefix, literal) {
  if (literal === "/") {
    return normalizePath(prefix);
  }

  return normalizePath(`${prefix.replace(/\/+$/, "")}/${literal.replace(/^\/+/, "")}`);
}

function readOpenApiOperations() {
  const openapi = readText("contracts/control-plane/openapi.yaml");
  const operations = new Set();
  let currentPath = null;

  for (const line of openapi.split(/\r?\n/)) {
    const pathMatch = line.match(/^  (\/[^:\n]+):$/);
    if (pathMatch) {
      currentPath = pathMatch[1];
      continue;
    }

    const methodMatch = line.match(/^    (get|post|put|patch|delete):$/);
    if (currentPath && methodMatch) {
      operations.add(normalizeOperation(methodMatch[1], currentPath));
    }
  }

  return operations;
}

function readOpenApiPaths() {
  return new Set([...readOpenApiOperations()].map((operation) => operation.split(" ")[1]));
}

function braceDeltaOutsideStrings(line) {
  const withoutStrings = line.replace(/"[^"]*"/g, '""');
  const opens = withoutStrings.match(/\{/g)?.length ?? 0;
  const closes = withoutStrings.match(/\}/g)?.length ?? 0;
  return opens - closes;
}

function readGoRouteOperations() {
  const server = readText("apps/control-plane/internal/api/server.go");
  const operations = new Set();
  const scopes = [{ depth: -1, prefix: "" }];
  let blockDepth = 0;

  for (const line of server.split(/\r?\n/)) {
    while (scopes.length > 1 && blockDepth < scopes.at(-1).depth) {
      scopes.pop();
    }

    const routeMatch = line.match(/\.Route\("([^"]+)"/);
    const endpointMatch = line.match(/\.(Get|Post|Put|Patch|Delete)\("([^"]+)"/);
    const lineBraceDelta = braceDeltaOutsideStrings(line);

    if (!routeMatch && !endpointMatch) {
      blockDepth += lineBraceDelta;
      continue;
    }

    if (routeMatch) {
      scopes.push({
        depth: blockDepth + lineBraceDelta,
        prefix: joinPaths(scopes.at(-1).prefix, routeMatch[1]),
      });
      blockDepth += lineBraceDelta;
      continue;
    }

    const route = joinPaths(scopes.at(-1).prefix, endpointMatch[2]);
    const path = route === "/api/v1/runtime/nodes/{taskId}" ? "/api/v1/runtime/nodes/{nodeId}" : route;
    operations.add(normalizeOperation(endpointMatch[1], path));
    blockDepth += lineBraceDelta;
  }

  return operations;
}

function readRustClientPaths() {
  const client = readText("apps/runtime-agent/src/controlplane/client.rs").split("#[cfg(test)]")[0];
  const stringPaths = [...client.matchAll(/\/api\/v1\/[^"?\s]+/g)].map((match) => match[0]);
  const formatPaths = [...client.matchAll(/\/api\/v1\/[^"?\s{]*(?:\{\}[^"?\s{]*)+/g)].map(
    (match) => match[0].replaceAll("{}", "{taskId}"),
  );

  return new Set([...stringPaths, ...formatPaths].map(normalizePath));
}

function readTypeScriptClientPaths() {
  const files = [
    "apps/web/src/lib/api/health.ts",
    "apps/web/src/lib/api/tasks.ts",
    "apps/web/src/lib/api/runtime.ts",
    "apps/web/src/lib/api/teams.ts",
    "apps/web/src/lib/api/employees.ts",
  ];
  const paths = new Set();

  for (const file of files) {
    const text = readText(file);
    for (const match of text.matchAll(/["`]((?:\/health|\/api\/v1)(?:\/(?:[A-Za-z0-9_-]+|\$\{[A-Za-z0-9_]+\}))*)/g)) {
      paths.add(
        normalizePath(
          match[1]
            .replaceAll("${taskId}", "{taskId}")
            .replaceAll("${nodeId}", "{nodeId}")
            .replaceAll("${encodedNodeId}", "{nodeId}")
            .replaceAll("${enrollmentId}", "{enrollmentId}")
            .replaceAll("${encodedEnrollmentId}", "{enrollmentId}")
            .replaceAll("${teamId}", "{teamId}")
            .replaceAll("${encodedTeamId}", "{teamId}")
            .replaceAll("${employeeId}", "{employeeId}")
            .replaceAll("${encodedEmployeeId}", "{employeeId}")
            .replaceAll("${runId}", "{runId}")
            .replaceAll("${encodedRunId}", "{runId}"),
        ),
      );
    }
  }

  return paths;
}

function assertSetContainsAll(label, actual, expected) {
  const missing = [...expected].filter((path) => !actual.has(path));
  if (missing.length > 0) {
    throw new Error(`${label} missing paths:\n${missing.map((path) => `- ${path}`).join("\n")}`);
  }
}

const openApiOperations = readOpenApiOperations();
const goRouteOperations = readGoRouteOperations();
const openApiPaths = readOpenApiPaths();
const rustClientPaths = readRustClientPaths();
const tsClientPaths = readTypeScriptClientPaths();

assertSetContainsAll("Control Plane OpenAPI", openApiOperations, requiredOpenApiOperations);
assertSetContainsAll("Go route registration", goRouteOperations, requiredOpenApiOperations);
assertSetContainsAll("Rust Control Plane client", rustClientPaths, requiredRustClientPaths);
assertSetContainsAll("TypeScript api-client", tsClientPaths, requiredTypeScriptClientPaths);
assertSetContainsAll("Rust Control Plane client", openApiPaths, rustClientPaths);
assertSetContainsAll("TypeScript api-client", openApiPaths, tsClientPaths);

console.log("foundation contract guard passed");
