# 运行总览地图生产化设计

日期：2026-07-05
> 复核状态：CHANGELOG 2026-07-06 00:09记录运行总览落地为真实数据驱动的三层地图
状态：待用户审阅，审阅通过后进入实现计划

## 1. 背景

当前 `test/superteam-runtime-overview-prototype` 已经证明参考图的视觉目标可以被接近 1:1 还原，但最终版本使用的是视觉锁定方式：整张参考图作为底图，再叠加透明数据热点。这个方式适合确认目标效果，不适合进入生产，因为团队卡片、数字员工头像、状态、任务详情和容量都被烘焙在图片里，无法承接真实数据流。

本设计把运行总览从“截图复刻”转为“生产可渲染地图”：保留参考图的视觉结构、明亮克制风格、办公区地图、团队状态卡和右侧详情，但所有业务内容由数据驱动。参考图左下角“提出需求，组建团队，完成闭环”的需求输入卡片在本次目标中移除。

## 2. 目标

- 达到参考图的整体视觉效果：左侧导航、顶部栏、中间办公地图、楼层筛选、状态图例、右侧运行概况与数字员工详情。
- 中间办公区由真实数据渲染：团队、工位、数字员工头像、状态点、选中态、连线、空闲容量和团队卡片都不能依赖整张截图。
- 支持最多 3 个楼层，每个楼层布局可以不同，但共享同一套渲染组件和数据契约。
- 每个团队硬限制最多 10 个数字员工，每个团队办公区固定 10 个工位。
- 支持数字员工新增、删除、状态变化、选中查看、团队满员提示和楼层切换。
- 支持不同屏幕分辨率：地图使用逻辑画布等比缩放，外层控制台布局响应式适配。
- 保留未来接入 Control Plane 真实数据的路径，不把原型 mock 结构固化成生产事实源。

## 3. 非目标

- 不实现第 11 个数字员工的自动溢出、自动扩容或跨团队重新排布；团队超过 10 人时由业务流程拒绝或要求先调整团队。
- 不做无限画布、自由拖拽排座或复杂空间编辑器。
- 不把整个运行总览导出为单张静态图片。
- 不让前端自行推断数字员工 operational status；状态口径以 Control Plane 聚合读模型为准。
- 不在本次设计中改变现有项目、任务、Runtime、Provider 的核心业务模型。

## 4. 推荐方案

采用“React 控制台外壳 + 2.5D 楼层素材 + SVG 连接线层 + HTML absolute 数据层”的方案，并保留 Konva/Canvas 作为后续复杂地图引擎的升级选项。

- React 负责页面框架、筛选、楼层切换、团队卡片、右侧详情、状态图例和普通表单/按钮。
- 2.5D 楼层素材负责办公室墙体、地面、门、家具和装饰的基础视觉，但素材不能包含业务文字、员工头像、状态点或团队数据。
- SVG 负责楼层动线、团队之间的连接线、选中团队边界和任务流转虚线。
- HTML absolute 数据层负责团队卡片、10 个工位、员工头像、状态点、选中光圈、空闲容量和交互命中区。
- 工位不动态生成，每个团队固定 10 个 seat 配置。员工只占用 seat，不改变团队区域的几何形状。
- 生成式图片只用于干净视觉资产，例如无文字、无头像、无业务数据的办公区底纹、墙体、家具纹理或头像占位资源；团队名称、人数、状态、任务、日志和按钮必须由前端真实渲染。

选择这个方案的原因：当前规模被硬限制为每团队 10 人、每团队 10 工位、最多 3 层，HTML/SVG 足以支撑清晰交互和缩放对齐，还能直接复用项目 Tailwind、v3 token、状态组件和可访问性能力。Konva/Canvas 更适合自由拖拽排座、动态生成办公室、大量节点或复杂动画；这些不是第一版目标。

项目当前 Web 主栈是 React + Vite + TanStack Router + Tailwind CSS。本设计不引入 Next.js，也不引入并行前端主栈。

## 5. 信息架构

页面保留参考图的信息层级：

