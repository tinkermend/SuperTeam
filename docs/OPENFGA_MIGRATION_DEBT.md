# OpenFGA 迁移债务台账

> 这是一份**长期累积**的文档，不是设计稿，没有日期后缀，不归档。
>
> **写入时机**：开发任何功能时，若发现「现在不做、但真正切 OpenFGA 时必须处理」的问题，就在 §3 追加一条。
> **读取时机**：真正推进 OpenFGA 落地时，先通读本文档——这里的每一条都会在切换瞬间变成故障。
>
> 追加格式见 §4。只写关键信息与可定位的锚点，不写论证过程（论证留在各自的 spec 里）。

## 1. 为什么会有债

授权早期按「统一 `Authorizer` 接口 + DB 实现先跑起来，OpenFGA 后续替换实现」的路子建（见 `docs/superpowers/specs/2026-06-01-incremental-authorization-openfga-design.md`）。接口这层守住了，但 **OpenFGA 侧的映射与模型一直是按需零星补的**，从没跟着业务 action 同步增长。

结果：DB 授权是完整的，OpenFGA 授权是稀疏的，而**这个差距在默认配置下完全不可见**。

## 2. 当前接线（改动前必读）

**三种引擎**，`AUTHZ_ENGINE` 控制，默认 `db`（`config.go:228`）：

| 引擎 | 未映射 action 的下场 | 备注 |
|---|---|---|
| `db`（**默认**） | 不查 FGA，只有 `authz/authorizer.go` 说了算 | 生产/开发现状 |
| `openfga_shadow` | **静默放行**：主(DB)决策生效，不查不记不算背离（`shadow_authorizer.go:55-58`） | rollout 脚本用的（`scripts/openfga-bootstrap.sh:66`） |
| `openfga`（纯执行） | **直接拒绝**：`ReasonUnsupportedAction`（`openfga_authorizer.go:93-100`） | 未启用 |

**关键后果**：shadow 模式**不会**暴露任何未映射 action——它们被静默跳过，背离统计里看不见。所以「shadow 跑了很久没告警」**不能**作为「可以切纯 openfga」的依据。这是本台账存在的根本原因。

**三个事实源**（改 authz 时三处都要看）：

- `authz/types.go` — action 与 resource 常量表（**76 个 action**）
- `authz/authorizer.go` — DB 判别逻辑（完整）
- `authz/openfga_mapping.go` — action→relation、resource→object 映射（**稀疏**）
- `authz/openfga/model.fga` — FGA 类型与关系（只有 `user` / `tenant` / `team` / `project`）

## 2.1 现行决定：暂不迁移（2026-08-14）

**已评估并拍板：现阶段不推进 OpenFGA 迁移，继续用 `db` 引擎。** 这不是拖延，是有依据的选择——半年后有人翻到 55 个未映射 action，请先读这一节再判断是不是事故。

依据：

1. **最强的 ReBAC 驱动已被产品侧删除。** 团队借调（跨团队委托）连同角色申请在 `8359a52f` 整体下线。剩下的模型基本是 `租户 → 团队 → 用户` 层级 + 少量 project scope，DB authorizer 处理得很好。没有关系型需求时，OpenFGA 不是更好的方案，只是更重的方案。
2. **债务是休眠的，成本为零。** 默认引擎 `db`（`config.go:228`）；OpenFGA 只在 `docker-compose.dev.yml` 里跑且数据落 `file:/tmp/openfga.db`（容器临时目录的 SQLite），是开发玩具不是部署。
3. **授权模型本身还在变形**（role 词表重构、员工配置重构均在途）。把还在动的模型翻译进 FGA = 翻译两遍。
4. **`Authorizer` 接口抽象完好**，业务 handler 不依赖 OpenFGA。这是 2026-06 那份 spec 买下的「延后权」，现在正是它付利息的时候。

**同时确认的事实**：`openfga_mapping.go` / `model.fga` 有史以来只有 7 次提交，最后一次实质改动是 `ae034d3e`（即 D4 的对象改写 hack）；而 `authz/types.go` 光 2026-07 就动了 7 次。**action 表每周在长，映射自试验性接入后没动过**——这是已经跑了几个月的稳定模式，不是一次疏忽。

**现在唯一要做的事**：给 action 覆盖率加一道护栏测试，把「看不见且在增长」的债变成「看得见且有边界」的清单。方案见 `docs/superpowers/specs/2026-08-14-authz-action-coverage-guard-design.md`，**已实现落地**：`apps/control-plane/internal/authz/openfga_coverage_test.go`，三条断言、七条反例验证全过。

> 从此新增 authz action 或 resource 类型时，测试会强制你二选一：映射它，或登记它。**债还清后删清单行也是被测试逼着做的**——过期条目会让反向断言失败。本文档 D1/D2/D4 的数字与清单从此有代码作为事实源，不靠人记得回来改。

**明确不做**：hybrid authorizer（按域灰度）、D6 报错/背离分离。这两项是投机性基建，可能白养一年没有用户。但它们改变的是**迁移开工时第一步做什么**——见下方「偿还顺序」。

### 重新评估的触发信号

不看时间，看有没有**真的关系型需求**。以下任一出现即重新评估：

