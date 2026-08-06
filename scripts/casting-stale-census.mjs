/**
 * 编制失真行普查（收口批风险缓解）：
 * 编制行指向的员工不持有该 role_key，或员工不可用（非 active/ready / 已删）。
 *
 *   node scripts/casting-stale-census.mjs
 *   DATABASE_URL=postgres://... node scripts/casting-stale-census.mjs
 *
 * 默认读 apps/control-plane/config/config.yaml 的 postgres.url（需本机 psql）。
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..");
const OUT_DIR = join(ROOT, ".scratch");
mkdirSync(OUT_DIR, { recursive: true });

function loadDatabaseURL() {
  if (process.env.DATABASE_URL) return process.env.DATABASE_URL;
  const cfg = readFileSync(join(ROOT, "apps/control-plane/config/config.yaml"), "utf8");
  const m = cfg.match(/postgres:\s*\n\s*url:\s*"([^"]+)"/);
  if (!m) throw new Error("postgres.url not found in config.yaml; set DATABASE_URL");
  return m[1];
}

const SQL = `
SELECT
  c.project_id::text,
  p.name AS project_name,
  c.scenario_template_key,
  c.role_key,
  c.digital_employee_id::text AS employee_id,
  COALESCE(e.name, '(missing employee)') AS employee_name,
  COALESCE(e.status, 'missing') AS employee_status,
  CASE
    WHEN e.id IS NULL OR e.deleted_at IS NOT NULL THEN 'employee_missing'
    WHEN e.status NOT IN ('active', 'ready') THEN 'employee_unavailable'
    WHEN NOT EXISTS (
      SELECT 1 FROM digital_employee_roles der
      WHERE der.tenant_id = c.tenant_id
        AND der.digital_employee_id = c.digital_employee_id
        AND der.role_key = c.role_key
    ) THEN 'role_not_held'
    ELSE 'ok'
  END AS reason
FROM project_playbook_casting c
JOIN projects p ON p.id = c.project_id AND p.tenant_id = c.tenant_id
LEFT JOIN digital_employees e
  ON e.id = c.digital_employee_id AND e.tenant_id = c.tenant_id
WHERE CASE
    WHEN e.id IS NULL OR e.deleted_at IS NOT NULL THEN true
    WHEN e.status NOT IN ('active', 'ready') THEN true
    WHEN NOT EXISTS (
      SELECT 1 FROM digital_employee_roles der
      WHERE der.tenant_id = c.tenant_id
        AND der.digital_employee_id = c.digital_employee_id
        AND der.role_key = c.role_key
    ) THEN true
    ELSE false
  END
ORDER BY p.name, c.scenario_template_key, c.role_key;
`;

const url = loadDatabaseURL();
const r = spawnSync("psql", [url, "-v", "ON_ERROR_STOP=1", "-F", "\t", "-A", "-c", SQL], {
  encoding: "utf8",
  maxBuffer: 20 * 1024 * 1024,
});
if (r.status !== 0) {
  console.error(r.stderr || r.stdout);
  process.exit(r.status || 1);
}

const lines = (r.stdout || "").trim().split("\n").filter(Boolean);
// psql -A -F tab: header + rows + (N rows)
const dataLines = lines.filter((l) => !/^\(\d+ rows?\)$/.test(l));
const [header, ...rows] = dataLines;
const cols = header ? header.split("\t") : [];
const items = rows.map((line) => {
  const parts = line.split("\t");
  const obj = {};
  cols.forEach((c, i) => {
    obj[c] = parts[i];
  });
  return obj;
});

const byProject = new Map();
for (const it of items) {
  const key = `${it.project_name} (${it.project_id})`;
  if (!byProject.has(key)) byProject.set(key, []);
  byProject.get(key).push(it);
}

const report = {
  generated_at: new Date().toISOString(),
  stale_count: items.length,
  projects_affected: byProject.size,
  by_project: Object.fromEntries(
    [...byProject.entries()].map(([k, v]) => [
      k,
      v.map((x) => ({
        template: x.scenario_template_key,
        role: x.role_key,
        employee: x.employee_name,
        employee_id: x.employee_id,
        reason: x.reason,
      })),
    ]),
  ),
  items,
};

const outPath = join(OUT_DIR, "casting-stale-census.json");
writeFileSync(outPath, JSON.stringify(report, null, 2));
console.log(`stale_count=${report.stale_count} projects_affected=${report.projects_affected}`);
console.log(`wrote ${outPath}`);
if (report.stale_count > 0) {
  for (const [proj, list] of byProject) {
    console.log(`- ${proj}: ${list.length} row(s)`);
    for (const x of list.slice(0, 5)) {
      console.log(`    ${x.scenario_template_key}.${x.role_key} ← ${x.employee_name} (${x.reason})`);
    }
  }
}
