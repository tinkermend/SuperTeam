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
  "GET /api/v1/runtime/nodes",
  "GET /api/v1/runtime/nodes/{nodeId}",
]);

const requiredRustClientPaths = new Set([
  "/api/v1/runtime/register",
  "/api/v1/runtime/heartbeat",
  "/api/v1/runtime/tasks/claim",
  "/api/v1/runtime/tasks/{taskId}/events",
  "/api/v1/tasks/{taskId}/status",
  "/api/v1/runtime/tasks/{taskId}/complete",
  "/api/v1/runtime/tasks/{taskId}/fail",
  "/api/v1/runtime/tasks/{taskId}/lease",
]);

const requiredTypeScriptClientPaths = new Set([
  "/health",
  "/api/v1/tasks",
  "/api/v1/runtime/nodes",
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
    .replace(/\/api\/v1\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/runtime/tasks/{taskId}")
    .replace(/\{id\}/g, "{taskId}")
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

function leadingWhitespaceLength(line) {
  return line.match(/^\s*/)[0].length;
}

function readGoRouteOperations() {
  const server = readText("apps/control-plane/internal/api/server.go");
  const operations = new Set();
  const scopes = [{ indent: -1, prefix: "" }];

  for (const line of server.split(/\r?\n/)) {
    const routeMatch = line.match(/\.Route\("([^"]+)"/);
    const endpointMatch = line.match(/\.(Get|Post|Put|Patch|Delete)\("([^"]+)"/);

    if (!routeMatch && !endpointMatch) {
      continue;
    }

    const indent = leadingWhitespaceLength(line);
    while (scopes.length > 1 && indent <= scopes.at(-1).indent) {
      scopes.pop();
    }

    if (routeMatch) {
      scopes.push({
        indent,
        prefix: joinPaths(scopes.at(-1).prefix, routeMatch[1]),
      });
      continue;
    }

    const route = joinPaths(scopes.at(-1).prefix, endpointMatch[2]);
    const path = route === "/api/v1/runtime/nodes/{taskId}" ? "/api/v1/runtime/nodes/{nodeId}" : route;
    operations.add(normalizeOperation(endpointMatch[1], path));
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
    "packages/api-client/src/health.ts",
    "packages/api-client/src/tasks.ts",
    "packages/api-client/src/runtime.ts",
  ];
  const paths = new Set();

  for (const file of files) {
    const text = readText(file);
    for (const match of text.matchAll(/["`]((?:\/health|\/api\/v1)[^"`$]*)["`]/g)) {
      paths.add(normalizePath(match[1]));
    }
    for (const match of text.matchAll(/`((?:\/health|\/api\/v1)[^`]*)`/g)) {
      paths.add(normalizePath(match[1].replaceAll("${taskId}", "{taskId}").replaceAll("${nodeId}", "{nodeId}")));
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