- 顶部区域：搜索、环境切换、通知、帮助、用户信息。
- 标题区域：`运行总览`、说明、当前工作中的数字员工数量。
- 控制区：地图视图 / 表格视图、全部重点、`1层 / 2层 / 3层`、异常优先。
- 图例：异常、工作中、待确认、排队、空闲。
- 地图区：当前楼层的团队办公区，最多展示该楼层配置中的团队。
- 右侧概况：团队数量、当前楼层、数字员工总数、容量使用、按状态统计、其他楼层摘要。
- 右侧详情：当前选中的数字员工头像、名称、角色、状态、所在团队、当前任务、命令日志、证据工件和消耗情况。

左下角需求输入卡片移除后，底部只保留地图操作条：拖拽、缩放、重置、适应视图。

## 6. 楼层与团队容量规则

### 6.1 楼层

- 系统最多支持 3 个楼层：`floor-1`、`floor-2`、`floor-3`。
- 每个楼层有独立 `FloorLayout`，可以配置不同团队、办公区位置、墙体、路径和装饰。
- 楼层按钮只展示存在布局或存在团队数据的楼层；如果三层都存在，则完整展示 1/2/3 层。
- 右侧“其他楼层”展示非当前楼层的团队数、异常数和可点击入口。

### 6.2 团队

- 每个团队最多支持 10 个数字员工。
- 每个团队布局固定 10 个工位。
- `capacity` 固定为 10，不由前端根据团队人数扩大。
- 团队人数大于 10 是数据错误或业务拒绝态，前端显示满员/超限告警，不能继续把第 11 人渲染进地图。
- 团队卡片展示 `已占用/10`，例如 `容量 7/10`。
- 空工位可以显示为空桌；当空位较多时，可在团队区域边缘显示 `+N 空闲`，但不隐藏真实座位配置。

### 6.3 工位分配

- 员工进入团队时分配一个稳定 `seatId`。
- 员工删除或移出团队时释放 seat，不移动其他员工。
- 员工状态变化只更新头像外圈、状态点、任务摘要和右侧详情。
- 如果员工没有 seat，前端把该员工列入“待分配”状态，不在地图上随机摆放。

## 7. 数据契约

前端消费一个运行总览聚合 DTO。第一版可以由 mock 提供同形数据，生产接入时由 Control Plane 提供。

```ts
type RuntimeOverviewDTO = {
  generatedAt: string;
  activeFloorId: "floor-1" | "floor-2" | "floor-3";
  summary: RuntimeOverviewSummary;
  floors: FloorOverview[];
  teams: TeamOverview[];
  employees: EmployeePresence[];
  selectedEmployeeId?: string;
};

type RuntimeOverviewSummary = {
  teamCount: number;
  employeeCount: number;
  capacityUsed: number;
  capacityTotal: number;
  workingCount: number;
  idleCount: number;
  waitingHumanCount: number;
  queuedCount: number;
  errorCount: number;
  cumulativeTaskCount: number;
};

type FloorOverview = {
  floorId: "floor-1" | "floor-2" | "floor-3";
  label: string;
  teamIds: string[];
  summary: {
    teamCount: number;
    errorCount: number;
    capacityUsed: number;
    capacityTotal: number;
  };
  layout: FloorLayout;
};

type FloorLayout = {
  canvasWidth: number;
  canvasHeight: number;
  backgroundAssetId?: string;
  paths: MapPath[];
  teamWorkspaces: TeamWorkspaceLayout[];
};

type TeamWorkspaceLayout = {
  teamId: string;
  polygon: Array<{ x: number; y: number }>;
  cardAnchor: { x: number; y: number };
  labelAnchor?: { x: number; y: number };
  seats: TeamSeatLayout[];
  decorationVariant?: "standard" | "lab" | "ops" | "review" | "data";
};

type TeamSeatLayout = {
  seatId: string;
  x: number;
  y: number;
  deskVariant?: "single" | "row" | "corner";
  rotation?: number;
};

type TeamOverview = {
  teamId: string;
  floorId: "floor-1" | "floor-2" | "floor-3";
  name: string;
  capacity: 10;
  employeeCount: number;
  workingCount: number;
  idleCount: number;
  waitingHumanCount: number;
  queuedCount: number;
  errorCount: number;
  overCapacity: boolean;
};

type EmployeePresence = {
  employeeId: string;
  teamId: string;
  floorId: "floor-1" | "floor-2" | "floor-3";
  seatId?: string;
  name: string;
  roleLabel: string;
  avatarAsset?: {
    id: string;
    url?: string;
    fallbackLabel?: string;
  };
  status: "working" | "idle" | "waiting_human" | "queued" | "error" | "unavailable" | "needs_configuration";
  currentTask?: {
    taskId: string;
    title: string;
    priority?: "low" | "medium" | "high";
  };
  runtime?: {
    nodeId?: string;
    providerType?: string;
    sessionId?: string;
  };
  recentEvents: RuntimeOverviewEvent[];
  artifacts: RuntimeOverviewArtifact[];
  usage?: {
    taskTokens?: number;
    dailyTokens?: number;
    dailyTokenLimit?: number;
  };
};
```

