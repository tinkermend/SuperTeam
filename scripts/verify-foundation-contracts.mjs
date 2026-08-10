import { existsSync, readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";

const require = createRequire(import.meta.url);
const root = process.cwd();

const CONTROL_PLANE_OPENAPI = "contracts/control-plane/openapi.yaml";
const CONTROL_PLANE_GENERATED_GO = "apps/control-plane/internal/api/gen/control_plane.gen.go";

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
  "PUT /api/v1/runtime/tasks/{taskId}/status",
  "POST /api/v1/runtime/tasks/{taskId}/complete",
  "POST /api/v1/runtime/tasks/{taskId}/fail",
  "POST /api/v1/runtime/tasks/{taskId}/lease",
  "POST /api/v1/runtime/commands/{commandId}/events",
  "POST /api/v1/runtime/commands/{commandId}/complete",
  "POST /api/v1/runtime/commands/{commandId}/fail",
  "POST /api/v1/runtime/commands/{commandId}/cancelled",
  "POST /api/v1/runtime/commands/{commandId}/timed-out",
  "POST /api/v1/runtime/project-task-attempts/{attemptId}/started",
  "POST /api/v1/runtime/project-task-attempts/{attemptId}/lease",
  "POST /api/v1/runtime/project-task-attempts/{attemptId}/complete",
  "POST /api/v1/runtime/project-task-attempts/{attemptId}/fail",
  "POST /api/v1/runtime/project-task-attempts/{attemptId}/wait-human",
  "GET /api/v1/runtime/nodes",
  "GET /api/v1/runtime/nodes/{nodeId}",
  "GET /api/v1/teams",
  "POST /api/v1/teams",
  "GET /api/v1/teams/{teamId}",
  "GET /api/v1/audit/events",
  "PATCH /api/v1/teams/{teamId}/constitution",
  "POST /api/v1/digital-employees/{employeeId}/config-revisions",
  "GET /api/v1/digital-employees/{employeeId}/runs",
  "POST /api/v1/digital-employees/{employeeId}/runs",
  "GET /api/v1/digital-employees/{employeeId}/runs/{runId}",
  "GET /api/v1/digital-employees/{employeeId}/runs/{runId}/events",
  "POST /api/v1/digital-employees/{employeeId}/runs/{runId}/stop",
  "GET /api/v1/skills",
  "POST /api/v1/skills/uploads",
  "GET /api/v1/skills/{skillId}",
  "DELETE /api/v1/skills/{skillId}",
  "GET /api/v1/projects",
  "POST /api/v1/projects",
  "GET /api/v1/projects/{projectId}",
  "PATCH /api/v1/projects/{projectId}",
  "POST /api/v1/projects/{projectId}/archive",
  "GET /api/v1/projects/{projectId}/overview",
  "GET /api/v1/projects/{projectId}/members",
  "PUT /api/v1/projects/{projectId}/members",
  "GET /api/v1/projects/{projectId}/tasks",
  "GET /api/v1/projects/{projectId}/events",
  "GET /api/v1/projects/{projectId}/config",
  "PUT /api/v1/projects/{projectId}/config",
  "GET /api/v1/projects/{projectId}/demands",
  "POST /api/v1/projects/{projectId}/demands",
  "GET /api/v1/projects/{projectId}/route-decisions",
  "GET /api/v1/projects/{projectId}/coordination-jobs",
  "GET /api/v1/projects/{projectId}/decisions",
  "POST /api/v1/projects/{projectId}/decisions/{decisionId}/resolve",
  "GET /api/v1/projects/{projectId}/execution-summaries",
  "GET /api/v1/projects/{projectId}/transfer-requests",
  "GET /api/v1/projects/{projectId}/evidence",
  "POST /api/v1/projects/{projectId}/evidence",
  "PATCH /api/v1/projects/{projectId}/evidence/{evidenceId}",
  "GET /api/v1/projects/{projectId}/artifacts",
  "GET /api/v1/projects/{projectId}/reports",
  "GET /api/v1/projects/{projectId}/budget-ledger",
  "GET /api/v1/projects/{projectId}/budget-summary",
  "GET /api/v1/projects/{projectId}/acceptance",
  "GET /api/v1/projects/{projectId}/archive-preview",
  "POST /api/v1/projects/{projectId}/archive-snapshot",
  "GET /api/v1/projects/{projectId}/archive-snapshots",
  "GET /api/v1/projects/{projectId}/config-revisions",
  "GET /api/v1/projects/{projectId}/config-revisions/{revisionId}",
]);

