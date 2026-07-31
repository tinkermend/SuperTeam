import { chromium } from 'playwright';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    console.log('Navigating to login page...');
    await page.goto('http://127.0.0.1:3100/login', { waitUntil: 'networkidle' });

    console.log('Logging in...');
    await page.locator('input[name="username"]').fill('admin');
    await page.locator('input[name="password"]').fill('admin');
    await page.getByRole('button', { name: '登录' }).click();

    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    console.log('Logged in successfully!');

    console.log('Navigating to projects page...');
    await page.goto('http://127.0.0.1:3100/projects', { waitUntil: 'networkidle' });

    console.log('Clicking create project button...');
    await page.getByRole('button', { name: /新建项目/i }).click();

    console.log('Filling out basics...');
    const projectName = `E2E Test ${Date.now()}`;
    await page.getByLabel(/项目名称/i).fill(projectName);
    await page.getByLabel(/项目目标/i).fill('This is a test project created by playwright');
    
    // Explicitly select the team so React onChange fires
    console.log('Selecting team...');
    const teamSelect = page.locator('select#project-create-team');
    // We select the first available team by its DOM index
    await teamSelect.selectOption({ index: 0 });
    
    await page.getByRole('button', { name: /下一步/i }).click();

    console.log('In Human Roles step...');
    await page.getByRole('button', { name: /下一步/i }).click();

    console.log('In Digital Employees step...');
    await page.getByRole('button', { name: /下一步/i }).click();

    console.log('In Policies step...');
    await page.getByRole('button', { name: /下一步/i }).click();

    console.log('In Review step, submitting...');
    const confirmButton = page.getByRole('button', { name: '确认创建', exact: true });
    
    // Ensure the button is enabled before clicking
    await confirmButton.waitFor({ state: 'visible' });
    const isDisabled = await confirmButton.isDisabled();
    if (isDisabled) {
      throw new Error('Submit button is still disabled! Validation failed.');
    }

    await confirmButton.click();

    console.log('Waiting for creation to finish (modal closes)...');
    await confirmButton.waitFor({ state: 'hidden', timeout: 10000 });
    console.log('✅ Creation finished, modal closed.');

    console.log('Navigating to /task-launches to verify selector...');
    await page.goto('http://127.0.0.1:3100/task-launches', { waitUntil: 'networkidle' });
    
    console.log('Opening project selector...');
    await page.getByRole('combobox').first().click();
    
    console.log(`Looking for project: ${projectName}`);
    const projectOption = page.getByRole('option', { name: projectName, exact: false });
    await projectOption.waitFor({ state: 'visible', timeout: 5000 });
    console.log('✅ Project successfully found in /task-launches selector!');

  } catch (err) {
    console.error('❌ E2E test failed:', err);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
})();
