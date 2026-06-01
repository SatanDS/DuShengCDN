import { expect, test } from '@playwright/test';

test('renders the public login page without a backend session', async ({
  page,
}) => {
  await page.route('**/api/user/self', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({
        success: false,
        message: 'unauthenticated',
      }),
    });
  });
  await page.route('**/api/status', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        message: '',
        data: {
          version: 'e2e',
          start_time: 1_714_032_000,
          email_verification: false,
          github_oauth: false,
          github_client_id: '',
          system_name: 'DuShengCDN',
          home_page_link: '',
          footer_html: '',
          wechat_qrcode: '',
          wechat_login: false,
          server_address: 'http://127.0.0.1:3000',
          turnstile_check: false,
          turnstile_site_key: '',
          register_enabled: false,
          password_register_enabled: false,
          auth_sources: [],
        },
      }),
    });
  });

  await page.goto('/login');

  await expect(page).toHaveTitle(/DuShengCDN/);
  await expect(page.getByText('DuShengCDN').first()).toBeVisible();
  await expect(page.getByRole('heading', { name: '用户登录' })).toBeVisible();
  await expect(page.getByPlaceholder('用户名')).toBeVisible();
  await expect(page.getByPlaceholder('密码')).toBeVisible();
  await expect(page.getByRole('button', { name: '登录' })).toBeVisible();
});
