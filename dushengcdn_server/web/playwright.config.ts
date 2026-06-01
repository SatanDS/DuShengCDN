import { defineConfig, devices } from '@playwright/test';

const browserChannel =
  process.platform === 'win32' ? { channel: 'chrome' as const } : {};
const e2ePort = process.env.E2E_WEB_PORT ?? '3100';
const e2eBaseURL =
  process.env.PLAYWRIGHT_TEST_BASE_URL ?? `http://127.0.0.1:${e2ePort}`;

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  globalSetup: './tests/e2e/global-setup.ts',
  use: {
    baseURL: e2eBaseURL,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], ...browserChannel },
    },
  ],
});
