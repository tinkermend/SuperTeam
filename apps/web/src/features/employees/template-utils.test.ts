import { describe, expect, it } from "vitest";
import type { DigitalEmployeeTypeOption } from "@/lib/api/employees";
import { orderedEmployeeTypes } from "./template-utils";

function employeeTypeOption(
  type: string,
  overrides: Partial<DigitalEmployeeTypeOption> = {},
): DigitalEmployeeTypeOption {
  return {
    type,
    label: type,
    description: "",
    default_role: type,
    ...overrides,
  };
}

describe("orderedEmployeeTypes", () => {
  it("filters system employee types from user-facing template lists", () => {
    const ordered = orderedEmployeeTypes([
      employeeTypeOption("database_admin"),
      employeeTypeOption("custom_agent", {
        label: "自定义数字员工",
        metadata: { creation_mode: "blank_custom", system_type: true },
      }),
      employeeTypeOption("frontend_engineer"),
    ]);

    expect(ordered.map((item) => item.type)).toEqual(["frontend_engineer", "database_admin"]);
  });
});
