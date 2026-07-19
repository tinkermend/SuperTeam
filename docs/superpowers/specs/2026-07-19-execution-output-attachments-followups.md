# 执行输出附件遗留立项:presign JSON 变体 / 生产桶 CORS / v2 声明式交付物

> 日期:2026-07-19
> 状态:立项(用户批准),未实施
> 上游:`2026-07-19-execution-output-attachments-design.md`(v1 已完结入 main c5733467,GATE 五项真实 E2E 全 PASS)
> 三项相互独立,可分别实施;优先级建议 §1 > §2(§1 落地后 §2 简化)> §3(独立功能线)。

---

## 1. Artifact content 端点 presign JSON 变体(消除 null-origin CORS 依赖)

### 动因(v1 实施中实证的事实)

- Web 预览(md/txt fetch、HTML fetch+srcDoc)拉内容走 `GET /api/v1/artifacts/{id}/content` → 302 → 对象存储 presigned URL。
- **fetch 跨域重定向后浏览器把 Origin 置为 `null`**(redirect taint),桶 CORS 规则必须显式放行 `null` origin 预览才能工作——v1 已在 dev 桶(superteam@TOS 广州)如此配置。
- 放行 `null` 虽经论证低风险(presigned URL 本身即持有可读凭证),但属非常规 CORS 姿势,生产桶复制此配置心智负担大,且任何一次桶配置整理都可能把它当垃圾规则清掉造成预览静默回归(错误态有下载兜底,不致命但劣化)。

### 方案

- `GET /api/v1/artifacts/{artifactRefId}/content?format=json`:鉴权与现 302 路径完全一致(项目读权限),返回 200 `{ "url": "<presigned GET>", "expires_at": "<RFC3339>" }`,不再重定向。默认(无参)行为不变,下载 `<a>` 链接继续走 302。
- Web `getArtifactContentText` 改两步:①credentialed fetch 该 JSON 端点拿 URL;②对 presigned URL 用 `credentials: "omit"` 直接 fetch——请求 Origin 干净(localhost:3000/生产域),非 credentialed 模式下桶 CORS 仅需常规 origin 放行(甚至 `*`)。
- 实施注意:改 `contracts/control-plane/openapi.yaml` 后必须 `generate:control-plane` + `verify:contracts`;handler 在 `apps/control-plane/internal/project/artifact_storage_handler.go` `GetArtifactContent`(改动约十行)。
- **v1 期间未做的原因**:需重启 CP,当时并发会话未提交迁移存在撞号风险,重启不安全。实施时确认 dev-services 归属后正常做即可。

### 验收(真实链路)

- 桶 CORS 规则**移除 `null` origin** 后,浏览器实测 md/txt/HTML 预览全部正常;302 下载路径回归不受影响;`verify:contracts` + `verify:control-plane` 绿。

## 2. 生产桶 CORS 配置(部署项)

- 生产对象存储桶需配置 CORS 允许生产 Web origin 的 GET/HEAD(§1 落地前还需含 `null`;落地后仅常规 origin)。
- 参考 dev 桶现行规则:AllowedOrigins=[web origins(+null)], AllowedMethods=[GET, HEAD], AllowedHeaders=[*], ExposeHeaders=[ETag, Content-Type, Content-Length], MaxAge=3600。
- 建议与部署文档/环境清单一起管理;属运维配置,无代码变更。另注意:**TOS 对原始对象强制 `Content-Disposition: attachment`**(v1 实测),前端已以 fetch+srcDoc 规避,生产选用其他 S3 兼容存储时此行为可能不同,不影响正确性。

## 3. v2 声明式交付物闭环(功能立项提纲)

### 动因

v1 是零 agent 配合的自动兜底:必然混入 agent 草稿类文件(notes.md 等),且"哪个是正式交付物"只能靠人眼分辨。根治 = 交付物由契约声明、由平台核对,与既有 produces/acceptance criteria 线闭环——同时偿还 intent/acceptance 线"判据与 artifact 松关联"的残债(`TaskResultDeliverable.Ref` 不强制指向已上传 artifact)。

### 方案方向(实施前细化为正式 spec)

1. **约定输出目录**:dispatch 注入的执行约定要求 agent 把正式交付物写入工作目录 `deliverables/`;runtime 对该目录内文件**豁免类型白名单与(可放宽的)大小熔断**,全部采集,`artifact_type=declared_output`(区别于兜底的 `execution_output`)。
2. **produces 对齐核对**:completion 时 runtime/CP 将 `deliverables/` 实际文件与任务 `produces` 清单对齐;缺失项走既有 `handoff_deliverable_missing` 打回(C1 自动返工环现成接住)。
3. **Ref 回填**:物化时把 `TaskResultDeliverable.Ref` 回填为 `artifact_ref_id`,打通"判据 → deliverable → 可下载文件"血缘;验收面板判据行可直接点开对应交付物预览(复用 v1 预览组件)。
4. **展示分层**:工件面板三区——正式交付物(declared_output)/ 执行输出附件(execution_output 兜底)/ 证据;验收签署入口优先呈现正式交付物。

### 验收门槛(真实 E2E)

- 真实任务声明 produces → agent 写入 `deliverables/` → 平台核对通过物化 → 验收面板从判据点开交付物预览;缺交付物场景真实触发打回返工并最终收敛。

## 4. 关联既定方向(不在本立项内)

- **平台配置管理功能**(用户方向 2026-07-19):附件白名单/熔断值/排除目录等散落配置收编统一管理——上游 spec §5 已记录,触达时另行立项。