const requiredRustClientPaths = new Set([
  "/api/v1/runtime/enrollments/hello",
  "/api/v1/runtime/register",
  "/api/v1/runtime/heartbeat",
  "/api/v1/runtime/sessions/{sessionId}/renew",
  "/api/v1/runtime/nodes/{nodeId}/capabilities",
  "/api/v1/runtime/commands/{commandId}/events",
  "/api/v1/runtime/commands/{commandId}/complete",
  "/api/v1/runtime/commands/{commandId}/fail",
  "/api/v1/runtime/commands/{commandId}/cancelled",
  "/api/v1/runtime/project-task-attempts/{attemptId}/started",
  "/api/v1/runtime/project-task-attempts/{attemptId}/complete",
  "/api/v1/runtime/project-task-attempts/{attemptId}/fail",
  "/api/v1/runtime/project-task-attempts/{attemptId}/wait-human",
  "/api/v1/runtime/project-task-attestations",
  "/api/v1/runtime/project-task-attempts/{attemptId}/budget-heartbeat",
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
  "/api/v1/teams/{teamId}/constitution",
  "/api/v1/digital-employees",
  "/api/v1/digital-employees/{employeeId}/config-revisions",
  "/api/v1/digital-employees/{employeeId}/runs",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}/events",
  "/api/v1/digital-employees/{employeeId}/runs/{runId}/stop",
  "/api/v1/skills",
  "/api/v1/skills/uploads",
  "/api/v1/automations",
  "/api/v1/automations/{ruleId}",
  "/api/v1/automations/{ruleId}/enable",
  "/api/v1/automations/{ruleId}/disable",
  "/api/v1/automations/{ruleId}/trigger",
  "/api/v1/automations/{ruleId}/fires",
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
    .replace(/\/api\/v1\/runtime\/project-task-attempts\/\{taskId\}/g, "/api/v1/runtime/project-task-attempts/{attemptId}")
    .replace(/\/api\/v1\/runtime\/project-task-attempts\/\{id\}/g, "/api/v1/runtime/project-task-attempts/{attemptId}")
    .replace(/\/api\/v1\/runtime\/project-task-attempts\/\{attempt_id\}/g, "/api/v1/runtime/project-task-attempts/{attemptId}")
    .replace(/\/api\/v1\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/tasks\/[0-9]+(?=\/|$)/g, "/api/v1/runtime/tasks/{taskId}")
    .replace(/\/api\/v1\/tasks\/\{id\}/g, "/api/v1/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/tasks\/\{id\}/g, "/api/v1/runtime/tasks/{taskId}")
    .replace(/\/api\/v1\/runtime\/sessions\/\{taskId\}/g, "/api/v1/runtime/sessions/{sessionId}")
    .replace(/\/api\/v1\/runtime\/nodes\/\{taskId\}/g, "/api/v1/runtime/nodes/{nodeId}")
    .replace(/\/api\/v1\/runtime\/sessions\/\{id\}/g, "/api/v1/runtime/sessions/{sessionId}")
    .replace(/\/api\/v1\/runtime\/nodes\/\{id\}/g, "/api/v1/runtime/nodes/{nodeId}")
    .replace(/\{nodeId\}/g, "{nodeId}")
    .replace(/\{projectId\}/g, "{projectId}")
    .replace(/\{evidenceId\}/g, "{evidenceId}")
    .replace(/\{decisionId\}/g, "{decisionId}")
    .replace(/\{revisionId\}/g, "{revisionId}")
    .replace(/\{projectTaskId\}/g, "{projectTaskId}")
    .replace(/\{attemptId\}/g, "{attemptId}");
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
  const openapi = readText(CONTROL_PLANE_OPENAPI);
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
    "apps/web/src/lib/api/skills.ts",
    "apps/web/src/lib/api/automations.ts",
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
            .replaceAll("${encodedRunId}", "{runId}")
            .replaceAll("${envName}", "{envName}")
            .replaceAll("${encodedEnvName}", "{envName}")
            .replaceAll("${skillId}", "{skillId}")
            .replaceAll("${encodedSkillId}", "{skillId}")
            .replaceAll("${projectId}", "{projectId}")
            .replaceAll("${encodedProjectId}", "{projectId}")
            .replaceAll("${ruleId}", "{ruleId}")
            .replaceAll("${encodedRuleId}", "{ruleId}")
            // Provider 原生配置（b84645d7）：openapi 侧是 {providerType}/{configKey}
            .replaceAll("${providerType}", "{providerType}")
            .replaceAll("${encodedProvider}", "{providerType}")
            .replaceAll("${configKey}", "{configKey}")
            .replaceAll("${encodedKey}", "{configKey}"),
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

/**
 * components.schemas 下声明的 schema 名（顶层两级缩进的 `Name:`）。
 */
function readOpenApiSchemaNames() {
  const lines = readText(CONTROL_PLANE_OPENAPI).split("\n");
  const names = new Set();
  let inComponents = false;
  let inSchemas = false;
  for (const line of lines) {
    if (/^components:\s*$/.test(line)) {
      inComponents = true;
      inSchemas = false;
      continue;
    }
    if (inComponents && /^\S/.test(line)) {
      // 离开 components 顶层块。
      inComponents = false;
      inSchemas = false;
      continue;
    }
    if (!inComponents) continue;
    if (/^ {2}schemas:\s*$/.test(line)) {
      inSchemas = true;
      continue;
    }
    if (inSchemas && /^ {2}\S/.test(line)) {
      // components 下的另一个小节（parameters/responses/...）。
      inSchemas = false;
      continue;
    }
    if (!inSchemas) continue;
    const match = /^ {4}([A-Za-z][A-Za-z0-9_]*):\s*$/.exec(line);
    if (match) names.add(match[1]);
  }
  return names;
}

/** 生成物里已声明的 Go 类型名。 */
function readGeneratedGoTypeNames() {
  const names = new Set();
  for (const line of readText(CONTROL_PLANE_GENERATED_GO).split("\n")) {
    const match = /^type ([A-Za-z][A-Za-z0-9_]*)\b/.exec(line);
    if (match) names.add(match[1]);
  }
  return names;
}

/**
 * 生成物新鲜度：契约里每个 schema 都必须能在生成物里找到同名 Go 类型。
 *
 * 立此门禁的由来：56b39666 / 6a531a86 改了 openapi.yaml 却没跑生成器，
 * control_plane.gen.go 落后约 290 行（缺 FeishuChannelAppStatus / channel_alert 等），
 * 而本守卫当时只比对路由与客户端路径，整段漂移一路绿灯提交。
 *
 * 边界（不要误以为它等价于重跑生成器）：只覆盖"新增 schema 未重新生成"这一类漂移；
 * 既有 schema 内部字段/枚举值的改动检测不到。彻底的做法是重跑 oapi-codegen 后
 * diff，但那会让本守卫依赖 Go 工具链——按需要再升级。
 */
function assertGeneratedGoIsFresh() {
  const schemaNames = readOpenApiSchemaNames();
  if (schemaNames.size === 0) {
    throw new Error("failed to parse components.schemas from control-plane openapi.yaml");
  }
  const goTypes = readGeneratedGoTypeNames();
  const missing = [...schemaNames].filter((name) => !goTypes.has(name));
  if (missing.length > 0) {
    throw new Error(
      `generated Go is stale vs contract (run \`corepack pnpm generate:control-plane\`).\n` +
        `${CONTROL_PLANE_GENERATED_GO} is missing types for these schemas:\n` +
        missing.map((name) => `- ${name}`).join("\n"),
    );
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
assertGeneratedGoIsFresh();
assertProviderSemanticContracts();

console.log("foundation contract guard passed");

/**
 * Provider semantic unification: schemas/fixtures/goldens must exist, parse,
 * and (Phase 4) validate fixtures with ajv against schemas.
 * Wire transport remains control-plane openapi.
 */
function assertProviderSemanticContracts() {
  const providerRoot = resolve(root, "contracts/provider");
  const requiredSchemas = [
    "schemas/provider-error.schema.json",
    "schemas/provider-result.schema.json",
    "schemas/provider-event.schema.json",
    "schemas/provider-capability.schema.json",
    "schemas/provider-usage.schema.json",
    "schemas/failure-family.json",
    "schemas/start-session-payload.schema.json",
  ];
  const schemaByName = new Map();
  for (const rel of requiredSchemas) {
    const path = resolve(providerRoot, rel);
    if (!existsSync(path)) {
      throw new Error(`provider contract missing: contracts/provider/${rel}`);
    }
    schemaByName.set(rel, JSON.parse(readFileSync(path, "utf8")));
  }

  let Ajv;
  try {
    Ajv = require("ajv").default || require("ajv");
  } catch {
    throw new Error(
      "ajv is required for provider schema validation (devDependency). Run: corepack pnpm install",
    );
  }
  // draft-2020-12: Ajv v8 default is draft-07; use 2020 if available.
  let ajv;
  try {
    const Ajv2020 = require("ajv/dist/2020.js").default || require("ajv/dist/2020.js");
    ajv = new Ajv2020({ allErrors: true, strict: false });
  } catch {
    ajv = new Ajv({ allErrors: true, strict: false });
  }
  // Register schemas by $id basename for $ref resolution.
  for (const schema of schemaByName.values()) {
    if (schema.$id) {
      try {
        ajv.addSchema(schema);
      } catch {
        // duplicate id on re-run — ignore
      }
    }
  }

  const fixturesDir = resolve(providerRoot, "fixtures");
  if (!existsSync(fixturesDir)) {
    throw new Error("provider contract missing: contracts/provider/fixtures/");
  }
  const fixtures = readdirSync(fixturesDir).filter((name) => name.endsWith(".json"));
  if (fixtures.length === 0) {
    throw new Error("provider fixtures empty: contracts/provider/fixtures/");
  }

  const fixtureSchema = {
    "error-": "schemas/provider-error.schema.json",
    "result-": "schemas/provider-result.schema.json",
    "start-session-": "schemas/start-session-payload.schema.json",
  };

  for (const name of fixtures) {
    const body = JSON.parse(readFileSync(resolve(fixturesDir, name), "utf8"));
    let schemaRel = null;
    for (const [prefix, rel] of Object.entries(fixtureSchema)) {
      if (name.startsWith(prefix)) {
        schemaRel = rel;
        break;
      }
    }
    if (!schemaRel) {
      // Legacy envelope key check for unnamed fixtures.
      const envelope = name.startsWith("error-") ? body : body.error;
      if (envelope && typeof envelope === "object") {
        for (const key of [
          "schema_version",
          "code",
          "family",
          "retryable",
          "message",
          "provider_type",
        ]) {
          if (!(key in envelope)) {
            throw new Error(`fixture ${name}: ErrorEnvelope missing ${key}`);
          }
        }
      }
      continue;
    }
    const schema = schemaByName.get(schemaRel);
    const validate = ajv.compile(schema);
    if (!validate(body)) {
      throw new Error(
        `fixture ${name} failed schema ${schemaRel}:\n${ajv.errorsText(validate.errors)}`,
      );
    }
  }

  const goldenRoot = resolve(providerRoot, "golden");
  for (const provider of ["claude-code", "opencode", "codex"]) {
    const dir = resolve(goldenRoot, provider);
    if (!existsSync(dir)) {
      throw new Error(`provider golden missing: contracts/provider/golden/${provider}/`);
    }
    const cases = readdirSync(dir).filter((n) => n.endsWith(".json"));
    if (cases.length === 0) {
      throw new Error(`provider golden empty: contracts/provider/golden/${provider}/`);
    }
    for (const name of cases) {
      const body = JSON.parse(readFileSync(resolve(dir, name), "utf8"));
      if (!Array.isArray(body.native_lines) || body.native_lines.length === 0) {
        throw new Error(`golden ${provider}/${name}: native_lines required`);
      }
    }
  }
}
