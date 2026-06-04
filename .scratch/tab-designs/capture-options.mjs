import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const require = createRequire(resolve(__dirname, '../../apps/web/package.json'))
const { chromium } = require('playwright')

const htmlPath = resolve(__dirname, 'superteam-tab-options.html')
const browser = await chromium.launch()
const page = await browser.newPage({
  deviceScaleFactor: 2,
  viewport: { width: 1800, height: 520 },
})

await page.goto(pathToFileURL(htmlPath).href)
for (const [selector, fileName] of [
  ['#option-a', 'superteam-tab-style-a.png'],
  ['#option-b', 'superteam-tab-style-b.png'],
  ['#option-c', 'superteam-tab-style-a2.png'],
]) {
  const target = page.locator(selector)
  await target.screenshot({
    path: resolve(__dirname, fileName),
  })
}

await browser.close()
