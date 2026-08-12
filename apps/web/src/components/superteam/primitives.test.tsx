import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import {
  IconTile,
  SignatureCard,
  SoftCard,
  StatusPill,
  Button,
  Chip,
  EmptyState,
  ErrorState,
  IconButton,
  LoadingState,
  MetricCard,
  Pagination,
  PermissionDenied,
  Segmented,
  StateSurface,
  PageTab,
  PageTabList,
  DataTable,
  PageTabs,
  Td,
  Th,
  ToolbarSearch,
  Tr,
  WorkSurface
} from "./primitives";

/**
 * 仿真测试：用本仓库真实浏览器测试栈（vitest-browser-react）挂载渲染 v3 组件族，
 * 验证其组合正确、应用 v3 token 派生 class，且各覆盖态与交互可被正确驱动。
 * 这是“v3 参考实现可用性”的回归护栏，不是业务测试。
 */

describe("v3 组件族 · 基础渲染", () => {
  it("SoftCard 应用 v3 容器 class", async () => {
    const { getByText } = await render(<SoftCard>技能概览</SoftCard>);
    const card = getByText("技能概览");
    await expect.element(card).toHaveAttribute("data-slot", "soft-card");
    await expect.element(card).toHaveClass("rounded-card");
    await expect.element(card).toHaveClass("bg-card");
    await expect.element(card).toHaveClass("shadow-sm");
  });

  it("IconTile 应用 tone 与柔色背景 class", async () => {
    const { getByLabelText } = await render(
      <IconTile tone="danger" size="lg" aria-label="危险图标">
        <span>x</span>
      </IconTile>,
    );
    const tile = getByLabelText("危险图标");
    await expect.element(tile).toHaveAttribute("data-slot", "icon-tile");
    await expect.element(tile).toHaveClass("text-danger");
    await expect.element(tile).toHaveClass("bg-danger-soft");
  });

  it("StatusPill 渲染圆点与文案，应用语义色", async () => {
    const { getByText } = await render(<StatusPill tone="warn">预警</StatusPill>);
    const pill = getByText("预警");
    await expect.element(pill).toHaveAttribute("data-slot", "status-pill");
    await expect.element(pill).toHaveClass("text-warn-text");
    await expect.element(pill).toHaveClass("bg-warn-soft");
  });

  it("StatusPill.dotClassName 覆盖圆点为 phase 分类色，不占用 tone 语义底", async () => {
    const { getByText, container } = await render(
      <StatusPill tone="mute" dotClassName="bg-phase-acceptance">
        验收中
      </StatusPill>,
    );
    const pill = getByText("验收中");
    await expect.element(pill).toHaveClass("bg-mute-soft");
    await expect.element(pill).toHaveClass("text-mute-text");
    const dot = container.querySelector('[data-slot="status-pill"] > span[aria-hidden]');
    expect(dot?.className).toContain("bg-phase-acceptance");
    // 圆点走了 dotClassName 就不再取 tone 的 solid 底（此处 tone=mute）。
    expect(dot?.className).not.toContain("bg-mute");
  });

  it("dotClassName 缺省时圆点回落 tone 的 solid 底", async () => {
    const { container } = await render(<StatusPill tone="danger">失败</StatusPill>);
    const dot = container.querySelector('[data-slot="status-pill"] > span[aria-hidden]');
    expect(dot?.className).toContain("bg-danger");
    expect(dot?.className).not.toContain("bg-phase-");
  });

  it("MetricCard 渲染标签、大数字；loud 时数值跟随 iconTone", async () => {
    const { getByText } = await render(
      <MetricCard label="待审批" value="4" loud iconTone="warn" meta="2 个高风险" />,
    );
    await expect.element(getByText("待审批")).toBeInTheDocument();
    await expect.element(getByText("4")).toHaveClass("text-warn");
    await expect.element(getByText("2 个高风险")).toBeInTheDocument();
  });

  it("MetricCard loud + danger 时数值点亮为 danger", async () => {
    const { getByText } = await render(
      <MetricCard label="高风险" value="3" loud iconTone="danger" />,
    );
    await expect.element(getByText("3")).toHaveClass("text-danger");
  });

  it("SignatureCard 渲染并写入蓝色渐变 style", async () => {
    const { getByText } = await render(<SignatureCard>signature</SignatureCard>);
    const sig = getByText("signature");
    await expect.element(sig).toHaveAttribute("data-slot", "signature-card");
    await expect.element(sig).toHaveAttribute("style");
  });
});

