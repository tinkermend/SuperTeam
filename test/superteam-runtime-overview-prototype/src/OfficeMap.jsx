import { Group, Layer, Line, Path, Rect, Stage } from "react-konva";

import avatar01 from "./assets/avatar-01.png";
import avatar02 from "./assets/avatar-02.png";
import avatar03 from "./assets/avatar-03.png";
import avatar04 from "./assets/avatar-04.png";
import avatar05 from "./assets/avatar-05.png";
import avatar06 from "./assets/avatar-06.png";
import avatar07 from "./assets/avatar-07.png";
import avatar08 from "./assets/avatar-08.png";
import avatar09 from "./assets/avatar-09.png";
import avatar10 from "./assets/avatar-10.png";
import avatar11 from "./assets/avatar-11.png";
import gaoxiuAvatarUrl from "./assets/gaoxiu-avatar.png";

const avatars = [
  avatar01,
  avatar02,
  avatar03,
  avatar04,
  avatar05,
  avatar06,
  avatar07,
  avatar08,
  avatar09,
  avatar10,
  avatar11,
];

export const employeePositions = [
  { id: "dev-1", name: "陆一鸣", role: "前端工程师 AI", teamId: "dev", x: 169, y: 142, avatar: 0, status: "working", task: "实现运行态组件" },
  { id: "dev-2", name: "沈嘉", role: "后端工程师 AI", teamId: "dev", x: 224, y: 170, avatar: 1, status: "working", task: "补齐任务 API 契约" },
  { id: "dev-3", name: "郑可", role: "架构工程师 AI", teamId: "dev", x: 278, y: 198, avatar: 2, status: "working", task: "校验模块边界" },
  { id: "test-1", name: "赵宁", role: "测试工程师 AI", teamId: "test", x: 456, y: 158, avatar: 3, status: "working", task: "回归关键流程" },
  { id: "test-2", name: "罗岚", role: "质量分析 AI", teamId: "test", x: 516, y: 190, avatar: 4, status: "working", task: "汇总失败样例" },
  { id: "ops-lead", name: "高秀英", role: "运维工程师 AI", teamId: "ops", x: 680, y: 210, avatar: "gaoxiu", status: "working", active: true, task: "排查线上告警并生成修复计划" },
  { id: "ops-2", name: "梁程", role: "SRE AI", teamId: "ops", x: 746, y: 244, avatar: 5, status: "working", task: "关联日志与指标" },
  { id: "ops-3", name: "陈骁", role: "发布工程师 AI", teamId: "ops", x: 807, y: 274, avatar: 6, status: "working", task: "检查发布窗口" },
  { id: "ops-4", name: "季敏", role: "告警分析 AI", teamId: "ops", x: 864, y: 214, avatar: 7, status: "alert", task: "定位异常实例" },
  { id: "ops-5", name: "许越", role: "容量工程师 AI", teamId: "ops", x: 922, y: 242, avatar: 8, status: "working", task: "评估容量水位" },
  { id: "security-1", name: "杨钧", role: "安全审计 AI", teamId: "security", x: 112, y: 428, avatar: 9, status: "alert", task: "确认高危权限变更" },
  { id: "security-2", name: "唐书", role: "策略分析 AI", teamId: "security", x: 170, y: 458, avatar: 10, status: "working", task: "生成策略差异报告" },
  { id: "security-3", name: "周遥", role: "权限治理 AI", teamId: "security", x: 224, y: 488, avatar: 4, status: "pending", task: "等待人工确认" },
  { id: "product-1", name: "秦沐", role: "产品分析 AI", teamId: "product", x: 450, y: 446, avatar: 0, status: "working", task: "整理用户反馈" },
  { id: "product-2", name: "苏芮", role: "需求分析 AI", teamId: "product", x: 507, y: 476, avatar: 1, status: "working", task: "拆分验收标准" },
  { id: "product-3", name: "何砚", role: "交互分析 AI", teamId: "product", x: 564, y: 506, avatar: 2, status: "working", task: "更新流程说明" },
  { id: "data-1", name: "韩知", role: "数据工程师 AI", teamId: "data", x: 734, y: 508, avatar: 3, status: "queue", task: "等待数据同步" },
  { id: "data-2", name: "林澈", role: "指标分析 AI", teamId: "data", x: 792, y: 538, avatar: 4, status: "queue", task: "排队生成报表" },
  { id: "data-3", name: "魏秋", role: "数据治理 AI", teamId: "data", x: 850, y: 568, avatar: 5, status: "working", task: "检查数据质量" },
];

