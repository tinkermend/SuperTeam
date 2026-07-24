import { beforeEach, describe, expect, it } from "vitest";
import {
  DEFAULT_TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE,
  readTeamDigitalEmployeePageSize,
  TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY,
  writeTeamDigitalEmployeePageSize,
  type TeamDigitalEmployeePageSize,
} from "./team-digital-employees-page-size";

function memoryStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return {
    getItem(key: string) {
      return Object.prototype.hasOwnProperty.call(data, key) ? data[key] : null;
    },
    setItem(key: string, value: string) {
      data[key] = value;
    },
    snapshot: () => ({ ...data }),
  };
}

describe("team digital employees page size", () => {
  beforeEach(() => {
    localStorage.removeItem(TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY);
  });

  it("defaults to 5 when storage is empty or invalid", () => {
    expect(readTeamDigitalEmployeePageSize(memoryStorage())).toBe(
      DEFAULT_TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE,
    );
    expect(readTeamDigitalEmployeePageSize(memoryStorage({ [TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY]: "20" }))).toBe(
      5,
    );
  });

  it("persists and restores 5 or 10 across reads", () => {
    const storage = memoryStorage();
    writeTeamDigitalEmployeePageSize(10, storage);
    expect(storage.snapshot()[TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY]).toBe("10");
    expect(readTeamDigitalEmployeePageSize(storage)).toBe(10);

    writeTeamDigitalEmployeePageSize(5, storage);
    expect(readTeamDigitalEmployeePageSize(storage)).toBe(5);
  });

  it("ignores unsupported page sizes when writing", () => {
    const storage = memoryStorage();
    writeTeamDigitalEmployeePageSize(20 as unknown as TeamDigitalEmployeePageSize, storage);
    expect(storage.snapshot()).toEqual({});
  });
});