## 8. 渲染层设计

### 8.1 画布坐标

- 每个楼层定义固定逻辑尺寸，例如 `1200 x 760`。
- 所有团队、工位、路径和装饰都使用逻辑坐标。
- 容器尺寸变化时计算 `scale = min(containerWidth / canvasWidth, containerHeight / canvasHeight)`。
- 地图缩放、拖拽和适应视图只改变 viewport transform，不改变业务坐标。
- 楼层背景、SVG 层和 HTML 数据层必须共享同一个逻辑坐标系，避免不同分辨率下出现连接线与头像错位。

### 8.2 图层顺序

从下到上：

1. 楼层背景层：gpt-image2 或手工绘制的干净 2.5D 背景素材。
2. SVG 路径层：楼层动线、团队边界、任务流转虚线、选中区域。
3. HTML 工位层：固定 10 个工位、空桌、空闲容量提示。
4. HTML 员工层：头像、状态点、选中光圈、告警标记。
5. HTML 浮层：团队卡片、工具条、右侧详情。
6. 交互命中层：员工、工位和团队区域的按钮或可访问元素。

团队卡片、头像和状态标记使用 HTML 渲染，便于复用项目字体、状态 pill、Tooltip、键盘可访问性和响应式文本处理。SVG 只表达线、边界和路径，不承载业务文本。

### 8.3 状态视觉

- `working`：绿色状态点。
- `idle`：灰色状态点或空闲态。
- `waiting_human`：橙色状态点。
- `queued`：蓝色状态点。
- `error`：红色状态点和小型告警标记。
- `unavailable` / `needs_configuration`：灰色弱化，同时在详情中显示原因。

选中员工使用蓝色环形高亮，参考图中高秀英的选中效果。选中团队卡片使用蓝色描边。

## 9. 响应式策略

- 桌面宽屏：左侧导航 + 中间地图 + 右侧概况/详情完整展示。
- 中等宽度：右侧详情可折叠，地图优先保持比例。
- 小屏：切换为分视图，地图、团队列表、员工详情分 Tab 展示。
- 地图不按 CSS 断点重新排座；只等比缩放或进入小屏替代视图。
- 参考图的 1536 x 1024 作为视觉验收主视口，同时验证 1366 x 768、1440 x 900、1920 x 1080。
- 响应式验证必须确认楼层背景、SVG 连线和 HTML 头像/卡片在缩放后仍然对齐。

## 10. 资产生成策略

允许使用 gpt-image2 生成以下资产：

- 不含文字、不含头像、不含业务数字的楼层基础视觉图。
- 办公家具、墙体、地面、绿植、门、看板等干净装饰资产。
- 头像 fallback 或占位图，但真实数字员工头像优先使用系统 avatar asset。

禁止生成或截取以下资产作为生产图层：

- 带团队名称、人数、容量、状态的团队卡片。
- 带员工姓名、任务、日志、证据、按钮的右侧详情。
- 带真实状态点或真实头像位置的整张运行总览截图。

## 11. 前端组件边界

建议拆分：

- `RuntimeOverviewPage`：路由页，负责查询、筛选、楼层状态和主布局。
- `RuntimeOverviewToolbar`：视图切换、楼层按钮、异常优先。
- `RuntimeMapStage`：地图根组件，负责逻辑坐标、缩放、拖拽和图层组合。
- `FloorBackgroundLayer`：当前楼层 2.5D 背景素材。
- `RuntimeMapSvgLayer`：路径、连接线、选中团队边界和流转线。
- `TeamWorkspaceRenderer`：团队办公区、10 个工位、空位提示。
- `EmployeeAvatarNode`：头像、状态点、选中态、告警态。
- `TeamSummaryCard`：团队浮层卡片。
- `RuntimeOverviewSidePanel`：右侧运行概况和员工详情。
- `RuntimeOverviewTable`：表格视图 fallback。

