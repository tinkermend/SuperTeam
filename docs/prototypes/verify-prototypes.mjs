#!/usr/bin/env node
/**
 * 原型验证门禁：递归扫描 docs/prototypes/ 下全部 HTML 原型，起本地静态服务 +
 * Playwright 截图 + console/布局检查。
 *
 * 历史：原脚本在 docs/prototypes/design-system/verify-prototypes.mjs，v3 迁移
 * (68a72c23) 删掉整个 design-system 子目录后 package.json 指向断链。本文件恢复
 * 门禁，扫描范围扩到整个 docs/prototypes/（覆盖 flow-live-graph 等新原型）。
 *
 * 用法：corepack pnpm verify:design-prototypes
 * 不跑 npx playwright install；浏览器解析路径与仓库既有 web 测试一致。
 */

import { createServer } from "node:http";
import { createRequire } from "node:module";
import { once } from "node:events";
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const prototypeRoot = scriptDir; // docs/prototypes/
const repoRoot = path.resolve(scriptDir, "../..");
const screenshotDir = path.join(prototypeRoot, "__screenshots__");

const viewports = [
  { name: "desktop", width: 1280, height: 720 },
  { name: "mobile", width: 390, height: 844 },
];

// 允许跳过的路径段（生成物/截图/备份）。
const SKIP_DIR_NAMES = new Set([
  "__screenshots__",
  "node_modules",
  ".git",
  "dist",
  "build",
]);

/**
 * 布局严检（overflow/offscreen/无障碍图标按钮）仅对显式目录生效。
 * 历史概念稿大多桌面优先、从未做移动端，硬拦布局会让门禁永久红；
 * 全部页面仍做 HTTP + console error + pageerror 硬检。
 * 新原型目录加入本集合，或在目录下放空文件 `.prototype-strict-gate`。
 */
const STRICT_LAYOUT_DIRS = new Set(["flow-live-graph"]);

const STRICT_ALL = process.argv.includes("--strict-all");

function isStrictLayoutPage(pageName) {
  if (STRICT_ALL) return true;
  const top = pageName.split("/")[0];
  if (STRICT_LAYOUT_DIRS.has(top)) return true;
  // directory marker: docs/prototypes/<dir>/.prototype-strict-gate
  return false;
}

async function discoverStrictMarkerDirs() {
  const entries = await fs.readdir(prototypeRoot, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory() || SKIP_DIR_NAMES.has(entry.name)) continue;
    try {
      await fs.access(path.join(prototypeRoot, entry.name, ".prototype-strict-gate"));
      STRICT_LAYOUT_DIRS.add(entry.name);
    } catch {
      // no marker
    }
  }
}

const mimeTypes = new Map([
  [".html", "text/html; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".mjs", "text/javascript; charset=utf-8"],
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".svg", "image/svg+xml"],
  [".woff2", "font/woff2"],
  [".woff", "font/woff"],
]);

function relativeToRepo(filePath) {
  return path.relative(repoRoot, filePath);
}

async function discoverPrototypePages(dir = prototypeRoot, base = "") {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  const pages = [];
  for (const entry of entries) {
    if (entry.name.startsWith(".")) continue;
    if (entry.isDirectory()) {
      if (SKIP_DIR_NAMES.has(entry.name)) continue;
      pages.push(
        ...(await discoverPrototypePages(
          path.join(dir, entry.name),
          path.posix.join(base, entry.name),
        )),
      );
      continue;
    }
    if (!entry.isFile()) continue;
    if (!entry.name.endsWith(".html")) continue;
    if (entry.name.startsWith("_")) continue;
    pages.push(path.posix.join(base, entry.name));
  }
  return pages.sort();
}

function loadPlaywright() {
  const require = createRequire(import.meta.url);
  const searchRoots = [repoRoot, path.join(repoRoot, "apps/web")];
  const tried = [];

  for (const root of searchRoots) {
    try {
      const resolved = require.resolve("playwright", { paths: [root] });
      return require(resolved);
    } catch (error) {
      tried.push(`${root}: ${error.code ?? error.message}`);
    }
  }

  throw new Error(
    [
      "Cannot resolve the existing Playwright dependency.",
      "Run from the repository checkout after dependencies are installed.",
      "Search paths:",
      ...tried.map((entry) => `- ${entry}`),
    ].join("\n"),
  );
}

