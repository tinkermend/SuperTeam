/**
 * Resolve fixture IDs by name via API (no hard-coded UUIDs required).
 * Env overrides still win when set.
 */
import { apiOk } from "./cp-client.mjs";

const DEFAULT_PROJECT_NAME =
  process.env.SUPERTEAM_PROJECT_NAME || "批二基线项目 P1";

export async function resolveFixtures(cookie) {
  const projects = await apiOk(cookie, "/api/v1/projects?limit=100");
  const projectItems = projects?.items ?? projects ?? [];
  const envProjectId = process.env.SUPERTEAM_PROJECT_ID;
  let project =
    (envProjectId && projectItems.find((p) => p.id === envProjectId)) ||
    projectItems.find((p) => p.name === DEFAULT_PROJECT_NAME) ||
    projectItems[0];
  if (!project) throw new Error("no project fixture found");

  const employees = await apiOk(cookie, "/api/v1/digital-employees?limit=100");
  const listed = employees?.items ?? employees ?? [];
  // list endpoint may omit role_keys; hydrate details for fixtures we care about
  const empItems = [];
  for (const e of listed) {
    try {
      const detail = await apiOk(cookie, `/api/v1/digital-employees/${e.id}`);
      empItems.push({ ...e, ...detail, role_keys: detail.role_keys ?? e.role_keys ?? [] });
    } catch {
      empItems.push(e);
    }
  }
  const byName = (name) => empItems.find((e) => e.name === name);
  const byRole = (role) =>
    empItems.find((e) => (e.role_keys ?? []).includes(role));

  const developer =
    (process.env.SUPERTEAM_EMP_DEVELOPER &&
      empItems.find((e) => e.id === process.env.SUPERTEAM_EMP_DEVELOPER)) ||
    byName("开发-A") ||
    byRole("developer");
  const reviewer =
    (process.env.SUPERTEAM_EMP_REVIEWER &&
      empItems.find((e) => e.id === process.env.SUPERTEAM_EMP_REVIEWER)) ||
    byName("审查-A") ||
    byRole("reviewer");
  const tester =
    (process.env.SUPERTEAM_EMP_TESTER &&
      empItems.find((e) => e.id === process.env.SUPERTEAM_EMP_TESTER)) ||
    byName("测试-A") ||
    byRole("tester");

  const roles = await apiOk(cookie, "/api/v1/role-vocabulary");
  const roleItems = Array.isArray(roles) ? roles : roles?.items ?? [];

  return {
    project,
    projectId: project.id,
    employees: empItems,
    developer,
    reviewer,
    tester,
    roles: roleItems,
  };
}
