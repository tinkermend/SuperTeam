# SuperTeam 私有化交付：部署 / 平台便捷 / 运维可维护性差距分析

| 字段 | 值 |
|---|---|
| 日期 | 2026-08-11 |
| 分支 | `docs/private-deploy-ops-analysis` |
| 性质 | **分析与优先级建议**（本文件即交付物；不实现镜像 / Compose / rustfs） |
| 调查基线 commit | `ce9766fc`（worktree 创建点；事实以仓库当时文件为准） |
| 读者 | 产品负责人、交付工程师、企业侧运维 |

---

## 0. 结论摘要

把 SuperTeam **打成一个可交付版本、在客户私有环境里站起来**，当前最大的缺口不是「业务功能不够」，而是 **交付形态仍绑定作者本机开发工作流**：

| 现状一句话 | 证据 |
|---|---|
| 依赖服务有 Compose，**一等业务进程无镜像** | 全仓一等 `Dockerfile*` 不存在；仅有 `docker-compose.dev.yml` |
| Compose **只起基础设施**，不启 CP / Web / Runtime / Temporal / Connector | `docker-compose.dev.yml`：postgres / redis / minio / openfga |
| 应用启停靠 `scripts/dev-services.sh` + 本机 `go run` / `cargo run` / `pnpm dev` | `package.json`、`dev-services.sh` |
| 对象存储路径已产品化（CP 配置 + presign），私有化桶与 **桶 CORS 引导** 仍缺交付工具 | `config.example.yaml` `objectStore`；`TODO.md` 生产桶 CORS |
| CORS / 反代同源方向已在代码与 TODO 中显式化，**生产反代样例未入库** | `http.allowedOrigins` + `environment: prod`；nginx 同源 TODO |
| 健康检查偏浅；Runtime `/health` 运维绑定文档缺失 | CP `GET /health` 基本 ok；hardening spec「零可观测性」 |

**若只做一件事**：交付「单站点 Compose（或等价）一键栈」——镜像 + 多文件 Compose + 配置契约样例 + 初始化脚本 + 健康门禁——客户才能在无源码开发环境中落地。其余 P0 项（桶 CORS、prod 配置清单、迁移/bootstrap、反代）都是这个栈的组成部分。

下文按 **现状盘点 → 三大目标问题 → 具名主题 → 分级建议（含验收形态）** 展开。本目标 **不** 实现镜像 / 生产 Compose / rustfs / Helm。

---

## 1. 现状盘点（Current inventory）

### 1.1 进程与职责（交付时必须交代清的边界）

| 进程 | 目录 | 角色 | 私有化默认是否必需 |
|---|---|---|---|
| **Web Console** | `apps/web` | 管理 / 审批 / 观察；非业务事实源 | 是（人类操作面） |
| **Control Plane** | `apps/control-plane` | 业务状态、调度、审批、审计、工件元数据、presign、authz 决策点 | 是 |
| **Runtime Agent** | `apps/runtime-agent` | 领任务、租约、拉起 Provider、工作目录、日志/工件上传 | 是（至少一个执行节点） |
| **Temporal** | 外部依赖 | 每 Project 协调工作流 `project-coordinator:{id}` | 生产协调能力开启时是（`temporal.enabled`） |
| **PostgreSQL** | 外部 | 主业务库 + 迁移 | 是 |
| **Redis** | 外部 | CP 依赖 | 是 |
| **对象存储（S3 兼容）** | 外部 | 工件 / skill 包 / raw log 等；**仅 CP 持凭证并发 presign** | 是（有技能/工件/执行输出即需要） |
| **OpenFGA** | 可选 | `authz.engine=openfga*` 时；默认 `engine: db` 可不启 | 默认可关；切 OpenFGA 时需要 |
| **feishu-connector** | `apps/feishu-connector` | 飞书投影；无 DB、无业务状态 | 否（仅开通飞书通道时） |
| **desktop** | `apps/desktop` | 桌面端 | 否（本分析不展开） |
| **Provider CLI** | 主机侧 | Claude Code / OpenCode / Codex 等；Runtime 管理进程 | 是（执行节点上） |

Agents.md 分层约束对交付同样成立：Runtime 不承载业务策略；Console 不成为事实源；客户差异进 Profile / Connector / Capability / Policy，不进核心流程硬编码。

### 1.2 现有「部署表面」清单