function createStaticServer(defaultPage) {
  return createServer(async (request, response) => {
    try {
      const requestUrl = new URL(request.url ?? "/", "http://127.0.0.1");
      if (requestUrl.pathname === "/favicon.ico") {
        response.writeHead(204);
        response.end();
        return;
      }

      const pathname =
        requestUrl.pathname === "/"
          ? `/${defaultPage}`
          : decodeURIComponent(requestUrl.pathname);
      const filePath = path.resolve(prototypeRoot, `.${pathname}`);

      if (
        filePath !== prototypeRoot &&
        !filePath.startsWith(`${prototypeRoot}${path.sep}`)
      ) {
        response.writeHead(403);
        response.end("Forbidden");
        return;
      }

      const stat = await fs.stat(filePath);
      if (!stat.isFile()) {
        response.writeHead(404);
        response.end("Not found");
        return;
      }

      response.setHeader(
        "Content-Type",
        mimeTypes.get(path.extname(filePath)) ?? "application/octet-stream",
      );
      response.end(await fs.readFile(filePath));
    } catch (error) {
      if (error.code === "ENOENT") {
        response.writeHead(404);
        response.end("Not found");
        return;
      }

      response.writeHead(500);
      response.end(error.message);
    }
  });
}

async function listen(server) {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  return server.address().port;
}

async function close(server) {
  if (!server.listening) {
    return;
  }

  await new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}

async function findSystemChromium() {
  const candidates = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
  ];

  for (const candidate of candidates) {
    try {
      await fs.access(candidate);
      return candidate;
    } catch {
      // Keep probing common local browser locations.
    }
  }

  return null;
}

async function launchChromium(chromium) {
  const launchErrors = [];

  try {
    return await chromium.launch({ headless: true, timeout: 15_000 });
  } catch (error) {
    launchErrors.push(
      `bundled Playwright Chromium: ${error.message.split("\n")[0]}`,
    );
  }

  const executablePath = await findSystemChromium();
  if (executablePath) {
    try {
      return await chromium.launch({
        executablePath,
        headless: true,
        timeout: 15_000,
      });
    } catch (error) {
      launchErrors.push(
        `system browser ${executablePath}: ${error.message.split("\n")[0]}`,
      );
    }
  } else {
    launchErrors.push("system browser: not found");
  }

  throw new Error(
    [
      "Cannot launch a browser for prototype verification.",
      "The script will not run npx playwright install.",
      "Install browsers through the repository-approved workflow if screenshots must be refreshed:",
      "corepack pnpm --filter @superteam/web run test:browser:install",
      "Launch attempts:",
      ...launchErrors.map((entry) => `- ${entry}`),
    ].join("\n"),
  );
}

function inspectLayout() {
  // 原型是概念稿，允许横向滚动的区域比产品页更宽；只拦页面级溢出与无障碍硬伤。
  const allowedOverflowSelector =
    ".table-wrap, .nav-section, .topology, .chart, .scroll-x, [data-allow-overflow], .canvas, #canvas, .graph, .flow-canvas, .react-flow, svg#edgeSvg";
  const textTargetSelector =
    "button, .btn, .tab, .badge, .panel-title, .page-title, .metric-value";

  const describe = (element) => {
    const id = element.id ? `#${element.id}` : "";
    const className =
      typeof element.className === "string" && element.className.trim()
        ? `.${element.className.trim().replace(/\s+/g, ".")}`
        : "";
    const text = element.textContent?.trim().replace(/\s+/g, " ").slice(0, 44);
    return `${element.tagName.toLowerCase()}${id}${className}${text ? ` "${text}"` : ""}`;
  };

  const isVisible = (element) => {
    const style = window.getComputedStyle(element);
    const rect = element.getBoundingClientRect();
    return (
      style.display !== "none" &&
      style.visibility !== "hidden" &&
      rect.width > 0 &&
      rect.height > 0
    );
  };

  const pageOverflowX = Math.max(
    0,
    Math.ceil(
      Math.max(
        document.documentElement.scrollWidth,
        document.body?.scrollWidth ?? 0,
      ) - window.innerWidth,
    ),
  );

  const offscreen = Array.from(document.querySelectorAll("body *"))
    .filter((element) => {
      if (!isVisible(element) || element.closest(allowedOverflowSelector)) {
        return false;
      }
      const rect = element.getBoundingClientRect();
      return rect.left < -2 || rect.right > window.innerWidth + 2;
    })
    .slice(0, 8)
    .map(describe);

  const textOverflow = Array.from(document.querySelectorAll(textTargetSelector))
    .filter((element) => {
      if (!isVisible(element) || element.closest(allowedOverflowSelector)) {
        return false;
      }
      return element.scrollWidth > element.clientWidth + 2;
    })
    .slice(0, 8)
    .map(describe);

  const unlabeledIconButtons = Array.from(document.querySelectorAll("button"))
    .filter(
      (button) =>
        isVisible(button) &&
        !button.textContent.trim() &&
        !button.getAttribute("aria-label") &&
        !button.getAttribute("title"),
    )
    .map(describe);

  return {
    pageOverflowX,
    offscreen,
    textOverflow,
    unlabeledIconButtons,
    documentWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  };
}

