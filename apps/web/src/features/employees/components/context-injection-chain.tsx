import { ArrowRight, Blocks, BookOpen, FolderGit2, KeyRound, Network, ScrollText, UserRound, Users } from "lucide-react";
import { IconTile, SoftCard } from "@/components/superteam";

type ContextInjectionChainProps = {
  roleLabel: string;
  hasPersonaMemory: boolean;
  personalSkillCount: number;
  inheritedSkillCount: number;
  mcpCount: number;
  envConfiguredCount: number;
  envTotalCount: number;
};

export function ContextInjectionChain({ roleLabel, hasPersonaMemory, personalSkillCount, inheritedSkillCount, mcpCount, envConfiguredCount, envTotalCount }: ContextInjectionChainProps) {
  const nodes = [
    { icon: <UserRound />, title: "角色说明", meta: roleLabel },
    { icon: <ScrollText />, title: "项目宪法", meta: "执行时由项目注入" },
    { icon: <BookOpen />, title: "人格记忆", meta: hasPersonaMemory ? "已配置" : "未设置" },
    { icon: <Blocks />, title: "个人技能", meta: `${personalSkillCount} 项` },
    { icon: <Users />, title: "团队继承技能", meta: `${inheritedSkillCount} 项` },
    { icon: <Network />, title: "MCP", meta: `${mcpCount} 项` },
    { icon: <KeyRound />, title: "环境变量", meta: `${envConfiguredCount} / ${envTotalCount}` },
    { icon: <FolderGit2 />, title: "工作目录", meta: "只读" },
  ];

  return (
    <SoftCard className="p-5">
      <p className="mb-3 text-sm font-semibold text-v3-ink">下次任务会注入的上下文包（按注入顺序 · 只读）</p>
      <div className="flex flex-wrap items-center gap-2">
        {nodes.map((node, index) => (
          <div className="flex items-center gap-2" key={node.title}>
            <div className="flex min-w-[104px] flex-col items-center gap-1.5 rounded-v3-inner bg-v3-card-soft px-3 py-2.5 text-center">
              <IconTile size="sm" tone="mute">
                {node.icon}
              </IconTile>
              <p className="text-xs font-semibold text-v3-ink">{node.title}</p>
              <p className="text-[11px] text-v3-ink-3">{node.meta}</p>
            </div>
            {index < nodes.length - 1 ? <ArrowRight aria-hidden className="size-4 shrink-0 text-v3-ink-3" /> : null}
          </div>
        ))}
      </div>
    </SoftCard>
  );
}
