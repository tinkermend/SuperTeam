export const TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_OPTIONS = [5, 10] as const;
export type TeamDigitalEmployeePageSize =
  (typeof TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_OPTIONS)[number];

export const DEFAULT_TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE: TeamDigitalEmployeePageSize = 5;

/** 团队详情「数字员工」每页条数；清浏览器存储前保持用户选择。 */
export const TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY =
  "superteam.team-detail.digital-employees.page-size";

export function isTeamDigitalEmployeePageSize(
  value: unknown,
): value is TeamDigitalEmployeePageSize {
  return value === 5 || value === 10;
}

export function readTeamDigitalEmployeePageSize(
  storage: Pick<Storage, "getItem"> | null = typeof localStorage === "undefined"
    ? null
    : localStorage,
): TeamDigitalEmployeePageSize {
  if (!storage) {
    return DEFAULT_TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE;
  }
  try {
    const raw = Number(storage.getItem(TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY));
    if (isTeamDigitalEmployeePageSize(raw)) {
      return raw;
    }
  } catch {
    // private mode / blocked storage
  }
  return DEFAULT_TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE;
}

export function writeTeamDigitalEmployeePageSize(
  size: TeamDigitalEmployeePageSize,
  storage: Pick<Storage, "setItem"> | null = typeof localStorage === "undefined"
    ? null
    : localStorage,
): void {
  if (!storage || !isTeamDigitalEmployeePageSize(size)) {
    return;
  }
  try {
    storage.setItem(TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_KEY, String(size));
  } catch {
    // private mode / blocked storage
  }
}
