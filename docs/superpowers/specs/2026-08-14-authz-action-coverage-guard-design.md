# 2026-08-14 授权 action 覆盖率护栏

> 状态：**已实现落地**（`apps/control-plane/internal/authz/openfga_coverage_test.go`，2026-08-14）。实现期对 §3.3 做了强化、对 §3.4 做了降级，见 §8 实现纪要  
> 范围：`apps/control-plane/internal/authz/` 单测一个文件；不改任何生产代码路径  
> 上位决定：`docs/OPENFGA_MIGRATION_DEBT.md` §2.1（暂不迁移 OpenFGA），本护栏是那个决定下唯一要做的事

## 1. 要解决什么

`authz/types.go` 有 **80 个 action**，`openfga_mapping.go` 只映射了 **25 个**。这个差距是**不可见**的：

- `AUTHZ_ENGINE=db`（默认）：根本不查 FGA，差距无感。
- `openfga_shadow`：未映射 action 被**静默跳过**（`shadow_authorizer.go:55-58`），不查不记不算背离——覆盖率最低的那 69% 恰恰完全静默。
- `openfga`（纯执行）：未映射 action **直接拒绝**（`openfga_authorizer.go:93-100`）。

于是形成一个最坏的组合：**平时零感知，切换瞬间大面积 403。** 而 action 表每周都在长（`types.go` 光 2026-07 就动了 7 次），映射自试验性接入后没动过（`openfga_mapping.go` 有史以来 7 次提交）。

**本护栏不修任何债，只让债停止隐形增长。** 新增 action 时，开发者必须做一次有意识的选择：映射它，或登记它。

## 2. 目标 / 非目标

**目标**

- 新增 `Action*` 常量而未处理 OpenFGA 映射 → **测试失败**，失败信息直接告诉人下一步做什么。
- 未映射清单变成显式、分域、可读的代码常量，而不是「读两个文件自己对差集」。
- D1 的「25/80」数字由测试自动维护，不靠人记得回来改台账。

**非目标**

- 不补任何映射、不改 `model.fga`、不动 authorizer 判别逻辑。
- 不做 hybrid authorizer、不做 D6 报错分离（见台账 §3 偿还顺序，那是迁移开工时的事）。
- 不校验「action 在 authorizer.go 的 switch 里有没有 case」——那是 DB 侧覆盖率，与本护栏无关，且漏写 case 会走 default 拒绝，本身是安全失败。

## 3. 方案

新增 `apps/control-plane/internal/authz/openfga_coverage_test.go`，包内测试（`package authz`），三条断言。

### 3.1 主护栏：action 覆盖率

**枚举方式：AST 解析本包源码。** Go 常量无法反射枚举，而手工维护 `AllActions` 切片只是把同一个「忘记同步」的问题挪了个地方。

```
os.ReadDir(".")
  → 每个非 _test.go 的 .go 文件
  → parser.ParseFile(fset, name, nil, 0)
  → 遍历 GenDecl(token.CONST) 的 ValueSpec
  → 收集 标识符以 "Action" 开头 且 值为字符串字面量 的常量
```

用 `os.ReadDir` + `parser.ParseFile` 而**不用** `parser.ParseDir`——后者在 Go 1.22 起已废弃，而本仓库是 Go 1.25.5。也不引入 `x/tools`，标准库够用。

> **坑（我核实时真踩过）**：判别必须是 `strings.HasPrefix(ident, "Action")`，**不能**用「包含 Action」。`types.go:136` 有 `ReasonUnsupportedAction = "unsupported action"`，用包含判别会把它算成一个 action，覆盖率分母凭空 +1。

对每个收集到的 action 值，调用**已有的** `openFGARelationForAction(value)`（同包，直接可见，不需要导出）：

- 返回 `ok=true` → 已映射。
- 返回 `ok=false` → 必须出现在 `knownUnmappedActions` 里，否则测试失败。

同时反向断言：`knownUnmappedActions` 里的每一项都必须仍是真实 action **且仍未映射**。这样映射补上之后，清单不会留下过期条目——**债还清了测试会主动提醒你删行**。

### 3.2 显式清单的形态

放在测试文件里，`map[string]string`（action → 归属域），按域分组注释。用 map 而非 slice：重复项在编译期就是错误，且断言时不用先去重。