| 类别 | 路径 / 能力 | 对私有化的含义 |
|---|---|---|
| **Compose（仅 dev 依赖）** | `docker-compose.dev.yml` | postgres:16、redis:7、minio(+mc 建桶 `superteam-artifacts` 且 **anonymous download**)、openfga(+migrate, **sqlite 卷**)。**无** CP/Web/Runtime/Temporal 服务定义 |
| **本地启停** | `scripts/dev-services.sh` | 启停 temporal / control-plane / web / runtime-agent / feishu-connector；迁移 atlas；OpenFGA 可 compose 或 local；pid/log 在 `.scratch/`。**明确是开发脚本**，非客户交付入口 |
| **OpenFGA 引导** | `scripts/openfga-bootstrap.sh` | 建 store + 写 model + smoke check，吐出 `OPENFGA_*` env；偏 shadow 试验 |
| **CP 配置样例** | `apps/control-plane/config/config.example.yaml` | `environment`、`http`（含 CORS）、`postgres`、`redis`、`objectStore`、`employeeEnv`、`temporal`、`authz`、`auth`、`planner` |
| **Runtime 配置样例** | `apps/runtime-agent/config.example.yaml` | `control_plane_url`、`bootstrap_key`、`http.addr`、workspace、providers、logging；**对象存储凭证不在 Runtime 配置中**——上传/下载走 CP presign（见 `src/artifacts.rs` / `raw_log.rs` / `skills.rs` 与 `config.rs` 说明） |
| **Web 构建** | `pnpm build:web` / Vite；`VITE_CONTROL_PLANE_URL` | 静态资源构建；API 基址见 `apps/web/src/lib/config/control-plane-url.ts`（默认同 hostname `:8080`） |
| **迁移** | `apps/control-plane/internal/storage/migrations/` + Atlas | 生产必须可重复 apply；dev-services 启动前自动 migrate |
| **健康** | CP `GET /health`；Runtime `GET /health`；OpenFGA `/healthz` | CP health 偏「进程活着」；hardening 已点名 **不探 DB/Temporal**、无 `/metrics` |
| **契约** | `contracts/control-plane/openapi.yaml` 等 | 版本交付应附带契约版本与 `verify:*` 门禁结果，但 **无发布流水线文档** |
| **已知债（平台）** | `docs/superpowers/specs/2026-07-25-p1-platform-hardening.md` §6 | 无 CI / Dockerfile / 部署清单；CP 硬单实例（进程内连接注册表）；零可观测性 |
| **已知债（TODO）** | `TODO.md` | 生产桶 CORS 引导命令；Console↔CP nginx 同源；Runtime `/health` 绑定与运维探针文档 |

### 1.3 配置 / 环境变量契约（摘录）

**Control Plane**（yaml + env 覆盖，见 `internal/config/config.go`）：

| 关注点 | 配置键 / 环境变量 | 私有化要点 |
|---|---|---|
| 环境模式 | `environment` / `CONTROL_PLANE_ENV` = `dev` \| `prod` | **prod 漏配 `http.allowedOrigins` 启动失败**（防 localhost 开发默认泄漏到生产） |
| 监听 | `http.addr` / `CONTROL_PLANE_ADDR` | 默认示例 `:8080` |
| CORS | `http.allowedOrigins` / `CONTROL_PLANE_CORS_ALLOWED_ORIGINS` | 跨源带凭据；同源反代后可空（TODO 尚未放宽 prod 校验） |
| 对象存储 | `objectStore.*` | S3 兼容 endpoint/region/bucket/keys/`forcePathStyle`；本地 MinIO 常 `forcePathStyle: true` |
| 员工 env 加密 | `employeeEnv` / `SUPERTEAM_ENV_ENCRYPTION_KEYS` | **必填**；密钥轮转靠多 key + activeKeyId |
| Temporal | `temporal.*` / `TEMPORAL_*` | `enabled` 时 address/namespace/taskQueue 必填 |
| Authz | `authz.engine`；OpenFGA 时 `OPENFGA_*` | 默认 `db` 降低外部依赖 |
| 登录验证码 | `auth.captchaEnabled` / `AUTH_CAPTCHA_ENABLED` | 示例注释：生产请 `true` |
| Planner | `planner.*` | 协调规划依赖外部 OpenAI-compatible 端点；**私有化需可达内网/代理 LLM** |
| 凭据加密 | `security.credentialEncryptionKey` / `CONTROL_PLANE_CREDENTIAL_KEY` | connector 等密文 |

