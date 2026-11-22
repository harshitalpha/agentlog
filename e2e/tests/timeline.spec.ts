import { writeFileSync } from 'node:fs';
import path from 'node:path';
import { test, expect, open, commitFile, commitAll } from './fixtures';

const SUBJECTS = [
  'Uncommitted changes (2 files)',
  '[auto] uncommitted changes: x.go',
  'Update README wording',
  'Fix empty-id handling in ProductRepository.Get',
  'Add GetProductsByDate query to product DB',
  'Add ProductRepository interface and Postgres impl',
  'Add controller layer for the orders API',
  'Add agentlog commit rules for AI agents',
  'Baseline snapshot before agent work',
];

const subjects = (page: any) => page.locator('.item .subj').allTextContents();
const item = (page: any, text: string) => page.locator('.item', { hasText: text });

test('timeline shows the working tree and every commit, with the right badges', async ({ page, demo }) => {
  await open(page, demo.url);
  expect(await subjects(page)).toEqual(SUBJECTS);
  await expect(page.locator('.item')).toHaveCount(9);

  // The human commit has no badge, the auto commit has "auto", agent commits show the agent.
  await expect(item(page, 'Update README wording').locator('.badge')).toHaveCount(0);
  await expect(item(page, '[auto] uncommitted changes').locator('.badge')).toHaveText('auto');
  await expect(item(page, 'Add GetProductsByDate').locator('.badge')).toHaveText('Claude');
  await expect(page.locator('.item.work .badge')).toHaveText('working tree');

  // Numbering counts from the oldest commit.
  await expect(item(page, 'Baseline snapshot').locator('.num')).toHaveText('#1');
  await expect(item(page, '[auto] uncommitted changes').locator('.num')).toHaveText('#8');
  // Day groups: the working tree sits under "Now".
  await expect(page.locator('.day').first()).toHaveText('Now');
  await expect(page.locator('.day').nth(1)).toHaveText('Today');
});

