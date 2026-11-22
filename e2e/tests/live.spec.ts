import { test as base, expect } from '@playwright/test';
import { makeDemoRepo, startServer, type Server } from './fixtures';

const PORT = 7798;
const dir = '/tmp/agentlog-e2e-live';

// Own server on its own port, because this test stops and restarts it.
base('the live dot goes grey after two failed polls and recovers when the server returns', async ({ page }) => {
  makeDemoRepo(dir);
  let server: Server = await startServer(dir, PORT);
  try {
    await page.goto(server.url);
    await expect(page.locator('.item.sel')).toBeVisible();
    const dot = page.locator('#live');
    await expect(dot).not.toHaveClass(/off/);

    await server.stop();
    await expect(dot).toHaveClass(/off/, { timeout: 12_000 });
    await expect(dot).toHaveAttribute('title', 'disconnected');
    // The page keeps what it already shows.
    await expect(page.locator('.item')).toHaveCount(9);

    server = await startServer(dir, PORT);
    await expect(dot).not.toHaveClass(/off/, { timeout: 8000 });
    await expect(dot).toHaveAttribute('title', /live/);
  } finally {
    await server.stop();
  }
});