地图组件只接收 DTO 和事件回调，不直接请求接口；接口、缓存和权限处理留在页面层。

状态管理采用 React Query 管服务端读模型，React state 管当前楼层、选中员工、缩放和本地 UI 状态。第一版不引入 Redux；只有当地图交互状态跨多个页面或复杂编辑流程时，再评估 Zustand。

## 12. 后端与数据接入要求

生产接入需要 Control Plane 提供运行总览聚合读模型，至少包含：

- 楼层和团队归属。
- 团队成员和固定容量 10。
- 员工 operational status、原因和可调度状态。
- 当前任务、任务优先级、任务节点状态。
- Runtime node、provider session、command channel 或执行摘要。
- 最近日志、证据工件和消耗统计。

服务端必须校验每个团队最多 10 个数字员工。前端也要做展示保护，但不能作为唯一约束。

运行总览、流程编排和项目管理应共享同一份 Control Plane 聚合读模型。运行总览只是同源数据的地图视图，不新增独立事实源，也不直接读取 Redis 作为业务事实源。

第一版刷新策略为 polling-first：前端通过 React Query 定期拉取 `RuntimeOverviewDTO`，建议间隔 5-10 秒，并提供手动刷新。SSE 仅作为后续增强，用于秒级事件流或日志局部刷新。WebSocket 不是本页面第一版目标，保留给 Runtime Agent / Provider 命令通道或未来双向协作场景。

## 13. 异常与空状态

- 无团队：地图显示空楼层，右侧概况为 0，并提供去团队管理的入口。
- 楼层无团队：当前楼层显示空办公区，不复制其他楼层数据。
- 员工无座位：在团队卡片和右侧列表中标记“待分配工位”，不随机摆放。
- 团队满员：团队卡片显示 `10/10`，新增入口禁用或提示先移出成员。
- 团队超限：显示错误态，不渲染第 11 人进地图，并在右侧概况显示数据异常。
- Runtime 离线：员工状态按 Control Plane operational status 展示，不在前端自行把所有员工改成异常。

## 14. 从当前原型迁移

当前原型可复用：

- 参考图作为视觉对照和验收基准。
- `employeePositions` 的坐标思想。
- 当前 Konva 绘制尝试中的墙体、桌子、地面、路径坐标可以转化为楼层素材提示、SVG 路径或 HTML 工位坐标，但不作为第一版主渲染引擎。
- Chrome 截图与像素 diff 的 QA 方法。

需要替换：

- `reference.png` 整图底层必须移除出生产渲染路径。
- 透明热点层改为真实头像和团队区域 hit area。
- teamCards mock 改为 RuntimeOverviewDTO 聚合数据。
- 右侧详情从截图内容改为真实选中员工数据。

## 15. 验证标准

视觉验收：

- 在 1536 x 1024 下，整体结构和参考图高度一致，且左下需求输入卡片已移除。
- 每个团队区域最多出现 10 个工位。
- 1/2/3 层切换后布局可区分，不出现错位或文本重叠。
- 选中员工、异常员工、空工位、团队满员状态清晰可见。

功能验收：

- 新增员工占用空 seat。
- 删除员工释放 seat，其他员工位置不跳动。
- 状态变化只更新视觉状态和详情，不重排地图。
- 团队达到 10 人后新增被阻止或显示业务拒绝态。
- 员工无 seat 时进入待分配展示。

工程验收：

- 地图组件不依赖整张截图。
- 可见业务文本由 React 数据渲染。
- 楼层背景、SVG 连线和 HTML 头像/卡片在不同分辨率下保持对齐。
- 刷新策略使用同源 Control Plane 聚合读模型，第一版不依赖 SSE/WebSocket 才能工作。
- 浏览器验证至少覆盖 1536 x 1024、1366 x 768、1920 x 1080。
- 若接入真实 API，必须通过当前项目真实 Web + Control Plane 数据链路验证，不能只用 mock 声明完成。