**Runtime Agent**：

| 关注点 | 配置 | 私有化要点 |
|---|---|---|
| 接入 | `runtime.control_plane_url`、`bootstrap_key` | 节点注册/心跳；密钥分发需交付流程 |
| 本地 HTTP | `http.addr` 默认 `127.0.0.1:7077` | 运维探针若绑 loopback，**集群外探不到**（TODO 已记） |
| 工作区 | `workspace.base_dir` 默认 `/var/superteam/workspaces` | 需持久卷与清理策略 |
| Provider | `providers.*.binary_path` | 执行机预装 CLI 与凭据；镜像化 Runtime **通常不内嵌** 全部 Provider 许可 |

**Web**：

- 构建期 `VITE_CONTROL_PLANE_URL`；运行时解析见 `control-plane-url.ts`。
- 未配置时：`${location.protocol}//${hostname}:8080` —— 假设 CP 与 Console **同主机不同端口**，与「nginx 同源反代」终态不一致（TODO）。

**feishu-connector**（可选）：

- `CONTROL_PLANE_URL`、`FEISHU_CONNECTOR_TOKEN`（必填）、`CONTROL_PLANE_WEB_ORIGIN`。

### 1.4 对象存储与跨域（已实现行为）

1. **架构**：Runtime / 浏览器 **不持长期对象存储密钥**；上传下载走 CP 签发的 **presigned URL**（artifact / raw-log / skill 等 API 已在 openapi）。
2. **浏览器预览陷阱**：`GET .../artifacts/{id}/content` 默认 302 到 presign；fetch 跨域重定向后 Origin 变 `null`。产品已提供 `format=json` 两步取 URL（见 followups / openapi 注释），以降低对桶放行 `null` 的依赖。
3. **dev Compose MinIO**：`mc anonymous set download` 对 `superteam-artifacts` —— **开发便利，生产不可照搬**（应私有桶 + presign + 明确 CORS）。
4. **TODO**：`apps/control-plane/cmd/bucket-cors/` 幂等引导（未做）；规则模板见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2。

### 1.5 健康与可观测性

| 组件 | 现状 | 私有化缺口 |
|---|---|---|
| CP `/health` | 返回 service/status ok 类载荷 | 不探 postgres/redis/objectStore/temporal；负载均衡「绿」≠ 可调度 |
| Runtime `/health` | 探针 Provider 可用性 | 默认监听与文档不完整；daemon 场景常 unbound（TODO） |
| OpenFGA | `/healthz` | compose 已用 |
| Metrics / 追踪 | hardening：**go.mod 无 prometheus/otel** | 企业运维无标准拉数面 |
| 日志 | Runtime 可 json；CP 常规 log | 缺统一字段约定（tenant/project/run_id）与采集说明 |

### 1.6 明确「不是」交付入口的东西

- `scripts/dev-services.sh` + worktree 并行约定（`docs/PARALLEL_DEVELOPMENT.md`）
- 仓库内 `.scratch/`、本机 `config.yaml`（gitignore）
- 仅 dev 的 OpenFGA sqlite + Playground
- MinIO anonymous download
- 无官方 Helm/K8s/Ansible 清单（至分析日）

---

## 2. 部署便捷性（私有站点如何快速、简单地部署）

目标读者：**客户运维**，手里是「版本包 + 配置模板 + 内网镜像仓库」，没有 SuperTeam 开发机。

### 2.1 推荐交付形态（分层，避免一步上 K8s）

| 层级 | 形态 | 适用 |
|---|---|---|
| **L1 默认交付** | 多文件 Docker Compose + 预构建镜像 + `.env` 样例 + `install`/`upgrade` 脚本 | 单机或小集群 VM；大多数私有化一期 |
| **L2** | 同一镜像，K8s/Helm 或客户已有编排 | 客户强制容器平台 |
| **L3** | 二进制 + 系统包（systemd） | 无容器、Runtime 必须贴物理机 GPU/内网工具链 |

**原则**：L1 必须能「填 env → up → 迁移 → 健康门禁 → 建租户/管理员 → 注册一个 Runtime」；L2/L3 复用同一镜像与配置契约，不各写一套语义。

### 2.2 建议的多 Compose 布局（文件职责，非本目标实现）

