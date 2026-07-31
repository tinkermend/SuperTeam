import { chromium } from 'playwright';

const cookieValue = process.env.SUPERTEAM_SESSION_TOKEN;
if (!cookieValue) {
  console.error('SUPERTEAM_SESSION_TOKEN 环境变量未设置');
  process.exit(1);
}

const browser = await chromium.launch({
  headless: true,
  executablePath:
    '/Users/tinker/Library/Caches/ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-mac-arm64/chrome-headless-shell',
});
const context = await browser.newContext({ viewport: { width: 1365, height: 1800 } });
await context.addCookies([
  {
    name: 'session_token',
    value: cookieValue,
    domain: '127.0.0.1',
    path: '/',
    httpOnly: true,
    secure: false,
    sameSite: 'Lax',
  },
]);

const page = await context.newPage();
const result = { url: null, selectMode: {}, configureMode: {}, runtimeStep: {} };

await page.goto('http://127.0.0.1:3100/employees/new', { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
result.url = page.url();
const bodyText = (await page.textContent('body')) || '';

// --- Select mode: CreationPathPanel disabled states (Task 2) ---
result.selectMode.blankCustomDisabled = await page
  .getByRole('button', { name: '空白自定义' })
  .first()
  .isDisabled();
result.selectMode.blankCustomBadgeHas暂未开放 = bodyText.includes('暂未开放');
result.selectMode.teamCopyDisabled = await page
  .getByRole('button', { name: '从团队角色复制' })
  .isDisabled();
result.selectMode.historyDisabled = await page
  .getByRole('button', { name: '从历史员工克隆' })
  .isDisabled();
result.selectMode.blankSubButtonDisabled = await page
  .getByRole('button', { name: '选择空白自定义（暂未开放）' })
  .isDisabled();
result.selectMode.templateEntryEnabled = await page
  .getByRole('button', { name: '从专业模板创建' })
  .isEnabled();

// --- Template cards provider-neutral (Task 2) ---
result.selectMode.providerTextInBody =
  bodyText.includes('Provider') || bodyText.includes('推荐 Provider');
result.selectMode.templateCount = await page.locator('button[aria-pressed]').count();

// --- Real-chain reality: did create-options load templates for the default team? ---
result.selectMode.emptyTemplateStateVisible = await page
  .getByText('当前团队治理配置未返回可用专业模板。')
  .isVisible()
  .catch(() => false);
result.selectMode.createOptionsErrorVisible = await page
  .getByText('创建选项加载失败')
  .isVisible()
  .catch(() => false);
result.selectMode.selectedTeamName = (
  await page.locator('text=归属团队').locator('..').locator('span').last().textContent()
).trim();

await page.screenshot({ path: '/tmp/emp-create-select.png', fullPage: true });

// --- If templates loaded, walk through configure + runtime (Task 3/4) ---
const canEnterConfigure =
  !result.selectMode.emptyTemplateStateVisible &&
  !result.selectMode.createOptionsErrorVisible &&
  result.selectMode.templateCount > 0;

if (canEnterConfigure) {
  // Select the first template card.
  await page.locator('button[aria-pressed]').first().click();
  await page.waitForTimeout(300);
  // Enter configure mode.
  await page.getByRole('button', { name: '进入配置', exact: true }).click();
  await page.waitForTimeout(600);

  // Fill required name (other identity fields auto-seed from team/template/avatar).
  await page.getByLabel('名称').fill('smoke 测试员工');
  await page.waitForTimeout(200);

  const configureText = (await page.textContent('body')) || '';
  result.configureMode.summaryVisible = await page.getByText('已选模板').isVisible();
  result.configureMode.changeTemplateVisible = await page
    .getByRole('button', { name: '更换模板' })
    .isVisible();
  result.configureMode.fullTemplateListNotShown = !configureText.includes('推荐起步画像');
  await page.screenshot({ path: '/tmp/emp-create-configure.png', fullPage: true });

  // Step to Runtime (3x 下一步).
  for (let i = 0; i < 3; i++) {
    await page.getByRole('button', { name: '下一步' }).click();
    await page.waitForTimeout(350);
  }
  const runtimeText = (await page.textContent('body')) || '';
  result.runtimeStep.emptyStateVisible = await page
    .getByText('当前团队没有可绑定的 Runtime Provider，请检查 Runtime 在线状态、Provider 健康状态或团队运行策略。')
    .isVisible();
  result.runtimeStep.unavailableSectionVisible = await page
    .getByText('暂不可绑定的 Runtime')
    .isVisible();
  result.runtimeStep.createDisabled = await page
    .getByRole('button', { name: '创建数字员工' })
    .isDisabled();
  result.runtimeStep.radioCount = await page.locator('input[type="radio"]').count();
  result.runtimeStep.disabledReasonVisible = runtimeText.includes('runtime_session_inactive');
  await page.screenshot({ path: '/tmp/emp-create-runtime.png', fullPage: true });
} else {
  result.blockedReason =
    '页面默认选中第一个 active 团队（test-team），其治理未配置，create-options 返回 422，select 步骤无团队切换入口，无法进入 configure / runtime 步骤进行浏览器端到端 smoke。';
}

await browser.close();
console.log(JSON.stringify(result, null, 2));
