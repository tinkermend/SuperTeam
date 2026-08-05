import { createFileRoute } from "@tanstack/react-router";
import { RoleVocabularyPage } from "@/features/role-vocabulary";

/**
 * 角色词表管理页。
 * 路径稳定供语义扩编「去注册角色」深链（批三 §5.1 / H9b）；勿改 url。
 */
export const Route = createFileRoute("/_authenticated/role-vocabulary/")({
  component: RoleVocabularyPage,
});
