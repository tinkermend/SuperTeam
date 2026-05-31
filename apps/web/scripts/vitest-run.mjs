import { spawnSync } from "node:child_process";

const vitestArgs = process.argv.slice(2).filter((arg) => arg !== "--");

const result = spawnSync("vitest", ["run", "--browser.headless", ...vitestArgs], {
  stdio: "inherit",
  shell: process.platform === "win32",
});

process.exit(result.status ?? 1);
