import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { AlertTriangle, Search } from "lucide-react";
import {
  Callout,
  Field,
  FormSection,
  Kbd,
  ListToolbar,
  SoftDialog,
  SoftDialogBody,
  SoftDialogContent,
  SoftDialogDescription,
  SoftDialogFooter,
  SoftDialogHeader,
  SoftDialogTitle,
  SoftSheet,
  SoftSheetBody,
  SoftSheetContent,
  SoftSheetDescription,
  SoftSheetFooter,
  SoftSheetHeader,
  SoftSheetTitle,
  Button,
  Chip,
  Segmented,
  ToolbarSearch,
} from "@/components/superteam";

describe("foundation · Kbd / Callout / Field", () => {
  it("Kbd 渲染固定 keycap 语义", async () => {
    const { getByText } = await render(
      <p>
        打开命令菜单 <Kbd>⌘</Kbd>
        <Kbd>K</Kbd>
      </p>,
    );
    const key = getByText("⌘");
    await expect.element(key).toHaveAttribute("data-slot", "kbd");
    await expect.element(key).toHaveClass("font-mono");
  });

  it("Callout 应用 tone 与标题", async () => {
    const { getByText, getByRole } = await render(
      <Callout
        tone="warn"
        title="需要人工确认"
        description="提交后将进入审批队列。"
        icon={<AlertTriangle aria-hidden />}
      />,
    );
    const note = getByRole("note");
    await expect.element(note).toHaveAttribute("data-slot", "callout");
    await expect.element(note).toHaveAttribute("data-tone", "warn");
    await expect.element(getByText("需要人工确认")).toBeInTheDocument();
    await expect.element(getByText("提交后将进入审批队列。")).toBeInTheDocument();
  });

  it("Field 渲染 label / required / error", async () => {
    const { getByText, getByRole } = await render(
      <Field label="名称" htmlFor="name" required error="必填">
        <input id="name" aria-label="名称" />
      </Field>,
    );
    await expect.element(getByText("名称")).toBeInTheDocument();
    await expect.element(getByText("*")).toBeInTheDocument();
    const err = getByRole("alert");
    await expect.element(err).toHaveAttribute("data-slot", "field-error");
    await expect.element(err).toHaveTextContent("必填");
  });

  it("FormSection 渲染分组标题与字段", async () => {
    const { getByText } = await render(
      <FormSection title="基本信息" description="用于目录展示">
        <Field label="显示名">
          <input aria-label="显示名" />
        </Field>
      </FormSection>,
    );
    await expect.element(getByText("基本信息")).toBeInTheDocument();
    await expect.element(getByText("用于目录展示")).toBeInTheDocument();
  });
});

describe("foundation · ListToolbar", () => {
  it("按槽位渲染 search / filters / segments / actions", async () => {
    const onSeg = vi.fn();
    const { getByPlaceholder, getByRole, getByText } = await render(
      <ListToolbar
        search={<ToolbarSearch placeholder="搜索项目" />}
        filters={<Chip>运行中</Chip>}
        segments={
          <Segmented
            value="all"
            onChange={onSeg}
            options={[
              { label: "全部", value: "all" },
              { label: "我的", value: "mine" },
            ]}
          />
        }
        actions={<Button size="sm">导出</Button>}
      />,
    );
    await expect.element(getByPlaceholder("搜索项目")).toBeInTheDocument();
    await expect.element(getByText("运行中")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "我的" })).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "导出" })).toBeInTheDocument();
  });
});

describe("foundation · SoftDialog / SoftSheet", () => {
  it("SoftDialog 渲染标题、说明、底栏主操作与关闭", async () => {
    const onOpenChange = vi.fn();
    const { getByRole, getByText } = await render(
      <SoftDialog open onOpenChange={onOpenChange}>
        <SoftDialogContent size="md">
          <SoftDialogHeader icon={<Search aria-hidden />}>
            <SoftDialogTitle>创建技能</SoftDialogTitle>
            <SoftDialogDescription>上传 zip 并填写元数据。</SoftDialogDescription>
          </SoftDialogHeader>
          <SoftDialogBody>
            <Field label="名称">
              <input aria-label="名称" />
            </Field>
          </SoftDialogBody>
          <SoftDialogFooter left="不会立即发布">
            <Button variant="outline">取消</Button>
            <Button>创建</Button>
          </SoftDialogFooter>
        </SoftDialogContent>
      </SoftDialog>,
    );

    await expect.element(getByRole("heading", { name: "创建技能" })).toBeInTheDocument();
    await expect.element(getByText("上传 zip 并填写元数据。")).toBeInTheDocument();
    await expect.element(getByText("不会立即发布")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "创建" })).toBeInTheDocument();

    await userEvent.click(getByRole("button", { name: "关闭" }));
    expect(onOpenChange).toHaveBeenCalled();
  });

  it("SoftSheet 渲染详情骨架", async () => {
    const { getByRole, getByText } = await render(
      <SoftSheet open onOpenChange={vi.fn()}>
        <SoftSheetContent size="lg">
          <SoftSheetHeader>
            <SoftSheetTitle>任务详情</SoftSheetTitle>
            <SoftSheetDescription>来自收件箱队列</SoftSheetDescription>
          </SoftSheetHeader>
          <SoftSheetBody>
            <p>摘要内容</p>
          </SoftSheetBody>
          <SoftSheetFooter>
            <Button variant="outline">关闭面板</Button>
            <Button>处理</Button>
          </SoftSheetFooter>
        </SoftSheetContent>
      </SoftSheet>,
    );

    await expect.element(getByRole("heading", { name: "任务详情" })).toBeInTheDocument();
    await expect.element(getByText("摘要内容")).toBeInTheDocument();
    await expect.element(getByRole("button", { name: "处理" })).toBeInTheDocument();
  });
});
