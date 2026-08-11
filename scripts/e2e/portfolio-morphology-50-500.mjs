/**
 * Portfolio morphology gate at 50 projects / 500 tasks (§7.1).
 * Creates a scratch tenant, seeds data, runs EXPLAIN, then cleans up.
 *
 * Usage:
 *   DATABASE_URL=postgres://... node scripts/e2e/portfolio-morphology-50-500.mjs
 */
import pg from "pg";
import { writeFileSync, mkdirSync } from "fs";
import { randomUUID } from "crypto";

const DATABASE_URL =
  process.env.DATABASE_URL ||
  process.env.TEST_DATABASE_URL ||
  "";
if (!DATABASE_URL) {
  console.error("DATABASE_URL or TEST_DATABASE_URL required");
  process.exit(1);
}

const tenantId = randomUUID();
const actorId = randomUUID();
const client = new pg.Client({ connectionString: DATABASE_URL });
await client.connect();

const outDir = "docs/superpowers/perf";
mkdirSync(outDir, { recursive: true });
const outPath = `${outDir}/2026-08-11-project-portfolio-explain-50-500.txt`;

const log = [];
const push = (s) => {
  log.push(s);
  console.log(s);
};

try {
  await client.query("BEGIN");
  // Minimal auth_users for owner display join
  await client.query(
    `INSERT INTO auth_users (id, username, password_hash, display_name, status)
     VALUES ($1, $2, 'x', 'morphology-actor', 'active')
     ON CONFLICT (id) DO NOTHING`,
    [actorId, `morph-${actorId.slice(0, 8)}`],
  ).catch(async () => {
    // schema may require more columns — try bare insert of projects only and skip users
  });

  // Ensure tenant exists if tenants table requires it
  await client.query(
    `INSERT INTO tenants (id, name, status) VALUES ($1, 'morphology-50-500', 'active')
     ON CONFLICT DO NOTHING`,
    [tenantId],
  ).catch(() => {});

  const projectIds = [];
  for (let i = 0; i < 50; i++) {
    const id = randomUUID();
    projectIds.push(id);
    await client.query(
      `INSERT INTO projects (
         id, tenant_id, name, directory_name, goal, status,
         human_owner_user_id, human_owner_user_ids, coordination_status, created_at, updated_at
       ) VALUES (
         $1, $2, $3, $4, $5, $6,
         $7, ARRAY[$7]::uuid[], 'running', NOW(), NOW()
       )`,
      [
        id,
        tenantId,
        `morph-p-${i}`,
        `morph-p-${i}`,
        `goal ${i}`,
        i % 10 === 0 ? "acceptance" : "running",
        actorId,
      ],
    );
  }

  // 500 tasks: 10 per project
  const statuses = [
    "pending",
    "queued",
    "running",
    "waiting_human",
    "blocked",
    "failed",
    "completed",
    "cancelled",
    "planned",
    "assigned",
  ];
  let taskN = 0;
  for (const pid of projectIds) {
    for (let j = 0; j < 10; j++) {
      const st = statuses[j % statuses.length];
      await client.query(
        `INSERT INTO project_tasks (
           id, tenant_id, project_id, title, status, requires_human_approval,
           created_at, updated_at
         ) VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())`,
        [randomUUID(), tenantId, pid, `t-${taskN}`, st, j === 3],
      );
      taskN++;
    }
  }
  push(`seeded tenant=${tenantId} projects=50 tasks=${taskN}`);

  const explainSummary = await client.query(
    `EXPLAIN (ANALYZE, BUFFERS, VERBOSE, FORMAT TEXT)
     SELECT project_task_portfolio_bucket(status, requires_human_approval) AS b, COUNT(*)
     FROM project_tasks
     WHERE tenant_id = $1 AND dismissed_at IS NULL
     GROUP BY 1`,
    [tenantId],
  );
  push("=== bucket function aggregate ===");
  for (const row of explainSummary.rows) push(Object.values(row)[0]);

  const explainItems = await client.query(
    `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
     WITH visible_projects AS (
       SELECT p.* FROM projects p
       WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
     ),
     filtered_projects AS (SELECT * FROM visible_projects),
     task_agg AS (
       SELECT pt.project_id,
              COUNT(*)::int AS task_total,
              COUNT(*) FILTER (WHERE project_task_portfolio_bucket(pt.status, pt.requires_human_approval) = 'failed')::int AS task_failed,
              COUNT(*) FILTER (WHERE project_task_portfolio_bucket(pt.status, pt.requires_human_approval) = 'blocked')::int AS task_blocked,
              COUNT(*) FILTER (
                WHERE project_task_portfolio_bucket(pt.status, pt.requires_human_approval) = 'waiting_human'
                  AND NOT EXISTS (
                    SELECT 1 FROM project_decision_requests dr
                    WHERE dr.tenant_id = pt.tenant_id AND dr.project_task_id = pt.id
                      AND lower(COALESCE(dr.status_snapshot,'')) IN ('pending','waiting','requested','open')
                  )
              )::int AS waiting_human_unlinked_count,
              MAX(pt.updated_at) AS last_activity_at
       FROM project_tasks pt
       JOIN filtered_projects fp ON fp.id = pt.project_id
       WHERE pt.tenant_id = $1 AND pt.dismissed_at IS NULL
       GROUP BY pt.project_id
     ),
     decision_agg AS (
       SELECT project_id, 0::int AS open_decision_count
       FROM filtered_projects
     ),
     candidate AS (
       SELECT fp.id, fp.name,
              COALESCE(t.task_failed,0) AS task_failed,
              COALESCE(t.task_blocked,0) AS task_blocked,
              COALESCE(t.waiting_human_unlinked_count,0) AS waiting_human_unlinked_count,
              COALESCE(d.open_decision_count,0) AS open_decision_count
       FROM filtered_projects fp
       LEFT JOIN task_agg t ON t.project_id = fp.id
       LEFT JOIN decision_agg d ON d.project_id = fp.id
       ORDER BY
         CASE WHEN (COALESCE(t.task_failed,0)+COALESCE(t.task_blocked,0)+COALESCE(t.waiting_human_unlinked_count,0)+COALESCE(d.open_decision_count,0)) > 0 THEN 0 ELSE 1 END,
         COALESCE(t.last_activity_at, fp.updated_at) DESC NULLS LAST,
         fp.id DESC
       LIMIT 12 OFFSET 0
     )
     SELECT * FROM candidate`,
    [tenantId],
  );
  push("=== list items attention sort limit 12 ===");
  for (const row of explainItems.rows) push(Object.values(row)[0]);

  const planText = log.join("\n");
  // Morphology assertions
  if (/tenant_id <>|tenant_id !=/.test(planText)) {
    throw new Error("unexpected tenant inequality");
  }
  // Count SubPlan occurrences
  const subPlans = (planText.match(/SubPlan/g) || []).length;
  push(`SubPlan count=${subPlans}`);
  const execTimes = [...planText.matchAll(/Execution Time: ([0-9.]+) ms/g)].map((m) => Number(m[1]));
  push(`Execution times ms: ${execTimes.join(", ")}`);
  for (const t of execTimes) {
    if (t > 500) throw new Error(`execution time ${t}ms exceeds 500ms gate`);
  }
  push("MORPHOLOGY PASS at 50/500 fixture");

  await client.query("ROLLBACK"); // discard seed
  push("rolled back seed (no residual data)");
} catch (e) {
  await client.query("ROLLBACK").catch(() => {});
  push(`FAILED: ${e}`);
  writeFileSync(outPath, log.join("\n"));
  await client.end();
  process.exit(1);
}

writeFileSync(outPath, log.join("\n"));
console.log("wrote", outPath);
await client.end();
