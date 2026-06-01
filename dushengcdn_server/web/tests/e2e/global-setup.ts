import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { request } from 'node:http';

const DEFAULT_E2E_PORT = '3100';
const STARTUP_TIMEOUT_MS = 120_000;

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function isLocalhost(url: URL) {
  return (
    url.hostname === '127.0.0.1' ||
    url.hostname === 'localhost' ||
    url.hostname === '::1'
  );
}

function probe(url: URL) {
  return new Promise<boolean>((resolve) => {
    const req = request(
      url,
      {
        method: 'GET',
        timeout: 2_000,
      },
      (res) => {
        res.resume();
        res.on('end', () => resolve((res.statusCode ?? 0) < 500));
      },
    );

    req.on('timeout', () => {
      req.destroy();
      resolve(false);
    });
    req.on('error', () => resolve(false));
    req.end();
  });
}

async function waitForServer(url: URL, child?: ChildProcessWithoutNullStreams) {
  const deadline = Date.now() + STARTUP_TIMEOUT_MS;

  while (Date.now() < deadline) {
    if (await probe(url)) {
      return;
    }
    if (child && child.exitCode !== null) {
      throw new Error(
        `E2E dev server exited before becoming ready, exit code ${child.exitCode}.`,
      );
    }
    await sleep(500);
  }

  throw new Error(`Timed out waiting for E2E dev server at ${url.href}.`);
}

function waitForExit(child: ChildProcessWithoutNullStreams, timeoutMs: number) {
  return new Promise<boolean>((resolve) => {
    const timer = setTimeout(() => resolve(false), timeoutMs);
    child.once('exit', () => {
      clearTimeout(timer);
      resolve(true);
    });
  });
}

export default async function globalSetup() {
  const baseURL = new URL(
    process.env.PLAYWRIGHT_TEST_BASE_URL ??
      `http://127.0.0.1:${process.env.E2E_WEB_PORT ?? DEFAULT_E2E_PORT}`,
  );
  const loginURL = new URL('/login', baseURL);

  if (await probe(loginURL)) {
    return;
  }

  if (!isLocalhost(baseURL)) {
    throw new Error(
      `E2E baseURL ${baseURL.href} is not reachable and cannot be started locally.`,
    );
  }

  const child = spawn(process.execPath, ['scripts/dev-server.mjs'], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      HOSTNAME: baseURL.hostname,
      PORT: baseURL.port || DEFAULT_E2E_PORT,
    },
    stdio: 'pipe',
  });

  child.stdout.on('data', (chunk) => process.stdout.write(chunk));
  child.stderr.on('data', (chunk) => process.stderr.write(chunk));

  await waitForServer(loginURL, child);

  return async () => {
    if (child.exitCode !== null) {
      return;
    }

    child.kill();
    const exited = await waitForExit(child, 5_000);
    if (!exited) {
      child.kill('SIGKILL');
      await waitForExit(child, 2_000);
    }
  };
}
