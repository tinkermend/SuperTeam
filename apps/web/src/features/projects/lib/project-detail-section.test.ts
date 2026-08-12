import { describe, expect, it } from "vitest";
import {
  assetsInitialTabFromQuery,
  normalizeProjectDetailSection,
} from "./project-detail-section";

describe("project-detail-section", () => {
  it("normalizes legacy tabs onto tasks / flow / approval / history / assets", () => {
    expect(normalizeProjectDetailSection(undefined)).toBe("tasks");
    expect(normalizeProjectDetailSection("overview")).toBe("tasks");
    expect(normalizeProjectDetailSection("config")).toBe("tasks");
    expect(normalizeProjectDetailSection("workbench")).toBe("tasks");
    expect(normalizeProjectDetailSection("tasks")).toBe("tasks");
    expect(normalizeProjectDetailSection("demands")).toBe("tasks");
    expect(normalizeProjectDetailSection("trace")).toBe("history");
    expect(normalizeProjectDetailSection("history")).toBe("history");
    expect(normalizeProjectDetailSection("flow")).toBe("flow");
    expect(normalizeProjectDetailSection("approval")).toBe("approval");
    expect(normalizeProjectDetailSection("assets")).toBe("assets");
    expect(normalizeProjectDetailSection("artifacts")).toBe("assets");
    expect(normalizeProjectDetailSection("budget")).toBe("assets");
    expect(normalizeProjectDetailSection("acceptance")).toBe("assets");
    expect(normalizeProjectDetailSection("closure")).toBe("assets");
  });

  it("picks assets sub-tab from legacy query", () => {
    expect(assetsInitialTabFromQuery("artifacts")).toBe("artifacts");
    expect(assetsInitialTabFromQuery("budget")).toBe("budget");
    expect(assetsInitialTabFromQuery("acceptance")).toBe("acceptance");
    expect(assetsInitialTabFromQuery("closure")).toBe("acceptance");
    expect(assetsInitialTabFromQuery("overview")).toBe("artifacts");
  });
});
