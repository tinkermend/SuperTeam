import React, { useState } from "react";
import "./styles.css";
import { employeePositions } from "./OfficeMap.jsx";

import referenceUrl from "./assets/reference.png";

const teamNames = {
  dev: "开发团队",
  test: "测试团队",
  ops: "运维团队",
  security: "安全团队",
  product: "产品团队",
  data: "数据团队",
};

const statusLabels = {
  working: "工作中",
  alert: "异常",
  pending: "待确认",
  queue: "排队",
  idle: "空闲",
};

const teamCards = [
  {
    id: "dev",
    name: "开发团队",
    people: 24,
    capacity: "9/24",
    abnormal: 1,
    working: 7,
    queue: 0,
    pending: 1,
    idle: 15,
    pos: "dev",
  },
  {
    id: "test",
    name: "测试团队",
    people: 12,
    capacity: "5/12",
    abnormal: 0,
    working: 4,
    queue: 1,
    pending: 0,
    idle: 7,
    pos: "test",
  },
  {
    id: "ops",
    name: "运维团队",
    people: 18,
    capacity: "7/18",
    abnormal: 1,
    working: 5,
    queue: 1,
    pending: 0,
    idle: 11,
    pos: "ops",
    active: true,
  },
  {
    id: "security",
    name: "安全团队",
    people: 10,
    capacity: "4/10",
    abnormal: 1,
    working: 2,
    queue: 0,
    pending: 1,
    idle: 6,
    pos: "security",
  },
  {
    id: "product",
    name: "产品团队",
    people: 8,
    capacity: "3/8",
    abnormal: 0,
    working: 2,
    queue: 0,
    pending: 1,
    idle: 5,
    pos: "product",
  },
  {
    id: "data",
    name: "数据团队",
    people: 9,
    capacity: "3/9",
    abnormal: 0,
    working: 2,
    queue: 1,
    pending: 0,
    idle: 6,
    pos: "data",
  },
];

const teamHotspots = {
  dev: { x: 228, y: 244, width: 214, height: 88 },
  test: { x: 517, y: 244, width: 216, height: 88 },
  ops: { x: 804, y: 244, width: 236, height: 88 },
  security: { x: 236, y: 548, width: 216, height: 88 },
  product: { x: 538, y: 536, width: 216, height: 88 },
  data: { x: 817, y: 601, width: 207, height: 88 },
};

function VisualLockPrototype() {
  const [selectedEmployee, setSelectedEmployee] = useState(
    employeePositions.find((employee) => employee.active) ?? employeePositions[0],
  );
  const [selectedTeam, setSelectedTeam] = useState(selectedEmployee.teamId);
  const selectedTeamName = teamNames[selectedTeam];

  return (
    <main className="visual-lock-shell" aria-label="SuperTeam 运行总览视觉锁定原型">
      <img className="visual-lock-reference" src={referenceUrl} alt="SuperTeam 运行总览参考视觉" />
      <div className="visual-hotspot-layer" aria-label="数据驱动透明热点层">
        {employeePositions.map((employee) => (
          <button
            key={employee.id}
            className="visual-hotspot employee-hotspot"
            type="button"
            style={{ left: `${199 + employee.x - 22}px`, top: `${243 + employee.y - 22}px` }}
            onClick={() => {
              setSelectedEmployee(employee);
              setSelectedTeam(employee.teamId);
            }}
            aria-label={`${employee.name}，${teamNames[employee.teamId]}，${statusLabels[employee.status]}`}
            data-employee-id={employee.id}
            data-team-id={employee.teamId}
            data-status={employee.status}
            data-x={employee.x}
            data-y={employee.y}
          />
        ))}
        {teamCards.map((team) => {
          const box = teamHotspots[team.id];
          return (
            <button
              key={team.id}
              className="visual-hotspot team-hotspot"
              type="button"
              style={{ left: `${box.x}px`, top: `${box.y}px`, width: `${box.width}px`, height: `${box.height}px` }}
              onClick={() => setSelectedTeam(team.id)}
              aria-label={`${team.name}，容量 ${team.capacity}`}
              data-team-id={team.id}
            />
          );
        })}
      </div>
      <output
        className="sr-only"
        data-selected-employee={selectedEmployee.id}
        data-selected-team={selectedTeam}
      >
        当前选择：{selectedEmployee.name}，{selectedTeamName}
      </output>
    </main>
  );
}

export function App() {
  return <VisualLockPrototype />;
}