async function verifyPage(browser, baseUrl, pageName, viewport, strictLayout) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  const consoleMessages = [];
  const pageErrors = [];

  page.on("console", (message) => {
    if (["error", "warning"].includes(message.type())) {
      consoleMessages.push(`${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));

  const url = `${baseUrl}/${pageName}`;
  const response = await page.goto(url, {
    waitUntil: "networkidle",
    timeout: 20_000,
  });
  if (!response || !response.ok()) {
    throw new Error(
      `${pageName} ${viewport.name}: HTTP ${response?.status() ?? "no response"}`,
    );
  }

  await page.waitForTimeout(250);
  const metrics = await page.evaluate(inspectLayout);

  const safeName = pageName.replaceAll("/", "__").replace(/\.html$/, "");
  const screenshotPath = path.join(
    screenshotDir,
    `${safeName}-${viewport.name}-${viewport.width}x${viewport.height}.png`,
  );
  await page.screenshot({ path: screenshotPath, fullPage: true });
  await context.close();

  const failures = [];
  if (pageErrors.length > 0) {
    failures.push(`page errors: ${pageErrors.join(" | ")}`);
  }
  // console：只把 error 当硬失败；warning 记入 metrics 但不失败（CDN/弃用警告常见于概念稿）。
  const consoleErrors = consoleMessages.filter((m) => m.startsWith("error:"));
  if (consoleErrors.length > 0) {
    failures.push(`console errors: ${consoleErrors.join(" | ")}`);
  }
  if (strictLayout) {
    if (metrics.pageOverflowX > 1) {
      failures.push(`page overflow-x ${metrics.pageOverflowX}px`);
    }
    if (metrics.offscreen.length > 0) {
      failures.push(`offscreen elements: ${metrics.offscreen.join(" | ")}`);
    }
    if (metrics.textOverflow.length > 0) {
      failures.push(`text overflow: ${metrics.textOverflow.join(" | ")}`);
    }
    if (metrics.unlabeledIconButtons.length > 0) {
      failures.push(
        `unlabeled icon buttons: ${metrics.unlabeledIconButtons.join(" | ")}`,
      );
    }
  }

  return {
    pageName,
    viewport: viewport.name,
    strictLayout,
    screenshot: relativeToRepo(screenshotPath),
    metrics,
    consoleWarnings: consoleMessages.filter((m) => m.startsWith("warning:")),
    failures,
  };
}

async function main() {
  await fs.mkdir(screenshotDir, { recursive: true });
  await discoverStrictMarkerDirs();
  const pages = await discoverPrototypePages();
  if (pages.length === 0) {
    throw new Error(
      `No prototype HTML files found under ${relativeToRepo(prototypeRoot)}.`,
    );
  }

  const { chromium } = loadPlaywright();
  if (!chromium) {
    throw new Error("Resolved Playwright package does not expose chromium.");
  }

  const server = createStaticServer(pages[0]);
  let browser;
  const results = [];

  try {
    const port = await listen(server);
    const baseUrl = `http://127.0.0.1:${port}`;
    browser = await launchChromium(chromium);

    for (const pageName of pages) {
      const strictLayout = isStrictLayoutPage(pageName);
      for (const viewport of viewports) {
        const result = await verifyPage(
          browser,
          baseUrl,
          pageName,
          viewport,
          strictLayout,
        );
        results.push(result);
        const status = result.failures.length === 0 ? "ok" : "FAIL";
        console.log(
          [
            `[prototype] ${status} ${pageName} ${viewport.name}`,
            strictLayout ? "strict" : "smoke",
            `overflowX=${result.metrics.pageOverflowX}`,
            `screenshot=${result.screenshot}`,
          ].join(" "),
        );
      }
    }
  } finally {
    if (browser) {
      await browser.close();
    }
    await close(server);
  }

  const failures = results.flatMap((result) =>
    result.failures.map(
      (failure) => `${result.pageName} ${result.viewport}: ${failure}`,
    ),
  );

  if (failures.length > 0) {
    console.error("\nPrototype verification failed:");
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    console.error(
      `\nTip: layout strictness only applies to ${[...STRICT_LAYOUT_DIRS].join(", ")} (or dirs with .prototype-strict-gate). Pass --strict-all to enforce layout on every page.`,
    );
    process.exitCode = 1;
    return;
  }

  const strictCount = results.filter((r) => r.strictLayout).length;
  console.log(
    `\nPrototype verification passed: ${results.length} page/viewport checks across ${pages.length} prototypes (${strictCount} strict-layout checks).`,
  );
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