const rooms = [
  {
    id: "dev",
    floor: [26, 182, 170, 104, 327, 188, 179, 276],
    walls: [
      { points: [30, 136, 120, 84, 120, 114, 31, 166] },
      { points: [318, 142, 318, 193, 275, 217, 275, 163] },
      { points: [124, 83, 206, 129, 206, 156, 124, 110] },
    ],
    desks: [
      { x: 96, y: 180 },
      { x: 146, y: 206 },
      { x: 194, y: 232 },
      { x: 152, y: 142 },
      { x: 206, y: 166 },
      { x: 256, y: 193 },
    ],
    plant: [330, 214],
  },
  {
    id: "test",
    floor: [352, 171, 471, 103, 622, 183, 502, 251],
    walls: [
      { points: [354, 134, 420, 96, 420, 126, 354, 165] },
      { points: [618, 139, 618, 185, 581, 207, 581, 160] },
    ],
    desks: [
      { x: 430, y: 166 },
      { x: 486, y: 194 },
    ],
    plant: [624, 164],
  },
  {
    id: "ops",
    floor: [633, 176, 785, 88, 956, 183, 801, 280],
    walls: [
      { points: [637, 129, 748, 63, 748, 101, 637, 166] },
      { points: [948, 140, 948, 215, 894, 247, 894, 172] },
      { points: [760, 66, 873, 129, 873, 168, 760, 104] },
    ],
    board: [684, 151],
    desks: [
      { x: 725, y: 177 },
      { x: 783, y: 207 },
      { x: 842, y: 236 },
      { x: 867, y: 167 },
      { x: 913, y: 191 },
      { x: 820, y: 133 },
      { x: 870, y: 112 },
      { x: 914, y: 137 },
    ],
    plant: [957, 209],
  },
  {
    id: "security",
    floor: [27, 448, 161, 372, 306, 448, 171, 528],
    walls: [
      { points: [32, 398, 93, 363, 93, 405, 31, 440] },
      { points: [301, 407, 301, 452, 260, 477, 260, 431] },
    ],
    desks: [
      { x: 112, y: 430 },
      { x: 167, y: 459 },
      { x: 217, y: 487 },
    ],
  },
  {
    id: "product",
    floor: [356, 451, 496, 373, 646, 453, 506, 534],
    walls: [
      { points: [356, 412, 449, 358, 449, 394, 356, 447] },
      { points: [641, 410, 641, 451, 598, 477, 598, 433] },
    ],
    desks: [
      { x: 455, y: 446 },
      { x: 505, y: 473 },
      { x: 552, y: 498 },
    ],
  },
  {
    id: "data",
    floor: [638, 502, 770, 428, 911, 503, 778, 584],
    walls: [
      { points: [639, 462, 711, 421, 711, 459, 639, 500] },
      { points: [906, 463, 906, 512, 862, 538, 862, 488] },
    ],
    desks: [
      { x: 738, y: 500 },
      { x: 789, y: 527 },
      { x: 838, y: 554 },
    ],
    plant: [908, 560],
  },
];

export function getEmployeeAvatarUrl(employee) {
  return employee.avatar === "gaoxiu" ? gaoxiuAvatarUrl : avatars[employee.avatar % avatars.length];
}

function IsoFloor({ points }) {
  return (
    <Line
      points={points}
      closed
      fill="#f8fafc"
      stroke="#cdd5df"
      strokeWidth={1}
      dash={[5, 4]}
      shadowColor="#d7dde7"
      shadowBlur={6}
      shadowOpacity={0.22}
    />
  );
}

function Wall({ points }) {
  return (
    <Line
      points={points}
      closed
      fillLinearGradientStartPoint={{ x: 0, y: 0 }}
      fillLinearGradientEndPoint={{ x: 0, y: 90 }}
      fillLinearGradientColorStops={[0, "#f9fbfe", 1, "#e9eef5"]}
      stroke="#d9e1eb"
      strokeWidth={1}
      shadowColor="#cfd6df"
      shadowBlur={5}
      shadowOpacity={0.18}
    />
  );
}