```go
// knownUnmappedActions 是"已知未接入 OpenFGA"的显式清单。
// 新增 action 时二选一：
//   1) 在 openFGARelationForAction 里映射它（同时确认 openFGAObjectForRequest
//      能为它的资源类型产出对象，否则等于没映射——见台账 D2）；
//   2) 加进本清单，并在 docs/OPENFGA_MIGRATION_DEBT.md 更新对应债务条目。
// 不允许静默跳过——那正是本护栏要杜绝的事。
var knownUnmappedActions = map[string]string{
    ActionProjectCreate: "project",
    // ... 55 项，按 project / employee / credential / mcp_registry /
    //     scenario_template / system_config / skill / task / audit / misc 分组
}
```

**初始 55 项由实现者跑一次测试拿到**，不要照抄本文档——写这份方案时的计数是 80/25/55，实现时若已有偏差，以测试输出为准。

### 3.3 附带断言：资源类型覆盖

`types.go:18-25` 定义 8 种 resource，`openFGAObjectForRequest`（`:114-145`）只认 4 种（tenant / team / skill / console）。这是台账 D2：**即使补了 relation，资源产不出对象也等于没映射。**

同样用 AST 收集 `Resource*` 常量，逐个用一个最小 `CheckRequest` 调 `openFGAObjectForRequest`，断言「能产出对象」的集合恰好等于显式清单 `resourceTypesWithFGAObject`。新增 resource 类型时同样强制表态。

### 3.4 附带断言：authzcenter 枚举不含幽灵 action（可选，优先级最低）

`authzcenter/generated.go:40-61` 的 `CheckPermissionRequestAction` 是 OpenAPI 生成的封闭枚举（21 项）。可断言**其中每一项都是真实存在的 authz action**，防止契约侧写错字符串或留下已删除的 action。

**只断言这一个方向。** 反方向（每个 action 都要进枚举）是故意不成立的——枚举只覆盖权限中心自查接口需要的子集，那是 D5，不在本期修。

**放置位置（已核实）**：`authzcenter` import 了 `authz`（`pg_repository.go:12`、`handler.go:11`），而 `authz` 不 import 它。所以在 `package authz` 的**包内**测试里 import `authzcenter` 会**成环编译失败**。两条出路：

| 方案 | 代价 |
|---|---|
| 放进 `authzcenter` 包自己的测试，复制一份 ~15 行的 AST 扫描去读 `authz/types.go` | 少量重复，但依赖方向自然、无脆弱路径 |
| 留在 authz 侧，AST 解析 `../authzcenter/generated.go` | 单文件集中，但跨包按相对路径解析**生成文件**，重新生成或改布局就断 |

**选前者。** §3.1/§3.3 那两条主断言本来就在一个文件里，"集中在一处"的目标已经达成；为了把这条最弱的断言也塞进同一文件而去跨包解析生成物，是拿脆弱换整齐。

**若时间紧，这条可以不做**——它防的是契约笔误，不是 OpenFGA 覆盖率，与本护栏的核心目标只是相邻。

## 4. 失败信息（这是本护栏的产品面）

护栏的价值全在失败那一刻是否说得清楚。失败信息必须包含：**是什么、为什么拦、两个具体选项、去哪登记**。

```
action "project.demand.submit" 未接入 OpenFGA，也不在 knownUnmappedActions 清单里。

新增 authz action 时必须二选一：
  1) 在 openfga_mapping.go 的 openFGARelationForAction 里映射它；
     注意同时确认 openFGAObjectForRequest 能为它的资源类型产出对象，
     否则 FGA 会拿一个模型里不存在的对象类型去查（见台账 D2/D4）。
  2) 若本期不接入（多数情况），把它加进 openfga_coverage_test.go 的
     knownUnmappedActions，并更新 docs/OPENFGA_MIGRATION_DEBT.md 的 D1。

背景：AUTHZ_ENGINE=openfga 时未映射 action 一律拒绝，
     而 shadow 模式会静默跳过它们、不产生任何告警。
```

另外用 `t.Logf` 输出一行覆盖率摘要（`OpenFGA action 覆盖率: 25/80，未映射 55`），方便更新台账 D1 时直接读数，不必手数。

## 5. 实现顺序

1. 建 `openfga_coverage_test.go`，先只写 AST 收集 + `t.Logf` 打印全部 action 与映射状态，跑一次拿到真实清单。
2. 把输出灌进 `knownUnmappedActions`（按域分组加注释）。
3. 补三条断言与失败信息。
4. 跑 `corepack pnpm test:go` 确认全绿。
5. 用真实反例验护栏有效（见 §6）。
6. 把测得的覆盖率数字回填 `docs/OPENFGA_MIGRATION_DEBT.md` 的 D1。

