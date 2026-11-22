import { defineConfig } from '@playwright/test';

// Unit specs (*.unit.spec.ts) are pure Node. Browser specs start their own
// agentlog server in a fixture, so there is no webServer here.
export default defineConfig({
  testDir: './tests',
  workers: 1,
  timeout: 30_000,
  reporter: [['list']],
  use: { baseURL: 'http://127.0.0.1:7799', viewport: { width: 1440, height: 900 } },
});
