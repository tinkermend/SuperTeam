/**
 * 浏览器 E2E 共享层：登录 + 控制台错误采集。
 *
 * 存在的理由是两条反复踩的坑，不该每个脚本重新踩一遍：
 *
 * 1. **登录必须用 placeholder 选择器，且登录后要等**。表单里没有 name/id，
 *    `input[type=text]` 会命中错的元素；`waitForURL` 返回时会话还没完全落地，
 *    紧接着的硬导航会被弹回 /sign-in（本文件的 login 已经把这两点封好）。
 * 2. **`/api/auth/me` 的 401 是正常语义，不是缺陷**。会话 cookie 是 httpOnly，
 *    前端读不到，只能问服务端"我有会话吗"，未登录时答案就是 401；而
 *    `auth-provider` 的启动探测是裸 useEffect，开发模式下被 StrictMode 双调，
 *    于是每次应用启动固定产生两条红色控制台错误（生产构建实测只发一次）。
 *    "控制台零错误"断言必须把它排掉，否则永远绿不了。
 */
import { WEB, USER, PASS } from "./cp-client.mjs";

/** 已知良性的控制台噪音：命中任一条即不计入真实错误。 */
export const BENIGN_CONSOLE_PATTERNS = [
  // 未登录时的会话探测（httpOnly cookie 决定了它只能问服务端），
  // 且 StrictMode 在开发模式下双调启动 effect ⇒ 每次启动两条。
  // 采集侧已把 location().url 拼进字符串，所以按 URL 匹配即可。
  /\/api\/auth\/me\b/,
  // 常驻 SSE（收件箱、运行总览）在每次页面导航时被浏览器 abort，
  // 是导航的正常副作用，不是失败。
  /requestfailed: net::ERR_ABORTED .*\/(stream|events)\b/,
];

/** 从原始控制台错误里滤掉已知良性噪音。 */
export function realConsoleErrors(errors, extraPatterns = []) {
  const patterns = [...BENIGN_CONSOLE_PATTERNS, ...extraPatterns];
  return (errors ?? []).filter((e) => !patterns.some((p) => p.test(e)));
}

/**
 * 起一个已登录的页面，并挂好控制台错误采集。
 *
 * @returns {Promise<{browser, context, page, consoleErrors: string[], realErrors: (extra?: RegExp[]) => string[]}>}
 */
export async function launchLoggedIn({
  chromium,
  web = WEB,
  username = USER,
  password = PASS,
  headless = true,
  viewport = { width: 1440, height: 900 },
} = {}) {
  if (!chromium) {
    throw new Error("launchLoggedIn 需要传入 playwright 的 chromium（调用方自行 require，避免本文件绑定解析路径）");
  }
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();

  const consoleErrors = [];
  page.on("console", (m) => {
    if (m.type() !== "error") return;
    // 关键：资源类错误的 text 里**没有 URL**（"Failed to load resource: ... 401"），
    // URL 只在 location() 里。不拼上去，按 URL 过滤的规则就永远命中不了。
    const url = m.location?.()?.url ?? "";
    consoleErrors.push(url ? `${m.text()} @ ${url}` : m.text());
  });
  // 同源的第二个来源：请求本身失败时不一定产生 console 事件，补一条便于排查。
  page.on("requestfailed", (req) => {
    consoleErrors.push(`requestfailed: ${req.failure()?.errorText ?? "unknown"} @ ${req.url()}`);
  });

  await page.goto(`${web}/sign-in`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(1500);
  await page.getByPlaceholder("请输入账号").fill(username);
  await page.getByPlaceholder("请输入密码").fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForURL((u) => !u.pathname.includes("sign-in"), { timeout: 30000 });
  // 会话落地缓冲：省了这一步，紧接着的硬导航会被弹回登录页（真踩过两次）。
  await page.waitForTimeout(1500);

  return {
    browser,
    context,
    page,
    consoleErrors,
    realErrors: (extra = []) => realConsoleErrors(consoleErrors, extra),
  };
}

/**
 * 不要用 `waitUntil: "networkidle"`：收件箱/运行总览有常驻 SSE，
 * networkidle 永远不会触发，只会等到超时。
 */
export const SAFE_WAIT_UNTIL = "domcontentloaded";
