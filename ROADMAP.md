# ROADMAP

## 未完成计划

### 渐进式授权与 OpenFGA 演进

- [ ] 落地统一 `Authorizer` 接口与 `DBAuthorizer`，业务代码不得直接散落权限判断。
- [ ] 将权限事实先存储在 PostgreSQL，覆盖租户、团队、Runtime 服务范围与 Capability 授权。
- [ ] 按资源域逐步接入 `OpenFGAAuthorizer`：先 tenant/team，再 runtime/capability，最后扩展到 task/artifact/approval。
- [ ] 接入 OpenFGA 前完成 relation model、tuple 同步、历史数据 backfill、DB 与 OpenFGA 双跑校验和灰度切换。
- [ ] 权限管理页面始终面向业务对象，不暴露 OpenFGA tuple、relation model 或底层 DSL。