```
deploy/
  compose/
    00-infra.yml          # postgres, redis, object-store(minio|rustfs), (optional) openfga
    10-platform.yml       # control-plane, web(nginx 静态), temporal(+ui 可选)
    20-runtime.yml        # runtime-agent 示例（常跑在执行机，可独立）
    30-connectors.yml     # feishu-connector 等可选
    profiles 或 -f 叠加：all-in-one | core | edge-runtime
  env/
    .env.example          # 全部公开键；密钥占位
    config/
      control-plane.prod.example.yaml
      runtime-agent.prod.example.yaml
  scripts/
    bootstrap-site.sh     # 等待健康 → migrate → seed admin → 打印下一步
    bucket-cors.sh        # 或未来 CP cmd
    upgrade.sh            # 拉镜像 → migrate → 滚动重启
  reverse-proxy/
    nginx.same-origin.example.conf
  README.md               # 拓扑图 + 端口 + 升级 + 备份
```

与现状对照：今天只有「00-infra 的 dev 子集」，且 openfga 用 sqlite、minio 匿名读——**不能直接改名当生产**。

### 2.3 容器镜像该打谁

| 镜像 | 建议 | 备注 |
|---|---|---|
| `superteam-control-plane` | **P0** | 多阶段 Go 构建；入口 `cmd/control-plane`；挂载 config 或 env |
| `superteam-web` | **P0** | 构建静态资源 + nginx/caddy；注入或运行时配置 API 基址策略 |
| `superteam-runtime-agent` | **P0（形态二选一）** | 精简 agent 镜像 **或** 官方二进制 + 主机 systemd；Provider CLI 建议 **主机层安装**，避免许可与体积 |
| `superteam-feishu-connector` | P1 | 可选通道 |
| 依赖镜像 | 固定 digest | postgres/redis/minio 或 rustfs/openfga/temporal | **钉版本**，禁止 `latest` 交付 |

**不建议** 把「作者本机 go/cargo/pnpm」写进客户 runbook。

### 2.4 私有对象存储（MinIO / rustfs / 云 S3）

| 项 | 建议 |
|---|---|
| 默认私有化选项 | **S3 兼容** 自建：MinIO（生态熟）或 **rustfs**（用户倾向；交付时二选一写清兼容矩阵与 `forcePathStyle`） |
| CP 配置 | `objectStore.endpoint/region/bucket/accessKeyId/secretAccessKey/forcePathStyle` |
| 桶策略 | **私有**；禁止 dev 的 anonymous download；生命周期/配额另议 |
| CORS | 对 Web origin 放行 GET/HEAD（及预检需要的头）；`format=json` 路径下优先常规 origin，避免依赖 `null` |
| 网络 | CP 与 Runtime 均需能访问 **presign 返回的 URL 主机名**（内外网 endpoint 分裂时要配公共可达 endpoint 或反向代理） |
| 初始化 | Compose 侧 init 任务：建桶 + 应用 CORS；或 `bucket-cors` 命令读 CP 同款 S3 配置 |

### 2.5 反代与跨域：两条合法拓扑

| 拓扑 | 做法 | 配置后果 |
|---|---|---|
| **A. 同源反代（推荐终态）** | nginx 将 `/` → web，`/api` 与 `/health` → CP | 浏览器无跨源；`allowedOrigins` 可空；Web API 基址改为相对路径；**需同步放宽 prod fail-fast**（TODO 已写） |
| **B. 分域名 / 分端口** | Console `https://console…`，API `https://api…` | 必须显式 `allowedOrigins`；桶 CORS 对齐 Console origin；cookie/CORS 带凭据要严格 origin 匹配 |

当前代码默认偏 **B 的开发变体**（`:3100` ↔ `:8080`）。交付文档必须二选一写清，并给 **A 的 nginx 样例**（即使先落 B）。

### 2.6 最小安装路径（客户视角 checklist）

1. 准备主机：Docker 或等价；磁盘（PG + 对象 + Runtime workspace）。
2. 填 `.env` / prod yaml：库、Redis、对象存储、加密密钥、`CONTROL_PLANE_ENV=prod`、CORS 或反代、Temporal、Planner 端点。
3. `compose up` infra → 健康。
4. 起 CP → **Atlas migrate** → `/health`。
5. 起 Web / 反代 → 浏览器登录（seed 管理员策略需文档化；现有 `002_seed_dev_admin` 偏 dev）。
6. 初始化对象桶 + CORS。
7. 起 Temporal（若启用）并核对 CP `temporal.enabled`。
8. 在执行节点装 Runtime + Provider CLI，配置 `bootstrap_key`，确认节点在线。
9. 冒烟：建项目 → 派一简单任务 → 工件 presign 上传 → Console 预览/下载。

