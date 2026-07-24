import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { Settings, Trash2 } from "lucide-react";
import {
  ActionMenu,
  CardGridSkeleton,
  DescriptionList,
  DetailSkeleton,
  EntityCard,
  FactRow,
  IconTile,
  ObjectHeader,
  StatusPill,
  TableSkeleton,
  Button,
} from "@/components/superteam";

describe("patterns · FactRow / EntityCard", () => {
  it("FactRow 渲染标签与值", async () => {
    const { getByText } = await render(
      <FactRow label="成员" value="12" />,
    );
    await expect.element(getByText("成员")).toBeInTheDocument();
    await expect.element(getByText("12")).toBeInTheDocument();
  });

  it("EntityCard 渲染标题、状态、事实与选中态", async () => {
    const { getByText } = await render(
      <EntityCard
        selected
        interactive
        leading={<IconTile tone="brand">A</IconTile>}
        title="研发一组"
        subtitle="工程团队"
        status={<StatusPill tone="ok">运行中</StatusPill>}
        facts={[
          { label: "成员", value: "8" },
          { label: "ID", value: "abc-123", mono: true },
        ]}
      />,
    );
    const card = getByText("研发一组");
    // climb to entity-card
    const root = card.element().closest("[data-slot='entity-card']");
    expect(root).not.toBeNull();
    expect(root?.getAttribute("data-selected")).toBe("true");
    await expect.element(getByText("运行中")).toBeInTheDocument();
    await expect.element(getByText("成员")).toBeInTheDocument();
    await expect.element(getByText("8")).toBeInTheDocument();
  });

  it("EntityCard actions 点击不冒泡到卡", async () => {
    const onCard = vi.fn();
    const onAction = vi.fn();
    const { getByRole } = await render(
      <EntityCard
        title="项目 A"
        onClick={onCard}
        interactive
        actions={
          <Button type="button" size="sm" variant="outline" onClick={onAction}>
            配置
          </Button>
        }
      />,
    );
    await userEvent.click(getByRole("button", { name: "配置" }));
    expect(onAction).toHaveBeenCalledOnce();
    expect(onCard).not.toHaveBeenCalled();
  });
});

describe("patterns · ObjectHeader / DescriptionList", () => {
  it("ObjectHeader 渲染名、状态、主操作", async () => {
    const { getByRole, getByText } = await render(
      <ObjectHeader
        leading={<IconTile>E</IconTile>}
        title="数字员工甲"
        subtitle="代码助手"
        status={<StatusPill tone="info">就绪</StatusPill>}
        meta={<span>Claude</span>}
        actions={<Button size="sm">编辑</Button>}
      />,
    );
    await expect.element(getByRole("heading", { name: "数字员工甲" })).toBeInTheDocument();
    await expect.element(getByText("就绪")).toBeInTheDocument();
    await expect.element(getByText("Claude")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "编辑" })).toBeInTheDocument();
  });

  it("DescriptionList 双列渲染键值", async () => {
    const { getByText } = await render(
      <DescriptionList
        columns={2}
        items={[
          { label: "负责人", value: "张三" },
          { label: "标识", value: "proj-1", mono: true },
          { label: "说明", value: "长文本", fullWidth: true },
        ]}
      />,
    );
    await expect.element(getByText("负责人")).toBeInTheDocument();
    await expect.element(getByText("张三")).toBeInTheDocument();
    await expect.element(getByText("proj-1")).toBeInTheDocument();
  });
});

describe("patterns · ActionMenu / Skeleton", () => {
  it("ActionMenu 打开并触发项", async () => {
    const onDelete = vi.fn();
    const { getByRole } = await render(
      <ActionMenu
        label="更多操作"
        items={[
          { key: "settings", label: "设置", icon: <Settings />, onSelect: vi.fn() },
          {
            key: "delete",
            label: "删除",
            icon: <Trash2 />,
            destructive: true,
            separatorBefore: true,
            onSelect: onDelete,
          },
        ]}
      />,
    );
    await userEvent.click(getByRole("button", { name: "更多操作" }));
    await userEvent.click(getByRole("menuitem", { name: "删除" }));
    expect(onDelete).toHaveBeenCalledOnce();
  });

  it("Skeleton 面带 aria-busy", async () => {
    const { getByLabelText } = await render(
      <div>
        <TableSkeleton rows={2} cols={3} />
        <CardGridSkeleton count={2} />
        <DetailSkeleton />
      </div>,
    );
    await expect.element(getByLabelText("表格加载中")).toHaveAttribute("aria-busy", "true");
    await expect.element(getByLabelText("卡片列表加载中")).toHaveAttribute("aria-busy", "true");
    await expect.element(getByLabelText("详情加载中")).toHaveAttribute("aria-busy", "true");
  });
});
