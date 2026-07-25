/// <reference types="vitest/config" />
import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { playwright } from '@vitest/browser-playwright'

const chromiumExecutablePath = process.env.VITEST_CHROMIUM_EXECUTABLE_PATH?.trim()

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
    fs: {
      allow: [path.resolve(__dirname, '../..')],
    },
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
