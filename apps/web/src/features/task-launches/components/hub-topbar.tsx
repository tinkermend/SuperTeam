import type { ReactNode } from "react";
import type { ApiClientOptions } from "@/lib/api/client";
import type { Project } from "@/lib/api/projects";
import {
  NoProjectsEmptyState,
  ProjectPicker,
  type ProjectChangeHandler,
} from "./task-launch-form";
import "./task-hub-chat.css";

export type HubTopbarProps = {
  apiOptions?: ApiClientOptions;
  /** 前置槽：「对话 / 任务」页签。与项目锚点同处一条，切面时工具条不跳。 */
  lead?: ReactNode;
  onProjectChange: ProjectChangeHandler;
  projectId: string;
  projects: Project[];
  /** 父层 listProjects 失败时不要当成「没有项目」。 */
  projectsError?: boolean;
  /** 父层 listProjects 进行中；空数组在加载期不是「没有项目」。 */
  projectsLoading?: boolean;
  /** 父层缓存的已选项目（搜索越界）。 */
  resolvedProject?: Project | null;
};

/**
 * 任务中枢顶部工具条：页签 + 项目锚点。
 *
 * 项目是两个台面共同的业务锚点（对话按它挑成员、任务按它落需求与过滤实例），
 * 所以它属于页面级上下文开关而非某个台面的表单字段——两面共用这一条。
 */
export function HubTopbar({
  apiOptions,
  lead,
  onProjectChange,
  projectId,
  projects,
  projectsError = false,
  projectsLoading = false,
  resolvedProject = null,
}: HubTopbarProps) {
  return (
    <div className="hub-topbar">
      {lead ? (
        <>
          {lead}
          <span aria-hidden className="hub-topbar-sep" />
        </>
      ) : null}
      {projectsLoading ? (
        <p className="tl-proj-none-text">加载项目…</p>
      ) : projectsError ? (
        <p className="tl-proj-none-text">项目列表加载失败</p>
      ) : projects.length === 0 ? (
        <NoProjectsEmptyState />
      ) : (
        <ProjectPicker
          apiOptions={apiOptions}
          onChange={onProjectChange}
          projects={projects}
          resolvedProject={resolvedProject}
          value={projectId}
        />
      )}
    </div>
  );
}