describe("v3 数据面 · 表格", () => {
  it("DataTable 表头与 danger 行可组合渲染，danger 行带 data-tone 标记", async () => {
    const { getByRole, getByText } = await render(
      <WorkSurface>
        <DataTable>
          <thead>
            <tr>
              <Th>事项</Th>
            </tr>
          </thead>
          <tbody>
            <Tr tone="danger">
              <Td>生产发布</Td>
            </Tr>
          </tbody>
        </DataTable>
      </WorkSurface>,
    );
    await expect.element(getByText("事项")).toBeInTheDocument();
    await expect.element(getByText("生产发布")).toBeInTheDocument();
    const row = getByRole("row", { name: /生产发布/ });
    await expect.element(row).toHaveAttribute("data-tone", "danger");
  });

  it("Tr 的 warn 行使用 warning 软底和左侧 accent", async () => {
    const { getByRole } = await render(
      <DataTable>
        <tbody>
          <Tr tone="warn">
            <Td>运行依赖缺失</Td>
          </Tr>
        </tbody>
      </DataTable>,
    );
    const row = getByRole("row", { name: /运行依赖缺失/ });
    await expect.element(row).toHaveAttribute("data-tone", "warn");
    await expect.element(row).toHaveClass("[&>td]:bg-warn-soft");
    await expect
      .element(row)
      .toHaveClass("[&>td:first-child]:shadow-[inset_3px_0_0_var(--warn)]");
  });
});

describe("v3 交互组件", () => {
  it("Button 记录 variant，文案可点", async () => {
    const onClick = vi.fn();
    const { getByRole } = await render(
      <Button variant="danger" onClick={onClick}>
        处理
      </Button>,
    );
    const btn = getByRole("button", { name: "处理" });
    await expect.element(btn).toHaveAttribute("data-variant", "danger");
    await userEvent.click(btn);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("Chip 选中态切换到蓝柔底", async () => {
    const { getByText } = await render(
      <Chip active type="button">全部</Chip>,
    );
    await expect.element(getByText("全部")).toHaveClass("bg-brand-soft");
    await expect.element(getByText("全部")).toHaveAttribute("data-slot", "chip");
  });

  it("Segmented 点击回传所选值", async () => {
    const onChange = vi.fn();
    const { getByRole } = await render(
      <Segmented
        value="comfortable"
        onChange={onChange}
        options={[
          { label: "舒适", value: "comfortable" },
          { label: "紧凑", value: "compact" },
        ]}
      />,
    );
    await userEvent.click(getByRole("button", { name: "紧凑" }));
    expect(onChange).toHaveBeenCalledWith("compact");
  });

  it("Pagination 点击下一页回传页码", async () => {
    const onPageChange = vi.fn();
    const { getByRole } = await render(
      <Pagination
        total={42}
        page={1}
        pageSize={10}
        pageCount={5}
        onPageChange={onPageChange}
      />,
    );
    await userEvent.click(getByRole("button", { name: "下一页" }));
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it("ToolbarSearch 渲染可输入搜索框", async () => {
    const { getByRole } = await render(<ToolbarSearch placeholder="搜索…" />);
    const input = getByRole("textbox");
    await expect.element(input).toBeInTheDocument();
    await expect.element(input).toHaveAttribute("placeholder", "搜索…");
  });

  it("PageTabs 组合渲染，激活 Tab 带蓝柔底", async () => {
    const { getByRole } = await render(
      <PageTabs>
        <PageTabList>
          <PageTab active>任务</PageTab>
          <PageTab>审计</PageTab>
        </PageTabList>
      </PageTabs>,
    );
    await expect
      .element(getByRole("button", { name: "任务" }))
      .toHaveClass("bg-brand-soft");
    await expect.element(getByRole("button", { name: "审计" })).toBeInTheDocument();
  });
});

describe("v3 覆盖态", () => {
  it("StateSurface: loading 渲染加载态", async () => {
    const { getByText } = await render(
      <StateSurface isLoading>
        <span>不应出现</span>
      </StateSurface>,
    );
    await expect.element(getByText("加载中…")).toBeInTheDocument();
  });

  it("StateSurface: error 显示错误标题、message 与重试按钮", async () => {
    const { getByText, getByRole } = await render(
      <StateSurface isError error={new Error("接口超时")} onRetry={() => {}} />,
    );
    await expect.element(getByText("加载失败")).toBeInTheDocument();
    await expect.element(getByText("接口超时")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "重试" })).toBeInTheDocument();
  });

  it("StateSurface: denied 渲染无权限", async () => {
    const { getByText } = await render(<StateSurface denied>x</StateSurface>);
    await expect.element(getByText("无访问权限")).toBeInTheDocument();
  });

  it("StateSurface: empty 渲染默认空态", async () => {
    const { getByText } = await render(<StateSurface empty>x</StateSurface>);
    await expect.element(getByText("暂无数据")).toBeInTheDocument();
  });

  it("独立状态组件各自渲染", async () => {
    await expect
      .element((await render(<EmptyState title="空空如也" description="desc" />)).getByText("空空如也"))
      .toBeInTheDocument();
    await expect
      .element((await render(<LoadingState label="请稍候" />)).getByText("请稍候"))
      .toBeInTheDocument();
    await expect
      .element((await render(<ErrorState title="出错了" />)).getByText("出错了"))
      .toBeInTheDocument();
    await expect
      .element((await render(<PermissionDenied title="禁止" />)).getByText("禁止"))
      .toBeInTheDocument();
    await expect
      .element((await render(<IconButton aria-label="更多">⋯</IconButton>)).getByLabelText("更多"))
      .toBeInTheDocument();
  });
});