test('clicking a commit shows its detail and a file card with added lines', async ({ page, demo }) => {
  await open(page, demo.url);
  await item(page, 'Add GetProductsByDate').click();
  await expect(page.locator('#detail h1')).toHaveText('Add GetProductsByDate query to product DB');
  await expect(page.locator('.file')).toHaveCount(1);
  await expect(page.locator('.file .f-path')).toHaveText('internal/repo/product_pg.go');
  await expect(page.locator('.file .f-status')).toHaveText('modified');
  await expect(page.locator('.file tr.add').first()).toBeVisible();
  // Trailer lines are not shown.
  await expect(page.locator('#detail')).not.toContainText('Agent: Claude');
  await expect(page.locator('#detail')).not.toContainText('Co-Authored-By');
  // The selected row is highlighted and the URL points at the commit.
  await expect(item(page, 'Add GetProductsByDate')).toHaveClass(/sel/);
  expect(await page.evaluate(() => location.hash)).toMatch(/^#[0-9a-f]{40}$/);
});

test('layout: the diff number columns are narrow (colgroup)', async ({ page, demo }) => {
  await open(page, demo.url);
  await item(page, 'Add GetProductsByDate').click();
  await expect(page.locator('table.diff')).toBeVisible();
  const widths = await page.$$eval('table.diff tr.add td.n', (cells) => cells.map((c) => c.getBoundingClientRect().width));
  expect(widths.length).toBeGreaterThan(0);
  for (const w of widths) expect(w).toBeLessThanOrEqual(70);
  // The code column gets the rest of the width.
  const code = await page.$eval('table.diff tr.add td.c', (c) => c.getBoundingClientRect().width);
  expect(code).toBeGreaterThan(400);
});

test('story order puts the baseline first and the working tree last, and survives a reload', async ({ page, demo }) => {
  await open(page, demo.url);
  await page.getByRole('button', { name: 'Story order' }).click();
  expect(await subjects(page)).toEqual([...SUBJECTS].reverse());
  await expect(page.locator('#order [aria-pressed="true"]')).toHaveText('Story order');
  await page.reload();
  await expect(page.locator('.item').first()).toBeVisible();
  expect((await subjects(page))[0]).toBe('Baseline snapshot before agent work');
  await page.getByRole('button', { name: 'Newest first' }).click();
  expect((await subjects(page))[0]).toBe('Uncommitted changes (2 files)');
});

test('filter narrows the list, shows an empty state, and clearing restores it', async ({ page, demo }) => {
  await open(page, demo.url);
  await page.locator('#filter').fill('product');
  const narrowed = await subjects(page);
  expect(narrowed.length).toBeGreaterThan(0);
  expect(narrowed.length).toBeLessThan(9);
  for (const s of narrowed) expect(s.toLowerCase()).toContain('product');
  await page.locator('#filter').fill('zzz-no-such-thing');
  await expect(page.locator('#timeline')).toHaveText('No matches.');
  await page.locator('#filter').fill('');
  expect(await subjects(page)).toEqual(SUBJECTS);
});

test('keyboard: j/k move the selection and the URL hash, / focuses the filter, Esc clears it', async ({ page, demo }) => {
  await open(page, demo.url);
  const selected = () => page.locator('.item.sel .subj').textContent();
  expect(await selected()).toBe('Uncommitted changes (2 files)');
  await page.keyboard.press('j');
  await expect(page.locator('.item.sel .subj')).toHaveText('[auto] uncommitted changes: x.go');
  const hash1 = await page.evaluate(() => location.hash);
  await page.keyboard.press('j');
  await expect(page.locator('.item.sel .subj')).toHaveText('Update README wording');
  const hash2 = await page.evaluate(() => location.hash);
  expect(hash2).not.toBe(hash1);
  await page.keyboard.press('k');
  await expect(page.locator('.item.sel .subj')).toHaveText('[auto] uncommitted changes: x.go');
  expect(await page.evaluate(() => location.hash)).toBe(hash1);
  await expect(page.locator('#detail h1')).toHaveText('[auto] uncommitted changes: x.go');

  await page.keyboard.press('/');
  await expect(page.locator('#filter')).toBeFocused();
  await page.keyboard.type('jk'); // typing must not move the selection
  await expect(page.locator('#filter')).toHaveValue('jk');
  await page.keyboard.press('Escape');
  await expect(page.locator('#filter')).toHaveValue('');
  await expect(page.locator('#filter')).not.toBeFocused();
});

test('deep links select a commit (full hash or prefix); unknown hashes fall back to the top', async ({ page, demo }) => {
  await open(page, demo.url);
  await item(page, 'Add controller layer').click();
  const hash = (await page.evaluate(() => location.hash)).slice(1);
  await page.goto('about:blank');
  await open(page, `${demo.url}/#${hash.slice(0, 8)}`);
  await expect(page.locator('#detail h1')).toHaveText('Add controller layer for the orders API');
  await page.goto('about:blank');
  await open(page, `${demo.url}/#deadbeefdead`);
  await expect(page.locator('#detail h1')).toHaveText('Uncommitted changes (2 files)');
});

test('View file opens a numbered modal and Esc closes it', async ({ page, demo }) => {
  await open(page, demo.url);
  await item(page, 'Add GetProductsByDate').click();
  await page.getByRole('button', { name: 'View file' }).click();
  await expect(page.locator('#modal')).toBeVisible();
  await expect(page.locator('#modalTitle')).toContainText('internal/repo/product_pg.go @ ');
  await expect(page.locator('#modalBody tr').first()).toBeVisible();
  await expect(page.locator('#modalBody td.n').first()).toHaveText('1');
  await expect(page.locator('#modalBody')).toContainText('package repo');
  const w = await page.$eval('#modalBody td.n', (c) => c.getBoundingClientRect().width);
  expect(w).toBeLessThanOrEqual(70);
  await page.keyboard.press('Escape');
  await expect(page.locator('#modal')).toBeHidden();

  // The backdrop closes it too.
  await page.getByRole('button', { name: 'View file' }).click();
  await expect(page.locator('#modal')).toBeVisible();
  await page.mouse.click(5, 5);
  await expect(page.locator('#modal')).toBeHidden();
});

test('file cards collapse, chips jump to a file, the hash button copies the full hash', async ({ page, demo, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await open(page, demo.url);
  await item(page, 'Add controller layer').click();
  const card = page.locator('.file').first();
  await expect(card).not.toHaveClass(/closed/);
  await card.locator('.tog').click();
  await expect(card).toHaveClass(/closed/);
  await page.locator('.chip').first().click(); // expands and scrolls to the file
  await expect(card).not.toHaveClass(/closed/);

  const hash = (await page.evaluate(() => location.hash)).slice(1);
  await page.locator('.copy').click();
  await expect(page.locator('.copy')).toHaveText('copied ✓');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(hash);
  await expect(page.locator('.copy')).not.toHaveText('copied ✓', { timeout: 4000 });
});

test('the working-tree item shows uncommitted files, including an untracked one', async ({ page, demo }) => {
  await open(page, demo.url);
  await page.locator('.item.work').click();
  await expect(page.locator('#detail')).toContainText('Not committed yet');
  await expect(page.locator('.file')).toHaveCount(2);
  const statuses = await page.locator('.file .f-status').allTextContents();
  expect(statuses.sort()).toEqual(['added', 'modified']);
});

// Mutates the demo repo, so it runs last in this file.
test('live update: a new commit appears at the top within 5s, without a reload', async ({ page, demo }) => {
  await open(page, demo.url);
  const navigations: string[] = [];
  page.on('framenavigated', (f) => navigations.push(f.url()));
  commitFile(demo.dir, 'live.txt', 'live', 'Add live update marker');
  const top = page.locator('.item:not(.work)').first();
  await expect(top.locator('.subj')).toHaveText('Add live update marker', { timeout: 5000 });
  await expect(top.locator('.num')).toHaveText('#9');
  expect(navigations).toEqual([]);
  await expect(page.locator('.item')).toHaveCount(10);
});

test('live update: the working-tree item follows uncommitted files and disappears once committed', async ({ page, demo }) => {
  await open(page, demo.url);
  const work = page.locator('.item.work .subj');
  await expect(work).toHaveText('Uncommitted changes (2 files)');
  await expect(page.locator('.file')).toHaveCount(2); // the working-tree detail is selected on load

  writeFileSync(path.join(demo.dir, 'scratch.txt'), 'new file\n');
  await expect(work).toHaveText('Uncommitted changes (3 files)', { timeout: 5000 });
  await expect(page.locator('.file')).toHaveCount(3); // the open detail refreshed too

  commitAll(demo.dir, 'Commit the scratch work');
  await expect(page.locator('.item.work')).toHaveCount(0, { timeout: 5000 });
  // The selection moved to the top commit, which is the one we just made.
  await expect(page.locator('.item.sel .subj')).toHaveText('Commit the scratch work');
  await expect(page.locator('#detail h1')).toHaveText('Commit the scratch work');
});
