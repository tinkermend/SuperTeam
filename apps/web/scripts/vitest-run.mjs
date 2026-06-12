import { spawnSync } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import os from "node:os";
import path from "node:path";

const vitestArgs = process.argv.slice(2).filter((arg) => arg !== "--");

function playwrightCacheDir() {
  const configured = process.env.PLAYWRIGHT_BROWSERS_PATH?.trim();
  if (configured && configured !== "0") return configured;
  if (process.platform === "darwin") return path.join(os.homedir(), "Library", "Caches", "ms-playwright");
  if (process.platform === "win32") {
    const localAppData = process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local");
    return path.join(localAppData, "ms-playwright");
  }
  return path.join(os.homedir(), ".cache", "ms-playwright");
}

function headlessShellExecutableSegments() {
  if (process.platform === "darwin") {
    return [process.arch === "arm64" ? "chrome-headless-shell-mac-arm64" : "chrome-headless-shell-mac-x64", "chrome-headless-shell"];
  }
  if (process.platform === "win32") return ["chrome-headless-shell-win64", "chrome-headless-shell.exe"];
  return ["chrome-headless-shell-linux64", "chrome-headless-shell"];
}

function availableHeadlessShellPath(cacheDir) {
  if (!existsSync(cacheDir)) return null;
  const executableSegments = headlessShellExecutableSegments();
  return readdirSync(cacheDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && entry.name.startsWith("chromium_headless_shell-"))
    .map((entry) => {
      const revision = Number(entry.name.replace("chromium_headless_shell-", ""));
      const executablePath = path.join(cacheDir, entry.name, ...executableSegments);
      return { executablePath, revision: Number.isFinite(revision) ? revision : 0 };
    })
    .filter((candidate) => existsSync(candidate.executablePath))
    .sort((left, right) => right.revision - left.revision)[0]?.executablePath ?? null;
}

function configureChromiumExecutableFallback() {
  if (process.env.VITEST_CHROMIUM_EXECUTABLE_PATH?.trim()) return;

  const cacheDir = playwrightCacheDir();
  const fallback = availableHeadlessShellPath(cacheDir);
  if (!fallback) return;

  process.env.VITEST_CHROMIUM_EXECUTABLE_PATH = fallback;
  console.warn(`[vitest-run] using Playwright headless shell: ${fallback}`);
}

configureChromiumExecutableFallback();

const result = spawnSync("vitest", ["run", "--browser.headless", ...vitestArgs], {
  stdio: "inherit",
  shell: process.platform === "win32",
});

process.exit(result.status ?? 1);
