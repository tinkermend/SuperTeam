import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { toast } from "sonner";
import { SkillReplaceDialog } from "./replace-dialog";
import { replaceSkillArchive, type Skill } from "@/lib/api/skills";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), message: vi.fn() },
}));

vi.mock("@/lib/api/skills", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/skills")>();
  return { ...actual, replaceSkillArchive: vi.fn() };
});

const skill = {
  id: "skill-1",
  tenant_id: "tenant-1",
  slug: "diagnose",
  name: "diagnose",
  description: "诊断",
  version: "v0.1.0",
  source: "upload",
  risk_level: "low",
  icon_key: "stethoscope",
  color_token: "cyan",
  tags: [],
  archive_object_ref: "s3://bucket/skills/a.zip",
  archive_filename: "diagnose.zip",
  archive_size_bytes: 12,
  archive_checksum_sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  archive_file_count: 1,
  created_by: "user-1",
  created_by_name: "平台管理员",
  team_bindings: [{ team_id: "team-1", team_name: "平台" }],
  agent_bindings: [],
} satisfies Skill;

async function renderDialog() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <SkillReplaceDialog
        apiBaseUrl="http://control-plane.local"
        onOpenChange={vi.fn()}
        open
        skill={skill}
      />
    </QueryClientProvider>,
  );
}

describe("SkillReplaceDialog", () => {
  it("toasts the new version and closes after a successful replace", async () => {
    vi.mocked(replaceSkillArchive).mockResolvedValue({
      ...skill,
      version: "v0.2.0",
      archive_checksum_sha256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    });
    const screen = await renderDialog();
    await userEvent.upload(
      screen.getByLabelText("技能 zip 包"),
      new File(["zip"], "diagnose.zip", { type: "application/zip" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "上传并替换" }));
    await vi.waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith("已更新到 v0.2.0", expect.any(Object));
    });
  });

  it("toasts checksum when version does not change", async () => {
    vi.mocked(replaceSkillArchive).mockResolvedValue({
      ...skill,
      archive_checksum_sha256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    });
    const screen = await renderDialog();
    await userEvent.upload(
      screen.getByLabelText("技能 zip 包"),
      new File(["zip"], "diagnose.zip", { type: "application/zip" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "上传并替换" }));
    await vi.waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith("已替换归档，checksum cccccccc", expect.any(Object));
    });
  });
});