function Desk({ x, y }) {
  return (
    <Group x={x} y={y}>
      <Line points={[0, 17, 55, -14, 98, 9, 43, 42]} closed fill="#f9fafc" stroke="#d7dee8" strokeWidth={1} />
      <Line points={[0, 17, 43, 42, 43, 50, 0, 25]} closed fill="#e8edf4" stroke="#d7dee8" strokeWidth={1} />
      <Line points={[55, -14, 98, 9, 98, 17, 43, 50, 43, 42, 98, 9]} closed fill="#eef2f7" stroke="#d7dee8" strokeWidth={1} />
      <Line points={[22, 32, 22, 52]} stroke="#c5ced9" strokeWidth={2} />
      <Line points={[76, 22, 76, 42]} stroke="#c5ced9" strokeWidth={2} />
      <Group x={42} y={-18}>
        <Rect width={30} height={22} fill="#33485b" stroke="#d9e1eb" strokeWidth={1} cornerRadius={2} />
        <Rect x={4} y={4} width={22} height={11} fill="#6a94b9" opacity={0.65} cornerRadius={1} />
        <Line points={[15, 22, 15, 30]} stroke="#9aa6b2" strokeWidth={2} />
      </Group>
    </Group>
  );
}

function Plant({ x, y }) {
  if (!x || !y) return null;
  return (
    <Group x={x} y={y}>
      <Rect x={-7} y={18} width={15} height={15} fill="#d9e2ea" stroke="#cbd5df" cornerRadius={3} />
      <Line points={[0, 22, -14, 3]} stroke="#7aa366" strokeWidth={3} lineCap="round" />
      <Line points={[0, 22, 11, -4]} stroke="#8ab774" strokeWidth={3} lineCap="round" />
      <Line points={[0, 22, 4, 2]} stroke="#6f9b5f" strokeWidth={3} lineCap="round" />
    </Group>
  );
}

function Board({ position }) {
  if (!position) return null;
  const [x, y] = position;
  return (
    <Group x={x} y={y}>
      <Line points={[0, 14, 44, -12, 44, 20, 0, 46]} closed fill="#31576d" stroke="#9fb6c5" strokeWidth={1} />
      <Line points={[8, 17, 36, 0]} stroke="#36c2d2" strokeWidth={2} />
      <Line points={[8, 27, 31, 14]} stroke="#75d27f" strokeWidth={2} />
    </Group>
  );
}

function Room({ room }) {
  return (
    <Group>
      <IsoFloor points={room.floor} />
      {room.walls.map((wall, index) => <Wall key={`${room.id}-wall-${index}`} points={wall.points} />)}
      <Board position={room.board} />
      {room.desks.map((desk, index) => <Desk key={`${room.id}-desk-${index}`} {...desk} />)}
      <Plant x={room.plant?.[0]} y={room.plant?.[1]} />
    </Group>
  );
}

function GridBackground() {
  const lines = [];
  for (let i = -220; i < 1120; i += 38) {
    lines.push(<Line key={`a-${i}`} points={[i, 690, i + 540, 380]} stroke="#eef2f6" strokeWidth={1} opacity={0.65} />);
    lines.push(<Line key={`b-${i}`} points={[i, 392, i + 536, 700]} stroke="#eef2f6" strokeWidth={1} opacity={0.65} />);
  }
  return <Group>{lines}</Group>;
}

export function OfficeMap({ width = 970, height = 687 }) {
  return (
    <Stage width={width} height={height} className="office-stage">
      <Layer listening={false} name="office-map-base">
        <Rect width={width} height={height} fill="#fbfcfe" />
        <GridBackground />
        <Path
          data="M 274 278 C 330 328, 468 324, 530 272 S 668 218, 745 241"
          stroke="#1677ff"
          strokeWidth={2}
          dash={[7, 5]}
          lineCap="round"
          opacity={0.85}
        />
        {rooms.map((room) => <Room key={room.id} room={room} />)}
        <Group x={476} y={600}>
          <Line points={[0, 44, 74, 0, 150, 45, 76, 88]} closed fill="#edf2f7" stroke="#d7dfe9" />
          <Line points={[30, 48, 75, 22, 75, 82, 30, 108]} closed fill="#f8fafc" stroke="#cfd8e3" />
          <Line points={[78, 21, 120, 46, 120, 106, 78, 82]} closed fill="#eef3f8" stroke="#cfd8e3" />
        </Group>
        <Plant x={472} y={633} />
        <Plant x={611} y={633} />
      </Layer>
    </Stage>
  );
}
