import type { NextConfig } from "next";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const monorepoRoot = resolve(fileURLToPath(new URL("../..", import.meta.url)));

const nextConfig: NextConfig = {
  turbopack: {
    root: monorepoRoot,
  },
  transpilePackages: ["@superteam/core", "@superteam/views", "@superteam/ui"],
};

export default nextConfig;