**今天卡在 2–3 步**：没有一等镜像与生产 Compose，客户只能 clone 源码当开发环境用。

### 2.7 版本升级与备份（部署便捷的后半截）

- **升级**：固定版本 manifest（镜像 digest + 迁移版本 + 契约版本）→ migrate → 滚动；禁止「git pull 主仓」作为客户升级路径。
- **备份**：PostgreSQL 逻辑/物理备份 + 对象存储桶 +（若用）OpenFGA 存储；加密密钥与 bootstrap 密钥 **离线保管**（丢密钥 = 不可恢复密文）。
- **降级**：迁移前向兼容策略需发布说明；无自动 down 不代表可随意回滚 schema。

---

## 3. 平台便捷性（装好后平台本身应提供什么）

部署只解决「进程起来」；企业日复一日用的是 **装完之后的操作面**。下列能力多数 **已有产品雏形**，私有化交付应写进「开箱清单 / 管理员手册」，并补缺口。

### 3.1 已有、应对客户强调的平台能力

| 能力 | 说明 | 交付话术 |
|---|---|---|
| 租户 / 成员 / console.access | 多租户与登录面 | 说明默认引擎 `authz.engine=db` 与 OpenFGA 升级路径 |
| 项目 / 需求 / 人工决策收件箱 | 业务闭环容器 | 「人类一等参与者」边界写进运维职责：审批不能关 |
| Runtime 节点注册与心跳 | 执行面可观察 | 节点失联排障手册 |
| 技能包与工件 | 对象存储 + presign | 与桶/CORS 运维联动 |
| 系统配置（system-config） | 工件大小、presign TTL 等 | 避免改代码调参 |
| 审计 / 日志类 Console | 治理面 | 与 SIEM 对接列为 P2 |
| 飞书通道（可选） | connector 心跳与 channel-health | 标明非核心路径 |
| Provider 语义与 failure_family | 中文状态标签 | 降低现场排障语言成本 |

### 3.2 建议补强的「平台侧便捷」（产品/工程，非仅运维脚本）

| 方向 | 为何 | 私有化价值 |
|---|---|---|
| **站点安装向导 / 首次 bootstrap API** | 今日 seed 与密钥靠手工 | 降低「第一位管理员怎么来」摩擦 |
| **部署拓扑自检页或 CLI** | 一次检查：DB、Redis、对象读写、Temporal、CORS/同源、Runtime 在线 | 装完 5 分钟定位 80% 问题 |
| **版本与构建信息** | `/health` 或 `/version` 暴露 build/git/契约版本 | 多节点偏斜可查 |
| **只读运维角色 / 大屏长会话** | TODO 已有投屏认证债 | 机房大屏不因 cookie 过期黑屏 |
| **备份与导出指导进 Console** | 「导出审计 / 导出项目卷宗」 | 合规客户自助 |
| **空气间隙友好** | 镜像与技能包离线导入；Planner/Provider 走内网代理 | 无公网客户 |
| **连接器与能力注册表** | 已有方向 | 客户差异不进核心发版 |

### 3.3 平台不应承诺的（避免交付过度销售）

- 默认 **多活 CP**：进程内 `ConnectionRegistry` 等使 **硬单实例** 仍是现状（hardening P0#2）；私有化一期应写清 **单 CP 副本** 或「粘性 + 已知限制」。
- Runtime 与 Provider 的任意横向扩缩 **不等于** 协调层 HA。
- 把客户专属审批流硬编码进核心（Agents.md 禁止）。

---

## 4. 运维工具（可运维性工具与定义）

### 4.1 建议工具箱（按优先级见 §6）

