import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { StaffGapDialog, type StaffGapDialogProps } from "./staff-gap-dialog";
import type { DigitalEmployee, DigitalEmployeeAvatarAsset } from "@/lib/api/employees";
import type { EmployeeTemplate } from "@/lib/api/employee-templates";
import type { ProjectDecisionRequest, ProjectMember } from "@/lib/api/projects";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

vi.mock("@/lib/api/employees", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/employees")>();
  return {
    ...actual,
    createDigitalEmployee: vi.fn(),
    listDigitalEmployeeAvatarAssets: vi.fn(),
  };
});

vi.mock("@/lib/api/employee-templates", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/employee-templates")>();
  return {
    ...actual,
    listEmployeeTemplates: vi.fn(),
  };
});

vi.mock("@/lib/api/projects", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/projects")>();
  return {
    ...actual,
    getProject: vi.fn(),
    listProjectMembers: vi.fn(),
    replaceProjectMembers: vi.fn(),
    resolveProjectDecision: vi.fn(),
  };
});

const { createDigitalEmployee, listDigitalEmployeeAvatarAssets } = await import(
  "@/lib/api/employees"
);
const { listEmployeeTemplates } = await import("@/lib/api/employee-templates");
const { getProject, listProjectMembers, replaceProjectMembers, resolveProjectDecision } =
  await import("@/lib/api/projects");

const codeReviewerTemplate: EmployeeTemplate = {
  capability_bindings: {
    environment_variable_refs: [],
    external_capabilities: ["code_review"],
    mcp_servers: [],
    skills: [],
  },
  created_at: "2026-07-01T00:00:00Z",
  default_role: "代码审查",
  description: "独立评审代码变更",
  id: "template-code-reviewer",
  is_system: true,
  label: "标准代码审查员",
  metadata: {},
  recommended_mcp_servers: [],
  recommended_provider_types: ["claude-code"],
  recommended_skills: [],
  status: "active",
  tenant_id: "tenant-1",
  type: "standard_code_reviewer",
  updated_at: "2026-07-01T00:00:00Z",
};

const testerTemplate: EmployeeTemplate = {
  ...codeReviewerTemplate,
  capability_bindings: {
    ...codeReviewerTemplate.capability_bindings,
    external_capabilities: ["test_execution"],
  },
  default_role: "测试",
  id: "template-tester",
  label: "标准测试员",
  type: "standard_tester",
};

const avatarAsset: DigitalEmployeeAvatarAsset = {
  age_range: "26-32",
  gender: "female",
  id: "avatar-1",
  image_url: "/images/avatar-1.webp",
  label: "工程师头像",
  license: "internal_product_asset",
  source: "ai_generated_internal_pack",
  status: "active",
  style: "photorealistic_2d",
  thumbnail_url: "/images/avatar-1-256.webp",
};

const createdEmployee: DigitalEmployee = {
  approval_policy: {},
  context_policy: {},
  employee_type: "standard_code_reviewer",
  id: "employee-new-1",
  name: "审查员-ab12",
  owner_user_id: "owner-1",
  permission_policy: {},
  provider_type: "claude-code",
  risk_level: "medium",
  role: "代码审查",
  status: "ready",
  tenant_id: "tenant-1",
};

const existingMember: ProjectMember = {
  display_name_snapshot: "负责人",
  id: "member-owner",
  principal_id: "owner-1",
  principal_type: "human_user",
  project_id: "project-1",
  project_role: "owner",
  settings: {},
  status: "active",
  tenant_id: "tenant-1",
};

const resolvedDecision: ProjectDecisionRequest = {
  approval_request_id: "approval-1",
  decision_type: "planning_gap",
  id: "decision-1",
  project_id: "project-1",
  status_snapshot: "rejected",
  target_user_id: "owner-1",
  tenant_id: "tenant-1",
  title_snapshot: "规划缺口：项目员工池无法满足审查独立性约束",
};