## 6. 验收

**必须做判别性验证**——一个恒绿的护栏和没有护栏等价：

- 临时加一个 `ActionFooBar = "foo.bar"` 常量 → 测试**失败**，且失败信息里出现 `foo.bar` 与两个选项。删掉后恢复绿。
- 把某个 action 从 `knownUnmappedActions` 里删掉 → 测试**失败**（说明清单不是摆设）。
- 把某个已映射 action（如 `skill.install`）加进 `knownUnmappedActions` → 测试**失败**（说明反向断言生效，还清的债会被要求删行）。
- 临时加一个 `ResourceFooBar = "foobar"` → 资源断言**失败**。
- （若做了 §3.4）把 `authzcenter` 枚举里某项改成不存在的字符串 → 该断言**失败**。
- 以上反例全部回滚后，`corepack pnpm test:go` 全绿。

轻量例外适用：本变更只加单测、不改生产代码路径、不碰交互/数据/权限/持久化链路，**不需要全链路 E2E**。

## 7. 后续

本护栏只管住「不再隐形增长」。真正的债（D1–D6）按台账 §3 的顺序在迁移开工时还，**第一步是 hybrid authorizer，不是补映射**。届时每补一批映射，`knownUnmappedActions` 就删一批，测试会逼着两边同步——这正是本护栏在迁移期的第二重价值。

## 8. 实现纪要（2026-08-14）

落地文件：`apps/control-plane/internal/authz/openfga_coverage_test.go`。三条断言 + 七条反例验证全过，`corepack pnpm test:go` 全量绿，gofmt / go vet 干净。

### 与方案的偏差

**§3.3 资源断言被强化（重要）。** 方案原写法是「维护一份『能产出对象的资源』清单，断言实际与清单一致」。实现后跑反例发现**这个写法恒绿**：新增一个未处理的资源类型时，「代码没处理」和「清单没记录」两个 `false` 相互抵消，断言认为一致，护栏形同虚设。

改为 `resourceFGAObjectStatus` **全覆盖表态**——每一个已声明资源类型都必须在表里记 `{emitsObject, note}`，不在表里即失败。这个缺陷是判别性验证抓出来的，代码里留了注释「勿改回去」。

**§3.4 未实现**（方案已标为可选、优先级最低）。它防的是契约笔误而非 OpenFGA 覆盖率，且需要在 `authzcenter` 包另开一份 AST 扫描。D5 仍留在台账里。

**新增第三条断言 `TestFGAEmittableObjectTypesExistInModel`**（方案里没有）。断言「代码能产出的 FGA 对象类型」必须存在于 `model.fga`，已知地雷登记在 `knownObjectTypesMissingFromModel`。这条比原 §3.3 更贴近 D4 的真实危害——它会在有人试图映射 `skill.read` 之类 action 时，直接指出「model 里没有 skill 类型」。`model.fga` 用行扫描解析（顶格 `type X`），不值得引 FGA 官方解析器。

### 实测数字

`25/81，未映射 56`。**比写方案时的 80 多一个**：并发会话已在工作区实现了技能包 spec §13.2，新增了 `ActionSkillArchiveReplace`（`types.go:71` + authorizer switch）。护栏立刻把它认成未映射并要求登记——这正是设计意图第一次生效。

> 该 action 当时是**未提交的工作区改动**。若它被回滚，`knownUnmappedActions` 里对应行会触发反向断言失败，失败信息会提示删行。这是设计行为，不是故障。

### 反例验证清单（全部按预期失败后回滚）

1. 新增未登记 action `ActionFooBar` → 主断言失败
2. 从 `knownUnmappedActions` 删掉 `ActionTaskClaim` → 主断言失败
3. 把已映射的 `ActionSkillInstall` 塞进清单 → 反向断言失败（还清的债被要求删行）
4. 新增资源类型 `ResourceFooBar` → 资源断言失败（**修正前此例不失败**）
5. 清空 `knownObjectTypesMissingFromModel` → model 类型断言失败，指出 `skill` 类型缺失
6. 把 `ResourceTask` 状态记成 `true` → 资源断言报实际/记录不符
7. 往资源表塞一个不存在的 `"ghost"` → 反向断言要求删行
