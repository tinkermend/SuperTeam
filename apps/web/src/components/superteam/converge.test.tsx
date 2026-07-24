import { describe, expect, it, vi, beforeEach } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { toast } from "sonner";
import {
  AvatarStack,
  Breadcrumb,
  Button,
  ButtonGroup,
  CodeBlock,
  EmptyNoData,
  EmptyNoMatch,
  EmptyUnconfigured,
  LogLine,
  RelativeTime,
  SectionHeader,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
  notifyError,
  notifySuccess,
} from "@/components/superteam";

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
  },
}));

describe("converge · Breadcrumb / SectionHeader", () => {
  it("Breadcrumb 末项 aria-current=page", async () => {
    const onTeams = vi.fn();
    const { getByText, getByRole } = await render(
      <Breadcrumb
        items={[
          { label: "团队", onClick: onTeams },
          { label: "安全响应组" },
        ]}
      />,
    );
    await expect.element(getByText("安全响应组")).toHaveAttribute("aria-current", "page");
    await userEvent.click(getByRole("button", { name: "团队" }));
    expect(onTeams).toHaveBeenCalledOnce();
  });

  it("SectionHeader 渲染标题与动作", async () => {
    const { getByRole, getByText } = await render(
      <SectionHeader
        title="成员"
        description="人类与数字员工"
        actions={<Button size="sm">添加</Button>}
      />,
    );
    await expect.element(getByRole("heading", { name: "成员" })).toBeInTheDocument();
    await expect.element(getByText("人类与数字员工")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "添加" })).toBeInTheDocument();
  });
});

describe("converge · Empty presets / SoftTabs", () => {
  it("Empty 预设带 data-empty-kind", async () => {
    const { container } = await render(
      <div>
        <EmptyNoData />
        <EmptyNoMatch />
        <EmptyUnconfigured />
      </div>,
    );
    expect(container.querySelector('[data-empty-kind="no-data"]')).toBeTruthy();
    expect(container.querySelector('[data-empty-kind="no-match"]')).toBeTruthy();
    expect(container.querySelector('[data-empty-kind="unconfigured"]')).toBeTruthy();
  });

  it("SoftTabs 切换内容", async () => {
    const { getByRole, getByText } = await render(
      <SoftTabs defaultValue="a">
        <SoftTabsList>
          <SoftTabsTrigger value="a">概览</SoftTabsTrigger>
          <SoftTabsTrigger value="b">事件</SoftTabsTrigger>
        </SoftTabsList>
        <SoftTabsContent value="a">面板A</SoftTabsContent>
        <SoftTabsContent value="b">面板B</SoftTabsContent>
      </SoftTabs>,
    );
    await expect.element(getByText("面板A")).toBeInTheDocument();
    await userEvent.click(getByRole("tab", { name: "事件" }));
    await expect.element(getByText("面板B")).toBeInTheDocument();
  });
});

describe("converge · RelativeTime / ButtonGroup / AvatarStack / Log", () => {
  it("RelativeTime 渲染 time 与 title", async () => {
    const iso = new Date(Date.now() - 5 * 60_000).toISOString();
    const { getByText } = await render(<RelativeTime value={iso} />);
    await expect.element(getByText("5 分钟前")).toBeInTheDocument();
    const el = getByText("5 分钟前");
    await expect.element(el).toHaveAttribute("datetime", iso);
    await expect.element(el).toHaveAttribute("title");
  });

  it("ButtonGroup 包裹按钮", async () => {
    const { getByRole } = await render(
      <ButtonGroup>
        <Button variant="outline">左</Button>
        <Button>右</Button>
      </ButtonGroup>,
    );
    await expect.element(getByRole("group")).toHaveAttribute("data-slot", "button-group");
    await expect.element(getByRole("button", { name: "左" })).toBeInTheDocument();
  });

  it("AvatarStack 溢出 +N", async () => {
    const { getByText } = await render(
      <AvatarStack
        max={2}
        items={[
          { id: "1", name: "甲" },
          { id: "2", name: "乙" },
          { id: "3", name: "丙" },
        ]}
      />,
    );
    await expect.element(getByText("+1")).toBeInTheDocument();
  });

  it("LogLine / CodeBlock 渲染", async () => {
    const { getByText } = await render(
      <div>
        <LogLine time="10:00" level="ERR" tone="danger">
          boom
        </LogLine>
        <CodeBlock>const x = 1</CodeBlock>
      </div>,
    );
    await expect.element(getByText("boom")).toBeInTheDocument();
    await expect.element(getByText("const x = 1")).toBeInTheDocument();
  });
});

describe("converge · notify helpers", () => {
  beforeEach(() => {
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
  });

  it("notifySuccess / notifyError 调 sonner", () => {
    notifySuccess("已保存");
    notifyError("失败了");
    expect(toast.success).toHaveBeenCalledWith("已保存", expect.any(Object));
    expect(toast.error).toHaveBeenCalledWith("失败了", expect.any(Object));
  });
});
