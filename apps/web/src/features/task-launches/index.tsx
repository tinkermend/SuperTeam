import { TaskLaunchShell } from "./components/task-launch-shell";

export function TaskLaunchPage() {
  return (
    <TaskLaunchShell
      title="任务发起"
      description="提交需求到项目，由项目协调线程编排后续任务"
    >
      <div>任务发起表单加载中</div>
    </TaskLaunchShell>
  );
}

export function TaskLaunchDetailPage({ demandId }: { demandId: string }) {
  return (
    <TaskLaunchShell title="发起详情" description="查看一次任务发起触发的协调事实">
      <div>发起详情 {demandId}</div>
    </TaskLaunchShell>
  );
}
