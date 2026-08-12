import { describe, expect, it } from "vitest";
import {
  projectPhaseColorVar,
  projectPhaseDotClass,
  type ProjectLifecyclePhase,
} from "./project-lifecycle-display";

const PHASES: ProjectLifecyclePhase[] = [
  "draft",
  "configuring",
  "running",
  "acceptance",
  "paused",
  "archived",
];

describe("project-lifecycle-display", () => {
  it("六个阶段各出一次圆点 class（均走 bg-phase-*）", () => {
    for (const phase of PHASES) {
      const cls = projectPhaseDotClass(phase);
      expect(cls).toMatch(/^bg-phase-/);
      expect(cls).not.toMatch(/bg-(ok|warn|danger|info|brand|mute)/);
    }
    // 已知映射，防止 silent rename
    expect(projectPhaseDotClass("running")).toBe("bg-phase-ready");
    expect(projectPhaseDotClass("acceptance")).toBe("bg-phase-acceptance");
    expect(projectPhaseDotClass("configuring")).toBe("bg-phase-configuring");
    expect(projectPhaseDotClass("draft")).toBe("bg-phase-draft");
    expect(projectPhaseDotClass("paused")).toBe("bg-phase-paused");
    expect(projectPhaseDotClass("archived")).toBe("bg-phase-archived");
  });

  it("六个阶段各出一次 CSS 变量", () => {
    for (const phase of PHASES) {
      const v = projectPhaseColorVar(phase);
      expect(v).toMatch(/^var\(--phase-/);
    }
    expect(projectPhaseColorVar("running")).toBe("var(--phase-ready)");
    expect(projectPhaseColorVar("acceptance")).toBe("var(--phase-acceptance)");
  });

  it("未知状态回退 draft 灰点", () => {
    expect(projectPhaseDotClass("unknown")).toBe("bg-phase-draft");
    expect(projectPhaseColorVar("")).toBe("var(--phase-draft)");
    expect(projectPhaseDotClass("weird_status")).toBe("bg-phase-draft");
  });
});
