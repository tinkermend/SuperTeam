import type { AutomationScheduleKind } from "@/lib/api/automations";

export type ScheduleNextInput = {
  enabled: boolean;
  schedule_kind: AutomationScheduleKind;
  cron_expr?: string | null;
  interval_seconds?: number | null;
  timezone?: string;
  latest_fire?: { scheduled_fire_at: string } | null;
};

type ZonedParts = {
  minute: number;
  hour: number;
  day: number;
  month: number;
  dow: number; // 0=Sun … 6=Sat
};

function zonedParts(date: Date, timeZone: string): ZonedParts {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    // h23 keeps midnight as 0, avoiding rare "24" hour parts.
    hourCycle: "h23",
    weekday: "short",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).formatToParts(date);

  const get = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";

  const weekday = get("weekday");
  const dowMap: Record<string, number> = {
    Sun: 0,
    Mon: 1,
    Tue: 2,
    Wed: 3,
    Thu: 4,
    Fri: 5,
    Sat: 6,
  };

  const rawHour = get("hour");
  // Defensive: some engines still emit "24" for midnight; treat as 0 on the
  // same calendar day parts Intl already returned (do not invent a day bump).
  const hour = Number(rawHour === "24" ? "0" : rawHour);

  return {
    minute: Number(get("minute")),
    hour,
    day: Number(get("day")),
    month: Number(get("month")),
    dow: dowMap[weekday] ?? 0,
  };
}

function expandField(field: string, min: number, max: number): Set<number> {
  const out = new Set<number>();
  for (const piece of field.split(",")) {
    const stepMatch = /^(.+)\/(\d+)$/.exec(piece);
    const body = stepMatch ? stepMatch[1]! : piece;
    const step = stepMatch ? Number(stepMatch[2]) : 1;
    if (!Number.isFinite(step) || step < 1) continue;

    let start = min;
    let end = max;
    if (body === "*") {
      // full range
    } else if (body.includes("-")) {
      const [a, b] = body.split("-").map(Number);
      if (!Number.isFinite(a) || !Number.isFinite(b)) continue;
      start = a as number;
      end = b as number;
    } else if (body !== "") {
      const n = Number(body);
      if (!Number.isFinite(n)) continue;
      start = n;
      end = n;
    } else {
      // Empty piece from malformed cron (e.g. trailing comma) — skip.
      continue;
    }

    for (let value = start; value <= end; value += step) {
      if (value >= min && value <= max) out.add(value);
    }
  }
  return out;
}

function matchCron(expr: string, parts: ZonedParts): boolean {
  const fields = expr.trim().split(/\s+/);
  if (fields.length !== 5) return false;
  const [minute, hour, dom, month, dow] = fields as [
    string,
    string,
    string,
    string,
    string,
  ];
  const minutes = expandField(minute, 0, 59);
  const hours = expandField(hour, 0, 23);
  const days = expandField(dom, 1, 31);
  const months = expandField(month, 1, 12);
  // cron DOW: 0 or 7 = Sunday
  const dows = new Set<number>();
  for (const value of expandField(dow.replace(/7/g, "0"), 0, 6)) {
    dows.add(value);
  }

  if (!minutes.has(parts.minute) || !hours.has(parts.hour) || !months.has(parts.month)) {
    return false;
  }

  const domStar = dom === "*";
  const dowStar = dow === "*";
  if (domStar && dowStar) return true;
  if (!domStar && dowStar) return days.has(parts.day);
  if (domStar && !dowStar) return dows.has(parts.dow);
  // Both constrained: classic cron OR semantics
  return days.has(parts.day) || dows.has(parts.dow);
}

function nextCronFire(cronExpr: string, timeZone: string, from: Date): Date | null {
  // Start at the next whole minute to avoid matching "now".
  let cursor = new Date(Math.floor(from.getTime() / 60_000) * 60_000 + 60_000);
  const limit = from.getTime() + 366 * 86_400_000;
  while (cursor.getTime() <= limit) {
    if (matchCron(cronExpr, zonedParts(cursor, timeZone))) {
      return cursor;
    }
    cursor = new Date(cursor.getTime() + 60_000);
  }
  return null;
}

function nextIntervalFire(
  intervalSeconds: number,
  from: Date,
  latestFireAt?: string | null,
): Date | null {
  if (!Number.isFinite(intervalSeconds) || intervalSeconds < 60) return null;
  const stepMs = intervalSeconds * 1000;
  const lastMs = latestFireAt ? Date.parse(latestFireAt) : Number.NaN;
  if (!Number.isNaN(lastMs)) {
    const steps = Math.floor((from.getTime() - lastMs) / stepMs) + 1;
    return new Date(lastMs + Math.max(steps, 1) * stepMs);
  }
  return new Date(from.getTime() + stepMs);
}

/** Estimate next scheduled fire for display. Disabled rules return null. */
export function computeNextFireAt(
  rule: ScheduleNextInput,
  from: Date = new Date(),
): Date | null {
  if (!rule.enabled) return null;
  const timeZone = rule.timezone?.trim() || "Asia/Shanghai";
  if (rule.schedule_kind === "interval") {
    return nextIntervalFire(
      rule.interval_seconds ?? 0,
      from,
      rule.latest_fire?.scheduled_fire_at,
    );
  }
  const cron = rule.cron_expr?.trim();
  if (!cron) return null;
  return nextCronFire(cron, timeZone, from);
}

export function computeNextFireIso(
  rule: ScheduleNextInput,
  from: Date = new Date(),
): string | null {
  const next = computeNextFireAt(rule, from);
  return next ? next.toISOString() : null;
}

/** Precompute next-fire ISO by rule id (one scan pass for list/rail/detail). */
export function buildNextFireById(
  rules: Array<ScheduleNextInput & { id: string }>,
  from: Date = new Date(),
): Map<string, string> {
  const map = new Map<string, string>();
  for (const rule of rules) {
    const next = computeNextFireIso(rule, from);
    if (next) map.set(rule.id, next);
  }
  return map;
}
