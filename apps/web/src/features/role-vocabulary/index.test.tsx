import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { RoleVocabularyView } from "@/features/role-vocabulary";
import {
  createRoleVocabulary,
  getRoleVocabularyReferences,
  listRoleVocabulary,
  type RoleVocabularyEntry,
} from "@/lib/api/casting";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

vi.mock("@/lib/api/casting", () => ({
  listRoleVocabulary: vi.fn(),
  createRoleVocabulary: vi.fn(),
  patchRoleVocabulary: vi.fn(),
  getRoleVocabularyReferences: vi.fn(),
}));

const developer: RoleVocabularyEntry = {
  id: "role-1",
  tenant_id: "tenant-1",
  role_key: "developer",
  title: "开发",
  description: "实现代码",
  status: "active",
  created_at: "2026-08-05T00:00:00Z",
  updated_at: "2026-08-05T00:00:00Z",
};

async function withClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe("Role vocabulary page", () => {
  it("renders existing roles, reference counts, and opens create dialog", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue([developer]);
    vi.mocked(getRoleVocabularyReferences).mockResolvedValue({
      scenario_templates: [{ key: "software_delivery", name: "软件开发" }],
      employees: [{ id: "emp-1", name: "开发-A" }],
      employee_count: 1,
      casting_count: 2,
    });

    const screen = await withClient(
      <RoleVocabularyView apiBaseUrl="http://control-plane.local" />,
    );

    await expect.element(screen.getByText("角色词表")).toBeVisible();
    await expect.element(screen.getByText("开发")).toBeVisible();
    await expect.element(screen.getByText("developer")).toBeVisible();
    await expect.element(screen.getByText("被引用")).toBeVisible();
    await expect.element(screen.getByText("剧本 1")).toBeVisible();
    await expect.element(screen.getByText("员工 1")).toBeVisible();
    await expect.element(screen.getByText("编制 2")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: /新建角色/ }));
    await expect.element(screen.getByText("role_key 创建后不可改")).toBeVisible();
  });

  it("shows reference impact before disable", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue([developer]);
    vi.mocked(getRoleVocabularyReferences).mockResolvedValue({
      scenario_templates: [{ key: "software_delivery", name: "软件开发" }],
      employees: [{ id: "emp-1", name: "开发-A" }],
      employee_count: 1,
      casting_count: 3,
    });

    const screen = await withClient(
      <RoleVocabularyView apiBaseUrl="http://control-plane.local" />,
    );

    await userEvent.click(screen.getByRole("button", { name: "停用" }));
    await expect.element(screen.getByText(/软件开发/)).toBeVisible();
    await expect.element(screen.getByText(/开发-A/)).toBeVisible();
    await expect.element(screen.getByText(/3 条/)).toBeVisible();
  });

  it("rejects invalid role_key on create", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue([]);
    vi.mocked(createRoleVocabulary).mockResolvedValue(developer);

    const screen = await withClient(
      <RoleVocabularyView apiBaseUrl="http://control-plane.local" />,
    );

    await userEvent.click(screen.getByRole("button", { name: /新建角色/ }));
    const keyInput = screen.getByLabelText("role_key");
    await userEvent.fill(keyInput, "Network Diagnostics");
    await userEvent.fill(screen.getByLabelText("中文名"), "网络诊断");
    await userEvent.click(screen.getByRole("button", { name: "创建" }));

    await expect
      .element(screen.getByText(/role_key 须为小写字母开头/))
      .toBeVisible();
    expect(createRoleVocabulary).not.toHaveBeenCalled();
  });
});
