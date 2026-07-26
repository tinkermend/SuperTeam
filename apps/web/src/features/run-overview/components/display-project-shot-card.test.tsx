import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectLens, ProjectRunBandOption } from "../runtime-overview-project-lens";
import { DisplayProjectShotCard } from "./display-project-shot-card";

const option: ProjectRunBandOption = {
  projectId: "p-1",
  name: "验收菜单复验",
  participantCount: 2,
  runningCount: 1,
  queuedCount: 0,
  waitingHumanCount: 1,
  failedCount: 0,
  unassignedCount: 0,
  completedTodayCount: 2,
  hasActive: true,
  source: "summary"
};

const lens: ProjectLens = {
  projectId: "p-1",
  participantEmployeeIds: ["emp-a", "emp-b"],
  stopEmployeeIds: [],
  edges: [{ id: "emp-a->emp-b", fromEmployeeId: "emp-a", toEmployeeId: "emp-b", tone: "primary", taskCount: 1 }],
  totalTaskCount: 3,
  unassignedTaskCount: 0,
  blockedTaskCount: 1
};

describe("DisplayProjectShotCard", () => {
  it("names the focused project and shows lens plus run-band counts", async () => {
    const screen = await render(
      <DisplayProjectShotCard option={option} lens={lens} demand={{ id: "d-1", title: "链路需求" } as never} />,
    );

    const card = screen.container.querySelector("[data-display-project-shot-card]");
    expect(card?.textContent).toContain("项目镜头");
    expect(card?.textContent).toContain("验收菜单复验");
    expect(card?.textContent).toContain("需求 · 链路需求");
    expect(card?.textContent).toContain("参与 2 人");
    expect(card?.textContent).toContain("交接 1 段");
    expect(card?.textContent).toContain("阻塞 1");
    expect(card?.textContent).toContain("运行 1");
    expect(card?.textContent).toContain("待人工 1");
  });

  it("renders nothing without an option and a loading hint without a lens", async () => {
    const none = await render(<DisplayProjectShotCard option={undefined} lens={lens} />);
    expect(none.container.querySelector("[data-display-project-shot-card]")).toBeNull();

    const loading = await render(<DisplayProjectShotCard option={option} lens={undefined} />);
    expect(loading.container.querySelector("[data-display-project-shot-card]")?.textContent).toContain(
      "正在加载任务链路",
    );
  });
});