function renderDialog(props: Partial<StaffGapDialogProps> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <StaffGapDialog
        apiOptions={{ baseUrl: "http://control-plane.local" }}
        decisionRequestId="decision-1"
        gap={{
          active_executor_count: 1,
          constraint_kind: "role_independence",
          options: ["restaff", "exempt", "lending"],
          required_capabilities: ["code_review"],
          roles: ["reviewer", "developer"],
        }}
        onOpenChange={vi.fn()}
        onStaffed={vi.fn()}
        open
        projectId="project-1"
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe("StaffGapDialog", () => {
  beforeEach(() => {
    // 补员员工必须带团队归属（参与门禁）：默认项目已绑定团队。
    vi.mocked(getProject).mockResolvedValue({ id: "project-1", team_id: "team-1" } as never);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("blocks restaffing with an explicit error when the project has no team", async () => {
    vi.mocked(listEmployeeTemplates).mockResolvedValue([codeReviewerTemplate, testerTemplate]);
    vi.mocked(listDigitalEmployeeAvatarAssets).mockResolvedValue([avatarAsset]);
    vi.mocked(getProject).mockResolvedValue({ id: "project-1" } as never);
    const screen = await renderDialog();

    await vi.waitFor(() => {
      expect(listEmployeeTemplates).toHaveBeenCalled();
    });
    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));

    await expect.element(screen.getByText("项目未绑定团队，无法补员：请先为项目绑定团队")).toBeVisible();
    expect(createDigitalEmployee).not.toHaveBeenCalled();
    expect(replaceProjectMembers).not.toHaveBeenCalled();
  });

  it("preselects the template matching gap.required_capabilities and submits create → members → resolve in order", async () => {
    vi.mocked(listEmployeeTemplates).mockResolvedValue([codeReviewerTemplate, testerTemplate]);
    vi.mocked(listDigitalEmployeeAvatarAssets).mockResolvedValue([avatarAsset]);
    vi.mocked(createDigitalEmployee).mockResolvedValue(createdEmployee);
    vi.mocked(listProjectMembers).mockResolvedValue([existingMember]);
    vi.mocked(replaceProjectMembers).mockResolvedValue([existingMember]);
    vi.mocked(resolveProjectDecision).mockResolvedValue(resolvedDecision);
    const onOpenChange = vi.fn();
    const onStaffed = vi.fn();

    const screen = await renderDialog({ onOpenChange, onStaffed });

    // code_review → standard_code_reviewer 自动预选
    await expect.element(screen.getByText("标准代码审查员")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));

    await vi.waitFor(() => {
      expect(resolveProjectDecision).toHaveBeenCalled();
    });

    expect(createDigitalEmployee).toHaveBeenCalledWith(
      { baseUrl: "http://control-plane.local" },
      expect.objectContaining({
        avatar_asset_id: "avatar-1",
        employee_type: "standard_code_reviewer",
        provider_type: "claude-code",
        role: "代码审查",
        team_id: "team-1",
      }),
    );
    expect(listProjectMembers).toHaveBeenCalledWith(
      { baseUrl: "http://control-plane.local" },
      "project-1",
    );
    expect(replaceProjectMembers).toHaveBeenCalledWith(
      { baseUrl: "http://control-plane.local" },
      "project-1",
      expect.arrayContaining([
        expect.objectContaining({
          principal_id: "owner-1",
          principal_type: "human_user",
        }),
        expect.objectContaining({
          principal_id: "employee-new-1",
          principal_type: "digital_employee",
          project_role: "executor",
        }),
      ]),
    );
    expect(resolveProjectDecision).toHaveBeenCalledWith(
      { baseUrl: "http://control-plane.local" },
      "project-1",
      "decision-1",
      expect.objectContaining({ decision: "restaffed" }),
    );

    // 依次调用：create → members read → members write → resolve
    const createOrder = vi.mocked(createDigitalEmployee).mock.invocationCallOrder[0]!;
    const membersReadOrder = vi.mocked(listProjectMembers).mock.invocationCallOrder[0]!;
    const membersWriteOrder = vi.mocked(replaceProjectMembers).mock.invocationCallOrder[0]!;
    const resolveOrder = vi.mocked(resolveProjectDecision).mock.invocationCallOrder[0]!;
    expect(createOrder).toBeLessThan(membersReadOrder);
    expect(membersReadOrder).toBeLessThan(membersWriteOrder);
    expect(membersWriteOrder).toBeLessThan(resolveOrder);

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onStaffed).toHaveBeenCalled();
  });

  it("retries after a mid-chain members-PUT failure without creating a second employee", async () => {
    vi.mocked(listEmployeeTemplates).mockResolvedValue([codeReviewerTemplate, testerTemplate]);
    vi.mocked(listDigitalEmployeeAvatarAssets).mockResolvedValue([avatarAsset]);
    vi.mocked(createDigitalEmployee).mockResolvedValue(createdEmployee);
    vi.mocked(listProjectMembers).mockResolvedValue([existingMember]);
    vi.mocked(replaceProjectMembers)
      .mockRejectedValueOnce(new Error("members PUT 网络失败"))
      .mockResolvedValueOnce([existingMember]);
    vi.mocked(resolveProjectDecision).mockResolvedValue(resolvedDecision);
    const onOpenChange = vi.fn();

    const screen = await renderDialog({ onOpenChange });
    await expect.element(screen.getByText("标准代码审查员")).toBeVisible();

    // 第一次提交：创建成功、成员 PUT 失败，链条中断，弹窗保持打开并显示错误。
    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));
    await expect.element(screen.getByText("members PUT 网络失败")).toBeVisible();
    expect(resolveProjectDecision).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);

    // 重试：跳过创建（不重复建员），从成员步续跑到 resolve。
    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));
    await vi.waitFor(() => {
      expect(resolveProjectDecision).toHaveBeenCalledWith(
        { baseUrl: "http://control-plane.local" },
        "project-1",
        "decision-1",
        expect.objectContaining({ decision: "restaffed" }),
      );
    });

    expect(createDigitalEmployee).toHaveBeenCalledTimes(1);
    expect(replaceProjectMembers).toHaveBeenCalledTimes(2);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("skips the members write on retry when the employee already joined, resuming at resolve", async () => {
    vi.mocked(listEmployeeTemplates).mockResolvedValue([codeReviewerTemplate, testerTemplate]);
    vi.mocked(listDigitalEmployeeAvatarAssets).mockResolvedValue([avatarAsset]);
    vi.mocked(createDigitalEmployee).mockResolvedValue(createdEmployee);
    // 第一次读到旧成员列表；重试时成员列表已含新员工（上次 PUT 实际已生效，仅 resolve 失败）。
    const joinedMember: ProjectMember = {
      ...existingMember,
      id: "member-new",
      principal_id: createdEmployee.id,
      principal_type: "digital_employee",
      project_role: "executor",
    };
    vi.mocked(listProjectMembers)
      .mockResolvedValueOnce([existingMember])
      .mockResolvedValueOnce([existingMember, joinedMember]);
    vi.mocked(replaceProjectMembers).mockResolvedValue([existingMember, joinedMember]);
    vi.mocked(resolveProjectDecision)
      .mockRejectedValueOnce(new Error("resolve 网络失败"))
      .mockResolvedValueOnce(resolvedDecision);
    const onOpenChange = vi.fn();

    const screen = await renderDialog({ onOpenChange });
    await expect.element(screen.getByText("标准代码审查员")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));
    await expect.element(screen.getByText("resolve 网络失败")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "创建并补员" }));
    await vi.waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    expect(createDigitalEmployee).toHaveBeenCalledTimes(1);
    // 重试读到员工已在列表 → 不再重复写成员。
    expect(replaceProjectMembers).toHaveBeenCalledTimes(1);
    expect(resolveProjectDecision).toHaveBeenCalledTimes(2);
  });

  it("does not call any api when cancelled", async () => {
    vi.mocked(listEmployeeTemplates).mockResolvedValue([codeReviewerTemplate, testerTemplate]);
    vi.mocked(listDigitalEmployeeAvatarAssets).mockResolvedValue([avatarAsset]);
    const onOpenChange = vi.fn();

    const screen = await renderDialog({ onOpenChange });
    await expect.element(screen.getByText("标准代码审查员")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "取消" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(createDigitalEmployee).not.toHaveBeenCalled();
    expect(replaceProjectMembers).not.toHaveBeenCalled();
    expect(resolveProjectDecision).not.toHaveBeenCalled();
  });
});
