import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = process.cwd();

const requiredOpenApiPaths = new Set([
  "/health",
  "/api/v1/tasks",
  "/api/v1/tasks/{taskId}",
  "/api/v1/tasks/{taskId}/status",
  "/api/v1/tasks/{taskId}/cancel",
  "/api/v1/runtime/register",
  "/api/v1/runtime/heartbeat",
  "/api/v1/runtime/tasks/claim",
  "/api/v1/runtime/tasks/{taskId}/events",
  "/api/v1/runtime/tasks/{taskId}/complete",
  "/api/v1/runtime/tasks/{taskId}/fail",
  "/api/v1/runtime/tasks/{taskId}/lease",
  "/api/v1/runtime/nodes",
  "/api/v1/runtime/nodes/{nodeId}",
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

function joinPaths(prefix, literal) {
  if (literal === "/") {
    return normalizePath(prefix);
  }

  return normalizePath(`${prefix.replace(/\/+$/, "")}/${literal.replace(/^\/+/, "")}`);
}

function readOpenApiPaths() {
  const openapi = readText("contracts/control-plane/openapi.yaml");
  const matches = [...openapi.matchAll(/^  (\/[^:\n]+):$/gm)];
  return new Set(matches.map((match) => normalizePath(match[1])));
}

function leadingWhitespaceLength(line) {
  return line.match(/^\s*/)[0].length;
}

function readGoRoutes() {
  const server = readText("apps/control-plane/internal/api/server.go");
  const routes = new Set();
  const scopes = [{ indent: -1, prefix: "" }];

  for (const line of server.split(/\r?\n/)) {
    const routeMatch = line.match(/\.Route\("([^"]+)"/);
    const endpointMatch = line.match(/\.(?:Get|Post|Put|Patch|Delete)\("([^"]+)"/);

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

    const route = joinPaths(scopes.at(-1).prefix, endpointMatch[1]);
    routes.add(route === "/api/v1/runtime/nodes/{taskId}" ? "/api/v1/runtime/nodes/{nodeId}" : route);
  }

  return routes;
}

function readRustClientPaths() {
  const client = readText("apps/runtime-agent/src/controlplane/client.rs");
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

const openApiPaths = readOpenApiPaths();
const goRoutes = readGoRoutes();
const rustClientPaths = readRustClientPaths();
const tsClientPaths = readTypeScriptClientPaths();

assertSetContainsAll("Control Plane OpenAPI", openApiPaths, requiredOpenApiPaths);
assertSetContainsAll("Go route registration", goRoutes, requiredOpenApiPaths);
assertSetContainsAll("Rust Control Plane client", rustClientPaths, requiredRustClientPaths);
assertSetContainsAll("TypeScript api-client", tsClientPaths, requiredTypeScriptClientPaths);
assertSetContainsAll("Rust Control Plane client", openApiPaths, rustClientPaths);
assertSetContainsAll("TypeScript api-client", openApiPaths, tsClientPaths);

console.log("foundation contract guard passed");