| 工具 / 定义 | 类型 | 作用 |
|---|---|---|
| **生产 Compose + 镜像构建定义** | 交付物 | 标准化安装 |
| **`bootstrap-site`** | 脚本/CLI | 健康等待、migrate、初始管理员、打印 Runtime 注册步骤 |
| **`bucket-cors`（TODO 已立项）** | CP `cmd` 或脚本 | 幂等写桶 CORS；`--check` 只读审计 |
| **配置契约文档 + `.env.example`** | 文档 | 全量键、默认值、prod 必填、密钥生成命令 |
| **nginx/Caddy 同源样例** | 配置 | 消除 CORS 类现场事故 |
| **`superteam doctor`** | CLI | 同源自检：TCP/HTTP、presign put/get 小对象、Temporal namespace、authz engine |
| **健康分层** | 产品 | `/health/live` vs `/health/ready`（ready 探 DB/Redis/对象/Temporal） |
| **`/metrics`（Prometheus）** | 产品 | 请求量、调度失败、Runtime 在线数、队列深度 |
| **结构化日志规范** | 规范 | json 字段：service、tenant_id、project_id、run_id、trace |
| **备份/恢复 runbook** | 文档 | RPO/RTO 建议值与演练步骤 |
| **Runtime 节点安装包** | deb/systemd 或 compose | 含 bootstrap 文件权限、workspace 目录、日志轮转 |
| **迁移校验** | 已有 `migrate-validate` | 发布流水线强制；客户 upgrade 脚本调用同一 atlas 目录 |
| **OpenFGA bootstrap** | 已有脚本可产品化 | engine 切换时一键 store/model |
| **金丝雀冒烟** | 脚本 | 登录 → 列项目 → Runtime 心跳 → 可选 dry-run 任务（无外网可 mock provider） |

### 4.2 运维对象与告警建议（定义，非实现）

| 信号 | 来源 | 告警意图 |
|---|---|---|
| CP ready 失败 | ready 探针 | 页面可能仍开但无法写库 |
| Runtime 节点 stale | CP 心跳 | 任务堆积 |
| Temporal 不可达 | CP 日志/ready | 协调停摆 |
| 对象 presign/上传失败率 | 指标 | 桶策略/CORS/endpoint 错误 |
| 迁移版本落后 | 启动校验 | 禁止旧二进制连新库或反之策略 |
| 证书/license（若有） | 到期 | 提前窗口 |

### 4.3 与开发脚本的边界

| 开发（保留） | 交付（新建） |
|---|---|
| `dev-services.sh` | `deploy/scripts/*` |
| `docker-compose.dev.yml` | `deploy/compose/*`（钉版本、无 anonymous、无 playground 默认） |
| 本机 `config.yaml` | 密钥注入（文件权限 600 / secret store） |
| worktree 并行联调 | 客户单版本栈 |

---

## 5. 具名主题深潜（用户点名项）

### 5.1 容器镜像

- **要写**：CP、Web、（可选）Runtime、Connector 的 `Dockerfile` + CI 构建 + 镜像签名/扫描（企业常见门禁）。
- **Runtime 特殊**：执行机常需 Docker-out-of-Docker 或主机网络访问内网 Git/制品库；更稳的是 **agent 二进制 + 主机 Provider**，容器仅作可选。
- **验收形态**：在干净 VM 仅用镜像 + 配置文件拉起 CP `/health` 与 Web 静态资源，无需安装 Go/Rust/Node 工具链。

### 5.2 多 Compose 配置

- **要写**：infra / platform / runtime / connectors 拆分；`profiles` 或文档化的 `-f` 组合；**all-in-one** 给 POC，**拆分** 给生产。
- **禁止**：把 dev compose 的 sqlite OpenFGA、anonymous MinIO、明文默认密码原样贴客户。
- **验收形态**：文档中一条命令序列可复现「仅 infra」与「infra+platform」；Runtime 可在第二台机器仅用 `20-runtime` + env 接入。

### 5.3 对象存储与 CORS

- 私有化默认起 **S3 兼容**（MinIO 或 rustfs）；CP `objectStore` 指向它；`forcePathStyle` 按实现选择。
- **CORS**：Console origin 的 GET/HEAD；优先依赖 artifact `format=json` 预览路径；生产禁用 anonymous download。
- **跨域 vs 同源**：浏览器↔CP 用反代同源可大幅简化；浏览器↔对象存储仍可能跨域，靠 presign + 桶 CORS。
- **验收形态**：`doctor` 或脚本完成 put/get 往返；Console 预览 md/html 不依赖 `null` origin；关闭匿名读后仍工作。

### 5.4 进程内置参数 / 配置契约

- 以 `config.example.yaml` 与 Runtime example 为 **契约源**；交付包附「生产必填表」。
- 敏感项：`employeeEnv` 密钥、`bootstrap_key`、对象存储密钥、DB、Planner API key、connector token。
- `environment: prod` 行为必须写进运维手册（CORS fail-fast）。
- Runtime `workspace.base_dir`、清理策略、并发度与主机容量规划挂钩。
- **验收形态**：缺任一 prod 必填项时进程 **拒绝启动并打印键名**（CP 已对 CORS/部分项这样做）；文档与 example 一致。

