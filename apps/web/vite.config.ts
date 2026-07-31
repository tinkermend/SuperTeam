/// <reference types="vitest/config" />
import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { playwright } from '@vitest/browser-playwright'

const chromiumExecutablePath = process.env.VITEST_CHROMIUM_EXECUTABLE_PATH?.trim()

/**
 * 本地开发 Web 端口的唯一事实源。改这里必须同步：
 * - `apps/control-plane/internal/api/middleware/cors.go` 的默认允许来源
 * - `apps/control-plane/internal/app/app.go` 的 feishuWebOrigin 默认值
 * - `apps/feishu-connector/main.go` 的 CONTROL_PLANE_WEB_ORIGIN 默认值
 * - `scripts/dev-services.sh` 的 WEB_WAIT_URL 默认值
 *
 * strictPort 是必须的：端口被占时 vite 默认会静默换一个，而 CORS 白名单与
 * dev-services 探活都钉死在这个端口上，一漂就变成「页面能开、接口全 CORS
 * 失败」的难查故障。2026-07-31 从 3000 迁到 3100，3000 让给其他进程。
 */
const DEV_SERVER_PORT = 3100

// vitest 浏览器模式复用下面的 server 配置；测试时不钉端口，否则本地开着
// dev server 就跑不了测试。
const isTest = process.env.VITEST === 'true' || process.env.NODE_ENV === 'test'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      // 2026-07-25 §5.4: shared HumanTask kind → 中文 fixture (contracts is source of truth)
      '@human-task-kind-labels': path.resolve(
        __dirname,
        '../../contracts/control-plane/human-task-kind-labels.json',
      ),
    },
  },
  build: {
    rollupOptions: {
      output: {
        // 把长期稳定的框架 vendor 从入口切出去（P1-D Step 3）：它们几乎不随业务改动，
        // 独立成块后业务代码更新不会使其缓存失效，回访者命中缓存。重依赖（xyflow/dicebear）
        // 已由 Step 1/2 移出入口，这里只处理剩下的框架大头（React/Router/Query/Radix）。
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return undefined
          }
          if (/[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/.test(id)) {
            return 'vendor-react'
          }
          if (id.includes('@tanstack')) {
            return 'vendor-tanstack'
          }
          if (id.includes('@radix-ui')) {
            return 'vendor-radix'
          }
          return undefined
        },
      },
    },
  },
  server: {
    host: '127.0.0.1',
    // 本地开发端口的唯一事实源（见文件顶部 DEV_SERVER_PORT 注释）。
    // 只在非 test 模式钉端口：vitest 浏览器模式会复用这段 server 配置，
    // 钉死+strictPort 会让「开着 dev server 时跑测试」直接起不来。
    ...(isTest ? {} : { port: DEV_SERVER_PORT, strictPort: true }),
    fs: {
      allow: [path.resolve(__dirname, '../..')],
    },
  },
  preview: {
    host: '127.0.0.1',
    port: DEV_SERVER_PORT,
    strictPort: true,
  },
  optimizeDeps: {
    include: ['@tanstack/react-query', '@radix-ui/react-alert-dialog', '@radix-ui/react-select', '@radix-ui/react-tabs', '@monaco-editor/react'],
  },
  test: {
    silent: 'passed-only',
    unstubEnvs: true,
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(
        chromiumExecutablePath
          ? {
              launchOptions: {
                executablePath: chromiumExecutablePath,
              },
            }
          : undefined,
      ),
      instances: [{ browser: 'chromium' }],
    },
    coverage: {
      // include: ['src/**/*.{js,jsx,ts,tsx}'], // Uncomment to expand the report to all src/**/* so untested modules appear as 0% coverage.
      exclude: [
        'src/components/ui/**',
        'src/assets/**',
        'src/tanstack-table.d.ts',
        'src/routeTree.gen.ts',
        'src/test-utils/**',
        'src/routes/**',
      ],
    },
  },
})