- 团队借调或类似的跨团队委托回归
- 资源级共享（「这个项目对 B 团队可见」）
- 跨租户委托 / 客户要求细粒度授权
- authz 查询成为 DB 性能瓶颈
- **外部硬约束**：合规或客户合同要求策略引擎。这一条会直接推翻上述判断——那时不是技术权衡题，且越晚越贵，应趁模型还小尽快上。

## 3. 债务清单

| # | 债务 | 切换时的影响 | 锚点 | 记于 |
|---|---|---|---|---|
| D1 | **action 映射覆盖率 25/76**，51 个未映射 | 切 `openfga` 瞬间，未映射的 51 个 action 全部 403。整域缺失：project（23 个全缺）、employee（12）、skill（5 缺 4）、credential（3）、mcp_registry（2）、scenario_template（2）、system_config（2）、audit（1）、`system.templates.manage`、`team.governance.edit`。task 族 5 个 action 已随 legacy 任务领取通道下线一并删除（2026-08-16），不再计入 | 数字由护栏测试自动维护，勿手改：`internal/authz/openfga_coverage_test.go` 的 `knownUnmappedActions`；跑 `go test ./internal/authz/ -run TestEveryAuthzAction -v` 读当前值 | 2026-08-14 |
| D2 | **FGA 对象映射只认 4 种资源**：tenant / team / skill / console；`employee` / `credential` / `project` 资源产不出对象 | 即使补了 D1 的 relation，这些资源仍返回 `!ok` → 等同未映射。补 D1 前必须先补这里和 model 类型 | `openfga_mapping.go:114-145`；`authz/types.go:18-25` 共 7 种 resource | 2026-08-14 |
| D3 | **model 里的 `project` 类型是死类型**：定义了 `team` / `owner` / `viewer` / `team_scope_user` 四个关系，但**没有任何代码产出 `project:{id}` 对象**。项目团队范围走的是 `team:{id}` + `project_scope_user` | 真做项目级授权时，要么启用这个类型（需补对象映射与 tuple 同步），要么删掉它免得误导 | `openfga/model.fga` 的 `type project`；实际路径在 `project/team_scope_shadow_authorizer.go:37-40` | 2026-08-14 |
| D4 | **skill 族靠对象改写伪装**：model 无 `skill` 类型；`skill.install` 能跑是因为对象被改写成 `tenant:{tenantID}`；`skill.read` / `skill.upload` / `skill.delete` 干脆未映射 | ⚠️ **新增 skill 类 action 时不要「顺手补映射」**——加进 relation 表却不加对象改写，FGA 会拿 `skill:{uuid}` 查一个不存在的类型 → shadow 下记成假背离（`shadow_authorizer.go:84-87`），enforcing 下报错拒绝。正解是补 model 的 `skill` 类型，或沿用 tenant 改写 | `openfga_mapping.go:126-136` | 2026-08-14 |
| D5 | **`authzcenter` 的 action 枚举与 authz 表严重不同步**：`CheckPermissionRequestAction` 只有 **21** 个，且是 OpenAPI 生成的封闭枚举；skill / project / employee / credential / mcp / scenario_template / system_config / audit 全族缺席 | 权限中心的 check-permission 自查接口问不了这些 action，迁移期无法用它验证判别一致性——正好在最需要它的时候不可用 | `authzcenter/generated.go:40-61` vs `authz/types.go:29-120` | 2026-08-14 |
| D6 | **shadow 把「FGA 报错」和「判别背离」记成同一个 `diff: true`** | 迁移期无法区分「模型缺类型/缺 tuple」与「规则真的不一致」，背离报表失去判别力。建议在补 D1/D2 之前先给 error 单独打标 | `shadow_authorizer.go:84-87` | 2026-08-14 |

### 建议的偿还顺序

真开工时：**先做 hybrid authorizer**（FGA 处理已映射 action、DB 兜底未映射，每次兜底打日志/指标）→ D6（让 shadow 报表可信）→ D2 + D3 + D4（补 model 类型与对象映射）→ D1（批量补 relation）→ D5（对齐自查枚举）。

两条不能搞反的：

- **第一步是 hybrid，不是补映射。** 引擎开关是全局全有全无的（`cfg.Authz.Engine` 一换 80 个 action 一起切）。没有 hybrid 就没有按域灰度，迁移必然是一次性大 cutover。
- **D1 不能先做。** 没有对象映射和 model 类型，补 relation 只会把「静默跳过」变成「报错」，让 shadow 报表更脏。

## 4. 追加模板

在 §3 表格末尾加一行：

```
| D{n} | **一句话说清是什么** | 切 OpenFGA 时会怎样坏 | `文件:行号` | YYYY-MM-DD |
```

要求：

- **锚点必须是文件+行号**，不写「在 authz 模块里」这种查不到的话。
- **影响列写「切换时会怎样」**，不写「不符合最佳实践」。
- 若这条债来自某份 spec，在该 spec 里也留一句反向指针（例：`2026-08-14-skill-package-replace-and-preview-design.md` §6.2 指向本文 D4）。
- 债务被还清后**不要删行**，把编号改成 ~~D{n}~~ 并在末列注明还清日期与提交，否则后来者会重新踩。
