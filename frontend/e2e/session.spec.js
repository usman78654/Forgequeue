import {expect, test} from '@playwright/test';

const session = {
  principal: {id: 'principal-1', name: 'Test Operator', role: 'operator'},
  csrf_token: 'csrf-token',
  expires_at: '2030-01-01T00:00:00Z',
};

test.beforeEach(async ({page}) => {
  await page.route('**/v1/**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({items: []}),
    }),
  );
  await page.route('**/v1/session', async (route) => {
    const method = route.request().method();
    if (method === 'GET') {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({detail: 'no session'}),
      });
      return;
    }
    if (method === 'POST') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(session),
      });
      return;
    }
    await route.fulfill({status: 204});
  });
});

test('exchanges a key and exposes authenticated navigation', async ({page}) => {
  await page.goto('./');
  await expect(page.getByRole('heading', {name: 'Connect to ForgeQueue'})).toBeVisible();
  await page.getByLabel('API key').fill('fq_browser_test');
  const exchangeRequest = page.waitForRequest(
    (request) => request.url().endsWith('/v1/session') && request.method() === 'POST',
  );
  await page.getByRole('button', {name: 'Connect'}).click();
  await expect(page.getByText('Connected as Test Operator.', {exact: true})).toBeVisible();
  expect((await exchangeRequest).headers().authorization).toBe('Bearer fq_browser_test');
  await expect(page.getByRole('navigation', {name: 'Primary navigation'})).toBeVisible();
  await expect(page.getByRole('heading', {name: 'Jobs'})).toBeVisible();
});
