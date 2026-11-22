import { test as base, expect, type Page } from '@playwright/test';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { cpSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const bin = path.join(root, 'bin', 'agentlog');

export interface Server { url: string; dir: string; proc: ChildProcess; stop(): Promise<void>; }

/** Builds the demo repo in `dir` (scripts/make-demo-repo.sh). */
export function makeDemoRepo(dir: string) {
  execFileSync(path.join(root, 'scripts', 'make-demo-repo.sh'), [dir], { stdio: 'pipe' });
}

/** Starts `agentlog ui` on a fixed port and waits until /api/repo answers. */
export async function startServer(dir: string, port: number): Promise<Server> {
  const proc = spawn(bin, ['ui', dir, '--no-open', '--port', String(port)], { cwd: root, stdio: 'pipe' });
  let log = '';
  proc.stdout?.on('data', (d) => (log += d));
  proc.stderr?.on('data', (d) => (log += d));
  const url = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 15_000;
  for (;;) {
    if (proc.exitCode !== null) throw new Error(`agentlog ui exited early (is port ${port} busy?):\n${log}`);
    try {
      if ((await fetch(`${url}/api/repo`)).ok) break;
    } catch { /* not up yet */ }
    if (Date.now() > deadline) { proc.kill(); throw new Error(`agentlog ui did not start:\n${log}`); }
    await new Promise((r) => setTimeout(r, 100));
  }
  const stop = () => new Promise<void>((resolve) => {
    if (proc.exitCode !== null) return resolve();
    proc.once('exit', () => resolve());
    proc.kill('SIGTERM');
    setTimeout(() => proc.kill('SIGKILL'), 3000).unref();
  });
  return { url, dir, proc, stop };
}

/** Commits a new file in the demo repo, as an agent would. */
export function commitFile(dir: string, file: string, content: string, subject: string) {
  const git = (...args: string[]) => execFileSync('git', ['-C', dir, '-c', 'user.name=e2e', '-c', 'user.email=e2e@localhost',
    '-c', 'commit.gpgsign=false', ...args], { stdio: 'pipe' });
  execFileSync('sh', ['-c', `printf '%s\\n' "$1" > "$2"`, 'sh', content, path.join(dir, file)]);
  git('add', file);
  git('commit', '-q', '-m', subject, '-m', 'Agent: Claude');
}

export const DEMO_PORT = 7799;

/**
 * `template`: the demo repo, built once per worker (building it is slow, copying is not).
 * `demo`: a private copy plus a running server for each test, so tests may mutate it.
 * `page`: fails the test on any console error or uncaught page exception. It depends on `demo`
 *   so the server outlives the page, and it leaves the app before checking so a poll that races
 *   the teardown cannot produce a spurious error.
 */
export const test = base.extend<{ demo: Server }, { template: string }>({
  template: [async ({}, use, workerInfo) => {
    const dir = path.join('/tmp', `agentlog-e2e-template-${workerInfo.workerIndex}`);
    makeDemoRepo(dir);
    await use(dir);
    rmSync(dir, { recursive: true, force: true });
  }, { scope: 'worker' }],

  demo: async ({ template }, use, testInfo) => {
    const dir = path.join('/tmp', `agentlog-e2e-${testInfo.workerIndex}`);
    rmSync(dir, { recursive: true, force: true });
    cpSync(template, dir, { recursive: true });
    const server = await startServer(dir, DEMO_PORT);
    await use(server);
    await server.stop();
    rmSync(dir, { recursive: true, force: true });
  },

  page: async ({ page, demo }, use) => {
    void demo;
    const problems: string[] = [];
    page.on('console', (m) => { if (m.type() === 'error') problems.push(`console: ${m.text()}`); });
    page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`));
    await use(page);
    await page.goto('about:blank'); // stop polling before anything is torn down
    expect(problems, 'browser console errors').toEqual([]);
  },
});

export { expect };
export type { Page };

/** Opens the UI and waits for the timeline and the first detail to render. */
export async function open(page: Page, url: string) {
  await page.goto(url);
  await expect(page.locator('.item.sel')).toBeVisible();
  await expect(page.locator('#detail h1')).toBeVisible();
}
