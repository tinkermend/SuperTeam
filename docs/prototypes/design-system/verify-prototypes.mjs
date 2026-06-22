#!/usr/bin/env node

import { createServer } from "node:http";
import { createRequire } from "node:module";
import { once } from "node:events";
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const prototypeDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(prototypeDir, "../../..");
const screenshotDir = path.join(prototypeDir, "__screenshots__");

const viewports = [
  { name: "desktop", width: 1280, height: 720 },
  { name: "mobile", width: 390, height: 844 },
];

const mimeTypes = new Map([
  [".html", "text/html; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".png", "image/png"],
  [".svg", "image/svg+xml"],
]);

async function discoverPrototypePages() {
  const entries = await fs.readdir(prototypeDir, { withFileTypes: true });
  const pages = entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".html") && !entry.name.startsWith("_"))
    .map((entry) => entry.name)
    .sort();

  if (pages.length === 0) {
    throw new Error(`No prototype HTML files found in ${relativeToRepo(prototypeDir)}.`);
  }

  return pages;
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

      const pathname = requestUrl.pathname === "/" ? `/${defaultPage}` : decodeURIComponent(requestUrl.pathname);
      const filePath = path.resolve(prototypeDir, `.${pathname}`);

      if (!filePath.startsWith(`${prototypeDir}${path.sep}`)) {
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

      response.setHeader("Content-Type", mimeTypes.get(path.extname(filePath)) ?? "application/octet-stream");
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
    launchErrors.push(`bundled Playwright Chromium: ${error.message.split("\n")[0]}`);
  }

  const executablePath = await findSystemChromium();
  if (executablePath) {
    try {
      return await chromium.launch({ executablePath, headless: true, timeout: 15_000 });
    } catch (error) {
      launchErrors.push(`system browser ${executablePath}: ${error.message.split("\n")[0]}`);
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

function relativeToRepo(filePath) {
  return path.relative(repoRoot, filePath);
}

function inspectLayout() {
  const allowedOverflowSelector = ".table-wrap, .nav-section, .topology, .chart";
  const textTargetSelector = "button, .btn, .tab, .badge, .panel-title, .page-title, .metric-value";

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
    return style.display !== "none" && style.visibility !== "hidden" && rect.width > 0 && rect.height > 0;
  };

  const pageOverflowX = Math.max(
    0,
    Math.ceil(Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) - window.innerWidth),
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
    .filter((button) => isVisible(button) && !button.textContent.trim() && !button.getAttribute("aria-label"))
    .map(describe);

  return {
    pageOverflowX,
    offscreen,
    textOverflow,
    unlabeledIconButtons,
    svgIcons: document.querySelectorAll("svg.lucide").length,
    iconPlaceholders: document.querySelectorAll("i[data-lucide]").length,
    documentWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  };
}

async function verifyPage(browser, baseUrl, pageName, viewport) {
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
  const response = await page.goto(url, { waitUntil: "networkidle", timeout: 20_000 });
  if (!response || !response.ok()) {
    throw new Error(`${pageName} ${viewport.name}: HTTP ${response?.status() ?? "no response"}`);
  }

  await page.waitForTimeout(250);
  const metrics = await page.evaluate(inspectLayout);

  const screenshotPath = path.join(
    screenshotDir,
    `${path.basename(pageName, ".html")}-${viewport.name}-${viewport.width}x${viewport.height}.png`,
  );
  await page.screenshot({ path: screenshotPath, fullPage: true });
  await context.close();

  const failures = [];
  if (consoleMessages.length > 0) {
    failures.push(`console messages: ${consoleMessages.join(" | ")}`);
  }
  if (pageErrors.length > 0) {
    failures.push(`page errors: ${pageErrors.join(" | ")}`);
  }
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
    failures.push(`unlabeled icon buttons: ${metrics.unlabeledIconButtons.join(" | ")}`);
  }
  if (metrics.iconPlaceholders > 0 || metrics.svgIcons === 0) {
    failures.push(`icons not fully rendered: ${metrics.svgIcons} svg, ${metrics.iconPlaceholders} placeholders`);
  }

  return {
    pageName,
    viewport: viewport.name,
    screenshot: relativeToRepo(screenshotPath),
    metrics,
    failures,
  };
}

async function main() {
  await fs.mkdir(screenshotDir, { recursive: true });
  const pages = await discoverPrototypePages();

  const { chromium } = loadPlaywright();
  if (!chromium) {
    throw new Error("Resolved Playwright package does not expose chromium.");
  }

  const server = createStaticServer(pages.includes("shell-dashboard.html") ? "shell-dashboard.html" : pages[0]);
  let browser;
  const results = [];

  try {
    const port = await listen(server);
    const baseUrl = `http://127.0.0.1:${port}`;
    browser = await launchChromium(chromium);

    for (const pageName of pages) {
      for (const viewport of viewports) {
        const result = await verifyPage(browser, baseUrl, pageName, viewport);
        results.push(result);
        console.log(
          [
            `[prototype] ${pageName} ${viewport.name}`,
            `overflowX=${result.metrics.pageOverflowX}`,
            `icons=${result.metrics.svgIcons}`,
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
    result.failures.map((failure) => `${result.pageName} ${result.viewport}: ${failure}`),
  );

  if (failures.length > 0) {
    console.error("\nPrototype verification failed:");
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log(`\nPrototype verification passed: ${results.length} page/viewport checks.`);
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
