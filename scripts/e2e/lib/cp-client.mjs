/**
 * Shared Control Plane HTTP client for casting E2E suites.
 * CP URL / credentials from env with safe defaults.
 */
export const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
export const WEB = process.env.SUPERTEAM_WEB_URL || "http://127.0.0.1:3100";
export const USER = process.env.SUPERTEAM_USER || "admin";
export const PASS = process.env.SUPERTEAM_PASS || "admin";

export async function login(cp = CP, username = USER, password = PASS) {
  const res = await fetch(`${cp}/api/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    throw new Error(`login ${res.status}`);
  }
  const setCookie = res.headers.getSetCookie?.() || [];
  const raw = res.headers.get("set-cookie") || "";
  const parts = setCookie.length ? setCookie : raw ? [raw] : [];
  return parts
    .map((c) => c.split(";")[0].trim())
    .filter(Boolean)
    .join("; ");
}

export async function api(cookie, path, { method = "GET", body, cp = CP } = {}) {
  const res = await fetch(`${cp}${path}`, {
    method,
    headers: {
      cookie,
      accept: "application/json",
      "content-type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  return { ok: res.ok, status: res.status, json, text };
}

export async function apiOk(cookie, path, opts = {}) {
  const r = await api(cookie, path, opts);
  if (!r.ok) {
    throw new Error(
      `${opts.method || "GET"} ${path} -> ${r.status} ${String(r.text).slice(0, 400)}`,
    );
  }
  return r.json;
}

export function assert(cond, msg) {
  if (!cond) throw new Error(msg || "assertion failed");
}