### 5.5 控制平面 ↔ Runtime ↔ Web 网络关系（简图）

```
[浏览器] --(HTTPS)--> [反代]
                        |-- /        --> Web 静态
                        |-- /api,/health --> Control Plane
[Control Plane] --S3 API--> [对象存储]
[Control Plane] --SQL--> [Postgres]  --Redis--> [Redis]
[Control Plane] --SDK--> [Temporal]
[Runtime Agent] --HTTPS API+心跳--> [Control Plane]
[Runtime Agent] --presigned PUT/GET--> [对象存储]
[Runtime Agent] --spawn--> [Provider CLI]
```

现场 90% 故障落在：**DNS/证书、CORS/同源、presign endpoint 浏览器不可达、Runtime 到 CP 网络、Provider 未装**。

---

## 6. 优先级建议（P0 / P1 / P2）

每条含：**差距**、**为何重要**、**验收形态（done looks like）**。  
本分析目标 **不实现** 下列项，仅定义后续工程 backlog。

### 6.1 P0 — 没有则难以称为「可交付版本」

| ID | 建议项 | 差距 vs 现状 | 为何重要 | 验收形态（done looks like） |
|---|---|---|---|---|
| P0-1 | **一等业务镜像**（至少 CP + Web） | 无 Dockerfile；靠源码 dev 启动 | 客户环境禁止开发工具链 | 干净机 `docker pull` + 配置可起 CP `/health` 与 Web；构建脚本在仓内可重复 |
| P0-2 | **生产向多文件 Compose（或等价栈定义）** | 仅有 `docker-compose.dev.yml` 依赖子集 | 标准化拓扑与依赖顺序 | `deploy/compose` 文档化组合；all-in-one POC 30 分钟内到登录页（含 migrate） |
| P0-3 | **生产配置契约包** | example 存在但不等于交付清单；dev 密钥习惯危险 | 漏配导致启动失败或安全事故 | `.env.example` + prod yaml 样例 + 必填表；`CONTROL_PLANE_ENV=prod` 矩阵测过 |
| P0-4 | **站点 bootstrap 脚本** | 迁移/管理员/节点步骤散落 | 降低实施对作者的依赖 | 一键：wait healthy → atlas apply → 初始租户/管理员 → 打印 Runtime 注册 |
| P0-5 | **对象存储私有化路径** | dev MinIO 匿名读；无 rustfs/MinIO 生产样例 | 工件/技能不可用则平台空转 | Compose 可起 S3 兼容；CP 指向它；私有桶；上传下载冒烟通过 |
| P0-6 | **桶 CORS 引导** | TODO 未做；现场易哑火预览 | Console 预览/下载类故障高频 | `bucket-cors --check` 可审计；apply 幂等；文档与 followups §2 一致 |
| P0-7 | **反代样例 + 拓扑选择文档** | nginx 同源仅 TODO | 避免家家现场重踩 CORS | 仓内 nginx 样例；手册写清拓扑 A/B 与 `allowedOrigins` / `VITE_*` 关系 |
| P0-8 | **发布版本清单** | 无「版本=镜像 digest+迁移+契约」 | 升级与排障无锚点 | 每个 release 附 SBOM/清单文件；升级脚本只消费清单 |

### 6.2 P1 — 显著降低 TCO / 支持成本

| ID | 建议项 | 差距 vs 现状 | 为何重要 | 验收形态 |
|---|---|---|---|---|
| P1-1 | **分层健康检查 ready/live** | `/health` 过浅 | LB 误导 | ready 失败当依赖挂；live 仅进程 |
| P1-2 | **`superteam doctor` CLI** | 无统一自检 | 远程支持成本 | 一条命令输出通过/失败项与修复提示 |
| P1-3 | **Runtime 交付件**（二进制或镜像 + systemd + `/health` 绑定文档） | TODO：health 常 unbound | 执行面不可观测 | 文档写明默认监听；探针可从运维网访问或 sidecar |
| P1-4 | **feishu-connector 镜像与 compose profile** | 仅 go run | 可选通道也要可装 | profile 开关即可起 |
| P1-5 | **Temporal 生产样例**（非 `start-dev`） | dev 用 temporal CLI dev server | 协调可靠性 | compose/helm 参考；CP 连得上 |
| P1-6 | **备份恢复 runbook + 演练脚本** | 无 | 企业验收刚需 | 文档含 PG+桶；演练记录模板 |
| P1-7 | **OpenFGA 生产存储**（Postgres 等） | dev sqlite 卷 | 权限引擎持久化 | engine 切换文档 + bootstrap 产品化 |
| P1-8 | **首次安装向导或管理员恢复流程** | 依赖 dev seed 思维 | 现场锁死 | 文档化 break-glass |

