import { userEvent } from "vitest/browser";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import type { CapabilityProjectionSnapshot } from "@/lib/api/projects";
import { AttemptCapabilityProjection } from "./attempt-capability-projection";

const baseSnap = (overrides: Partial<CapabilityProjectionSnapshot> = {}): CapabilityProjectionSnapshot => ({
  available: true,
  skills: [
    {
      skill_id: "11111111-1111-4111-8111-111111111111",
      skill_key: "linux",
      skill_name: "Linux 排障",
      source_scope: "project",
      version: "2",
    },
  ],
  mcp_servers: [
    {
      server_id: "22222222-2222-4222-8222-222222222222",
      server_key: "github-mcp",
      server_name: "GitHub",
      source_scope: "dependency_closure",
    },
  ],
  skill_conflicts: [
    {
      slug: "linux",
      source: "project_binding",
      winning_source: "project",
      dropped_source: "employee",
    },
  ],
  summary: {
    skill_count: 1,
    mcp_count: 1,
    conflict_count: 1,
    by_source: { project: 1, dependency_closure: 1 },
  },
  ...overrides,
});

describe("AttemptCapabilityProjection", () => {
  it("shows unavailable copy when snapshot is not available", async () => {
    const screen = await render(
      <AttemptCapabilityProjection
        projection={baseSnap({
          available: false,
          skills: [],
          mcp_servers: [],
          skill_conflicts: [],
          summary: { skill_count: 0, mcp_count: 0, conflict_count: 0, by_source: {} },
        })}
      />,
    );
    await expect
      .element(screen.getByTestId("attempt-capability-projection"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("本次尝试无能力投影快照")).toBeInTheDocument();
  });

  it("expands to show sources, dependency closure, and conflict sentence", async () => {
    const screen = await render(<AttemptCapabilityProjection projection={baseSnap()} />);
    const root = screen.getByTestId("attempt-capability-projection");

    await expect.element(screen.getByText("技能 1 · MCP 1 · 冲突 1")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("attempt-capability-projection-expand"));

    // Wait until expanded content mounts.
    await expect.element(screen.getByText("Linux 排障")).toBeInTheDocument();
    await expect
      .element(screen.getByText("「linux」保留项目级，覆盖员工"))
      .toBeInTheDocument();

    const body = root.element().textContent ?? "";
    for (const token of ["GitHub", "github-mcp", "依赖补全", "项目级", "员工"]) {
      expect(body).toContain(token);
    }
  });

  it("renders empty projection copy", async () => {
    const screen = await render(
      <AttemptCapabilityProjection
        projection={baseSnap({
          skills: [],
          mcp_servers: [],
          skill_conflicts: [],
          summary: { skill_count: 0, mcp_count: 0, conflict_count: 0, by_source: {} },
        })}
      />,
    );
    await expect.element(screen.getByText("未投影任何技能或 MCP")).toBeInTheDocument();
  });
});
