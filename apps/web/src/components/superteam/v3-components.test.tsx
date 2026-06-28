import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import {
  IconTile,
  SignatureCard,
  SoftCard,
  StatusPill,
  V3Button,
  V3Chip,
  V3EmptyState,
  V3ErrorState,
  V3IconButton,
  V3LoadingState,
  V3MetricCard,
  V3Pagination,
  V3PermissionDenied,
  V3Segmented,
  V3StateSurface,
  V3Tab,
  V3TabList,
  V3Table,
  V3Tabs,
  V3Td,
  V3Th,
  V3ToolbarSearch,
  V3Tr,
  WorkSurface,
} from "./v3-components";

/**
 * 仿真测试：用本仓库真实浏览器测试栈（vitest-browser-react）挂载渲染 v3 组件族，
 * 验证其组合正确、应用 v3 token 派生 class，且各覆盖态与交互可被正确驱动。
 * 这是“v3 参考实现可用性”的回归护栏，不是业务测试。
 */

describe("v3 组件族 · 基础渲染", () => {
  it("SoftCard 应用 v3 容器 class", async () => {
    const { getByText } = await render(<SoftCard>技能概览</SoftCard>);
    const card = getByText("技能概览");
    await expect.element(card).toHaveAttribute("data-slot", "v3-soft-card");
    await expect.element(card).toHaveClass("rounded-v3-card");
    await expect.element(card).toHaveClass("bg-v3-card");
    await expect.element(card).toHaveClass("shadow-sm");
  });

  it("IconTile 应用 tone 与柔色背景 class", async () => {
    const { getByLabelText } = await render(
      <IconTile tone="danger" size="lg" aria-label="危险图标">
        <span>x</span>
      </IconTile>,
    );
    const tile = getByLabelText("危险图标");
    await expect.element(tile).toHaveAttribute("data-slot", "v3-icon-tile");
    await expect.element(tile).toHaveClass("text-v3-danger");
    await expect.element(tile).toHaveClass("bg-v3-danger-soft");
  });

  it("StatusPill 渲染圆点与文案，应用语义色", async () => {
    const { getByText } = await render(<StatusPill tone="warn">预警</StatusPill>);
    const pill = getByText("预警");
    await expect.element(pill).toHaveAttribute("data-slot", "v3-status-pill");
    await expect.element(pill).toHaveClass("text-v3-warn");
    await expect.element(pill).toHaveClass("bg-v3-warn-soft");
  });

  it("V3MetricCard 渲染标签、大数字；loud 时数值点亮为 warn", async () => {
    const { getByText } = await render(
      <V3MetricCard label="待审批" value="4" loud meta="2 个高风险" />,
    );
    await expect.element(getByText("待审批")).toBeInTheDocument();
    await expect.element(getByText("4")).toHaveClass("text-v3-warn");
    await expect.element(getByText("2 个高风险")).toBeInTheDocument();
  });

  it("SignatureCard 渲染并写入蓝色渐变 style", async () => {
    const { getByText } = await render(<SignatureCard>signature</SignatureCard>);
    const sig = getByText("signature");
    await expect.element(sig).toHaveAttribute("data-slot", "v3-signature-card");
    await expect.element(sig).toHaveAttribute("style");
  });
});

describe("v3 数据面 · 表格", () => {
  it("V3Table 表头与 danger 行可组合渲染，danger 行带 data-tone 标记", async () => {
    const { getByRole, getByText } = await render(
      <WorkSurface>
        <V3Table>
          <thead>
            <tr>
              <V3Th>事项</V3Th>
            </tr>
          </thead>
          <tbody>
            <V3Tr tone="danger">
              <V3Td>生产发布</V3Td>
            </V3Tr>
          </tbody>
        </V3Table>
      </WorkSurface>,
    );
    await expect.element(getByText("事项")).toBeInTheDocument();
    await expect.element(getByText("生产发布")).toBeInTheDocument();
    const row = getByRole("row", { name: /生产发布/ });
    await expect.element(row).toHaveAttribute("data-tone", "danger");
  });

  it("V3Tr 的 warn 行使用 warning 软底和左侧 accent", async () => {
    const { getByRole } = await render(
      <V3Table>
        <tbody>
          <V3Tr tone="warn">
            <V3Td>运行依赖缺失</V3Td>
          </V3Tr>
        </tbody>
      </V3Table>,
    );
    const row = getByRole("row", { name: /运行依赖缺失/ });
    await expect.element(row).toHaveAttribute("data-tone", "warn");
    await expect.element(row).toHaveClass("[&>td]:bg-v3-warn-soft");
    await expect
      .element(row)
      .toHaveClass("[&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-warn)]");
  });
});