### 6.3 P2 — 规模化与企业集成

| ID | 建议项 | 差距 vs 现状 | 为何重要 | 验收形态 |
|---|---|---|---|---|
| P2-1 | **Prometheus `/metrics` + 仪表盘** | hardening 零可观测 | 并入客户监控 | 关键指标可 scrape |
| P2-2 | **CP 多副本 / 连接注册表外置** | 硬单实例 | 水平扩展 | 设计落地后文档更新拓扑 |
| P2-3 | **Helm chart / K8s 清单** | 无 | 容器平台客户 | 与 Compose 同契约 |
| P2-4 | **日志字段规范 + 示例采集** | 不统一 | SIEM | 示例 filebeat/otel 配置 |
| P2-5 | **离线包 / 空气间隙** | 未定义 | 高安全网 | 一次导入全镜像+迁移 |
| P2-6 | **自动升级通道** | 无 | 多站点运维 | 可选；默认可关 |
| P2-7 | **合规附件** | 部分 THIRD_PARTY | 采购 | 许可证汇总、数据流图 |

---

## 7. 建议落地顺序（仅规划）

```text
Phase A（交付最小闭环）
  P0-1 镜像 → P0-3 配置契约 → P0-2 Compose → P0-4 bootstrap
  → P0-5 对象存储 → P0-6 CORS → P0-7 反代文档 → P0-8 版本清单

Phase B（可支持）
  P1-1 ready → P1-2 doctor → P1-3 Runtime 交付 → P1-5 Temporal → P1-6 备份

Phase C（规模化）
  P2-1 metrics → P2-2 多副本设计 → P2-3 Helm → …
```

与既有人类拍板关系：hardening 曾将「无 CI/Dockerfile/部署清单」标为生产 P0 但 **当时明确不插队**；本文件在 **私有化交付** 语境下重新将其列为交付 P0——**产品功能优先级仍可由路线图另排**，但若目标是「给其他企业场地用」，上述 P0 不可再无限后置。

---

## 8. 非目标（本分析不覆盖或明确不做）

- 本文件 **不** 提交 Dockerfile、生产 Compose、rustfs 部署、Helm、nginx 真配置落地。
- 不改 CP/Runtime/Web 产品行为与 authz。
- 不做 Temporal/OpenFGA/DB 的完整 HA SRE 手册。
- 不设计客户专属连接器业务逻辑。
- 不在远端企业网做真实私有化 dry-run。

---

## 9. 关键仓库锚点（便于复核「现状」断言）

| 断言 | 锚点 |
|---|---|
| 仅 dev 依赖 Compose | `docker-compose.dev.yml` |
| 无业务 Dockerfile | 仓根检索 `Dockerfile*`（业务树无） |
| CP 配置含 objectStore + CORS + prod 规则 | `apps/control-plane/config/config.example.yaml`，`internal/config/config.go` |
| Runtime 对象仅 presign | `apps/runtime-agent/src/config.rs`（presign-only 注释）；`src/artifacts.rs` / `raw_log.rs` / `skills.rs`；example 无对象存储密钥段 |
| 开发启停 | `scripts/dev-services.sh`，`package.json` scripts |
| 生产桶 CORS 债 | `TODO.md`；`docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` |
| nginx 同源债 | `TODO.md`；`apps/web/src/lib/config/control-plane-url.ts` |
| 部署清单缺失已记录 | `docs/superpowers/specs/2026-07-25-p1-platform-hardening.md` §6 |
| 健康与可观测债 | 同上；CP `GET /health`；Runtime `src/server.rs` |

---

## 10. 修订记录

| 日期 | 说明 |
|---|---|
| 2026-08-11 | 初版：私有化部署/平台/运维差距分析与 P0–P2；分支 `docs/private-deploy-ops-analysis` |
