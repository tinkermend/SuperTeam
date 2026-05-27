import type { HealthResponse } from "@superteam/api-client";

export function createHealthSummary(health: HealthResponse): string {
  return `${health.service} is ${health.status}`;
}

