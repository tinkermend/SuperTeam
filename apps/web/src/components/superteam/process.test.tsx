import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import {
  CopyableMono,
  FileDropzone,
  Progress,
  ProgressRing,
  Stepper,
  Timeline,
} from "@/components/superteam";

describe("process · Stepper", () => {
  it("标记当前与已完成步骤", async () => {
    const { getByText, container } = await render(
      <Stepper
        current={1}
        steps={[
          { id: "a", label: "基础信息" },
          { id: "b", label: "确认" },
          { id: "c", label: "完成" },
        ]}
      />,
    );
    await expect.element(getByText("基础信息")).toBeInTheDocument();
    const current = container.querySelector('[data-state="current"]');
    expect(current?.textContent).toContain("确认");
    const completed = container.querySelector('[data-state="completed"]');
    expect(completed?.textContent).toContain("基础信息");
  });

  it("点击已完成步骤触发 onStepChange", async () => {
    const onStepChange = vi.fn();
    const { getByRole } = await render(
      <Stepper
        current={1}
        onStepChange={onStepChange}
        steps={[
          { id: "a", label: "基础信息" },
          { id: "b", label: "确认" },
        ]}
      />,
    );
    await userEvent.click(getByRole("button", { name: "回到步骤 1：基础信息" }));
    expect(onStepChange).toHaveBeenCalledWith(0);
  });
});

describe("process · Timeline / Progress", () => {
  it("Timeline 渲染条目与 tone", async () => {
    const { getByText } = await render(
      <Timeline
        items={[
          {
            id: "1",
            title: "已派发",
            description: "任务进入队列",
            time: "10:00",
            tone: "info",
          },
          {
            id: "2",
            title: "失败",
            description: "超时",
            tone: "danger",
          },
        ]}
      />,
    );
    await expect.element(getByText("已派发")).toBeInTheDocument();
    await expect.element(getByText("失败")).toBeInTheDocument();
    await expect.element(getByText("10:00")).toBeInTheDocument();
  });

  it("Progress 确定与不确定", async () => {
    const { getByRole, getByText } = await render(
      <div>
        <Progress value={40} label="上传" showValue />
        <Progress label="处理中" />
      </div>,
    );
    const bar = getByRole("progressbar", { name: "上传" });
    await expect.element(bar).toHaveAttribute("aria-valuenow", "40");
    await expect.element(getByText("40%")).toBeInTheDocument();
    await expect
      .element(getByRole("progressbar", { name: "处理中" }))
      .not.toHaveAttribute("aria-valuenow");
  });

  it("ProgressRing 展示百分比", async () => {
    const { getByRole, getByText } = await render(
      <ProgressRing value={75} label="完成度" />,
    );
    await expect.element(getByRole("progressbar", { name: "完成度" })).toHaveAttribute(
      "aria-valuenow",
      "75",
    );
    await expect.element(getByText("75%")).toBeInTheDocument();
  });
});

describe("process · FileDropzone / CopyableMono", () => {
  it("FileDropzone 选择文件后展示名称", async () => {
    const onFilesChange = vi.fn();
    const file = new File(["hello"], "skill.zip", { type: "application/zip" });
    const { getByLabelText, getByText, container } = await render(
      <FileDropzone
        label="技能包"
        description="仅 zip"
        accept=".zip"
        onFilesChange={onFilesChange}
        files={[]}
      />,
    );

    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    expect(input).toBeTruthy();
    await userEvent.upload(input, file);
    expect(onFilesChange).toHaveBeenCalled();
    const arg = onFilesChange.mock.calls[0]?.[0] as File[];
    expect(arg?.[0]?.name).toBe("skill.zip");

    // controlled empty still shows label
    await expect.element(getByText("技能包")).toBeInTheDocument();
    void getByLabelText;
  });

  it("FileDropzone 受控展示已选文件", async () => {
    const file = new File(["x"], "pack.zip");
    const { getByText } = await render(
      <FileDropzone files={[file]} label="上传" onClear={vi.fn()} />,
    );
    await expect.element(getByText("pack.zip")).toBeInTheDocument();
    await expect.element(getByText("清除")).toBeInTheDocument();
  });

  it("CopyableMono 可点击复制", async () => {
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    const { getByRole } = await render(
      <CopyableMono value="req_abc_123456" display="req_abc…" />,
    );
    await userEvent.click(getByRole("button", { name: "复制 req_abc_123456" }));
    expect(writeText).toHaveBeenCalledWith("req_abc_123456");
    writeText.mockRestore();
  });
});