describe("v3 交互组件", () => {
  it("V3Button 记录 variant，文案可点", async () => {
    const onClick = vi.fn();
    const { getByRole } = await render(
      <V3Button variant="danger" onClick={onClick}>
        处理
      </V3Button>,
    );
    const btn = getByRole("button", { name: "处理" });
    await expect.element(btn).toHaveAttribute("data-variant", "danger");
    await userEvent.click(btn);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("V3Chip 选中态切换到蓝柔底", async () => {
    const { getByRole } = await render(<V3Chip active>全部</V3Chip>);
    await expect
      .element(getByRole("button", { name: /全部/ }))
      .toHaveClass("bg-v3-brand-soft");
  });

  it("V3Segmented 点击回传所选值", async () => {
    const onChange = vi.fn();
    const { getByRole } = await render(
      <V3Segmented
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

  it("V3Pagination 点击下一页回传页码", async () => {
    const onPageChange = vi.fn();
    const { getByRole } = await render(
      <V3Pagination
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

  it("V3ToolbarSearch 渲染可输入搜索框", async () => {
    const { getByRole } = await render(<V3ToolbarSearch placeholder="搜索…" />);
    const input = getByRole("textbox");
    await expect.element(input).toBeInTheDocument();
    await expect.element(input).toHaveAttribute("placeholder", "搜索…");
  });

  it("V3Tabs 组合渲染，激活 Tab 带蓝柔底", async () => {
    const { getByRole } = await render(
      <V3Tabs>
        <V3TabList>
          <V3Tab active>任务</V3Tab>
          <V3Tab>审计</V3Tab>
        </V3TabList>
      </V3Tabs>,
    );
    await expect
      .element(getByRole("button", { name: "任务" }))
      .toHaveClass("bg-v3-brand-soft");
    await expect.element(getByRole("button", { name: "审计" })).toBeInTheDocument();
  });
});

describe("v3 覆盖态", () => {
  it("V3StateSurface: loading 渲染加载态", async () => {
    const { getByText } = await render(
      <V3StateSurface isLoading>
        <span>不应出现</span>
      </V3StateSurface>,
    );
    await expect.element(getByText("加载中…")).toBeInTheDocument();
  });

  it("V3StateSurface: error 显示错误标题、message 与重试按钮", async () => {
    const { getByText, getByRole } = await render(
      <V3StateSurface isError error={new Error("接口超时")} onRetry={() => {}} />,
    );
    await expect.element(getByText("加载失败")).toBeInTheDocument();
    await expect.element(getByText("接口超时")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "重试" })).toBeInTheDocument();
  });

  it("V3StateSurface: denied 渲染无权限", async () => {
    const { getByText } = await render(<V3StateSurface denied>x</V3StateSurface>);
    await expect.element(getByText("无访问权限")).toBeInTheDocument();
  });

  it("V3StateSurface: empty 渲染默认空态", async () => {
    const { getByText } = await render(<V3StateSurface empty>x</V3StateSurface>);
    await expect.element(getByText("暂无数据")).toBeInTheDocument();
  });

  it("独立状态组件各自渲染", async () => {
    await expect
      .element((await render(<V3EmptyState title="空空如也" description="desc" />)).getByText("空空如也"))
      .toBeInTheDocument();
    await expect
      .element((await render(<V3LoadingState label="请稍候" />)).getByText("请稍候"))
      .toBeInTheDocument();
    await expect
      .element((await render(<V3ErrorState title="出错了" />)).getByText("出错了"))
      .toBeInTheDocument();
    await expect
      .element((await render(<V3PermissionDenied title="禁止" />)).getByText("禁止"))
      .toBeInTheDocument();
    await expect
      .element((await render(<V3IconButton aria-label="更多">⋯</V3IconButton>)).getByLabelText("更多"))
      .toBeInTheDocument();
  });
});
