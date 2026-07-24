import { describe, expect, it } from "vitest";
import {
  assetsInitialTabFromQuery,
  normalizeProjectDetailSection,
} from "./project-detail-section";

describe("project-detail-section", () => {
  it("normalizes legacy tabs onto workbench / tasks / approval / assets", () => {
    expect(normalizeProjectDetailSection(undefined)).toBe("workbench");
    expect(normalizeProjectDetailSection("overview")).toBe("workbench");
    expect(normalizeProjectDetailSection("config")).toBe("workbench");
    expect(normalizeProjectDetailSection("workbench")).toBe("workbench");
    expect(normalizeProjectDetailSection("tasks")).toBe("tasks");
    expect(normalizeProjectDetailSection("approval")).toBe("approval");
    expect(normalizeProjectDetailSection("assets")).toBe("assets");
    expect(normalizeProjectDetailSection("artifacts")).toBe("assets");
    expect(normalizeProjectDetailSection("budget")).toBe("assets");
    expect(normalizeProjectDetailSection("acceptance")).toBe("assets");
  });

  it("picks assets sub-tab from legacy query", () => {
    expect(assetsInitialTabFromQuery("artifacts")).toBe("artifacts");
    expect(assetsInitialTabFromQuery("budget")).toBe("budget");
    expect(assetsInitialTabFromQuery("acceptance")).toBe("acceptance");
    expect(assetsInitialTabFromQuery("overview")).toBe("artifacts");
  });
});
