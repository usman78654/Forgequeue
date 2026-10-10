import {expect, test} from '@playwright/test';

const session = {
  principal: {id: 'principal-1', name: 'Test Operator', role: 'operator'},
  csrf_token: 'csrf-token',
  expires_at: '2030-01-01T00:00:00Z',
};

const queuedJob = {
  id: 'job-123456789',
  type: 'echo',
  queue: 'jobs.default',
  status: 'queued',
  attempt: 0,
  max_attempts: 3,
  created_at: '2030-01-01T00:00:00Z',
};

async function json(route, body, status = 200) {
  await route.fulfill({status, contentType: 'application/json', body: JSON.stringify(body)});
}

test.beforeEach(async ({page}) => {
  await page.route('**/v1/**', (route) => json(route, {items: []}));
  await page.route('**/v1/session', async (route) => {
    if (route.request().method() === 'GET') return json(route, {detail: 'no session'}, 401);
    if (route.request().method() === 'POST') return json(route, session, 201);
    await route.fulfill({status: 204});
  });
});

async function connect(page, expectedName = 'Test Operator') {
  await page.goto('./');
  await page.getByLabel('API key').fill('fq_browser_test');
  await page.getByRole('button', {name: 'Connect'}).click();
  await expect(page.getByText(`Connected as ${expectedName}.`, {exact: true})).toBeVisible();
}

test('submits and renders a new job', async ({page}) => {
  let submitted = false;
  await page.route('**/v1/jobs**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'POST' && url.pathname === '/v1/jobs') {
      submitted = true;
      return json(route, queuedJob, 201);
    }
    if (request.method() === 'GET' && url.pathname === '/v1/jobs') {
      return json(route, {items: submitted ? [queuedJob] : []});
    }
    return json(route, {items: []});
  });
  await connect(page);

  await page.getByRole('button', {name: 'Submit job'}).click();
  const requestPromise = page.waitForRequest(
    (request) => request.method() === 'POST' && new URL(request.url()).pathname === '/v1/jobs',
  );
  await page.getByRole('button', {name: 'Submit', exact: true}).click();
  const request = await requestPromise;
  expect(request.postDataJSON()).toMatchObject({type: 'echo', retry_policy: 'exponential'});
  await expect(page.getByText('Job submitted.', {exact: true})).toBeVisible();
  await expect(page.getByRole('button', {name: 'job-123'})).toBeVisible();
});

test('inspects and cancels a queued job', async ({page}) => {
  await page.route('**/v1/jobs**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path === '/v1/jobs') return json(route, {items: [queuedJob]});
    if (request.method() === 'GET' && path.endsWith('/events')) return json(route, {items: []});
    if (request.method() === 'GET') return json(route, queuedJob);
    await route.fulfill({status: 204});
  });
  await connect(page);

  await page.getByRole('button', {name: 'job-123'}).click();
  await expect(page.getByRole('dialog', {name: 'Job job-123'})).toBeVisible();
  await page.getByRole('button', {name: 'Close', exact: true}).click();
  const cancellation = page.waitForRequest((request) =>
    new URL(request.url()).pathname.endsWith('/cancel'),
  );
  const cancelButton = page.getByRole('button', {name: 'Cancel'});
  await cancelButton.scrollIntoViewIfNeeded();
  await cancelButton.click();
  await expect(page.getByRole('dialog', {name: 'Cancel job?'})).toBeVisible();
  await page.getByRole('button', {name: 'Confirm cancellation'}).click();
  expect((await cancellation).method()).toBe('POST');
  await expect(page.getByText('Job cancellation requested.', {exact: true})).toBeVisible();
});

test('retries a dead-lettered job', async ({page}) => {
  const deadJob = {...queuedJob, id: 'dead-123456789', status: 'dead', attempt: 3};
  await page.route('**/v1/jobs**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path === '/v1/jobs') return json(route, {items: [deadJob]});
    await route.fulfill({status: 204});
  });
  await connect(page);

  const retry = page.waitForRequest((request) =>
    new URL(request.url()).pathname.endsWith('/requeue'),
  );
  const retryButton = page.getByRole('button', {name: 'Retry'});
  await retryButton.scrollIntoViewIfNeeded();
  await retryButton.click();
  await expect(page.getByRole('dialog', {name: 'Retry dead job?'})).toBeVisible();
  await page.getByRole('button', {name: 'Confirm retry'}).click();
  expect((await retry).method()).toBe('POST');
  await expect(page.getByText('Job requeued.', {exact: true})).toBeVisible();
});

test('bulk requeues selected dead-lettered jobs after confirmation', async ({page}) => {
  const deadJobs = ['dead-11111111', 'dead-22222222'].map((id) => ({
    ...queuedJob,
    id,
    status: 'dead',
    attempt: 3,
  }));
  await page.route('**/v1/jobs**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path === '/v1/jobs') return json(route, {items: deadJobs});
    await route.fulfill({status: 204});
  });
  await connect(page);

  await page.getByRole('checkbox', {name: 'Select job dead-111'}).check();
  await page.getByRole('checkbox', {name: 'Select job dead-222'}).check();
  await page.getByRole('button', {name: 'Retry selected (2)'}).click();
  await expect(page.getByRole('dialog', {name: 'Retry selected dead jobs?'})).toBeVisible();
  const requests = Promise.all([
    page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith('dead-11111111/requeue'),
    ),
    page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith('dead-22222222/requeue'),
    ),
  ]);
  await page.getByRole('button', {name: 'Confirm retry of 2'}).click();
  expect((await requests).every((request) => request.method() === 'POST')).toBe(true);
});

test('pauses a queue only after explicit confirmation', async ({page}) => {
  await page.route('**/v1/queues**', async (route) => {
    if (route.request().method() === 'GET') {
      return json(route, {
        items: [{queue: 'jobs.default', paused: false, max_concurrency: 4, running: 2}],
      });
    }
    await route.fulfill({status: 204});
  });
  await connect(page);
  await page.getByRole('button', {name: 'Queues'}).click();

  await page.getByRole('button', {name: 'Pause'}).click();
  await expect(page.getByRole('dialog', {name: 'Pause queue?'})).toBeVisible();
  const update = page.waitForRequest(
    (request) =>
      request.method() === 'PUT' && new URL(request.url()).pathname === '/v1/queues/jobs.default',
  );
  await page.getByRole('button', {name: 'Confirm change'}).click();
  expect((await update).postDataJSON()).toEqual({paused: true, max_concurrency: 4});
});

test('shows key administration only to admins and confirms revocation', async ({page}) => {
  const adminSession = {
    ...session,
    principal: {id: 'admin-1', name: 'Test Admin', role: 'admin'},
  };
  await page.route('**/v1/session', async (route) => {
    if (route.request().method() === 'GET') return json(route, {detail: 'no session'}, 401);
    if (route.request().method() === 'POST') return json(route, adminSession, 201);
    await route.fulfill({status: 204});
  });
  await page.route('**/v1/admin/keys**', async (route) => {
    if (route.request().method() === 'GET') {
      return json(route, {
        items: [
          {
            key: {
              id: 'key-1',
              name: 'worker-key',
              prefix: 'fq_1234567',
              expires_at: null,
              last_used_at: null,
              revoked_at: null,
              created_at: '2030-01-01T00:00:00Z',
            },
            principal: adminSession.principal,
          },
        ],
      });
    }
    await route.fulfill({status: 204});
  });
  await connect(page, 'Test Admin');
  await page.getByRole('button', {name: 'Administration'}).click();

  await page.getByRole('button', {name: 'Revoke'}).click();
  await expect(page.getByRole('dialog', {name: 'Revoke API key?'})).toBeVisible();
  const revocation = page.waitForRequest(
    (request) =>
      request.method() === 'DELETE' && new URL(request.url()).pathname === '/v1/admin/keys/key-1',
  );
  await page.getByRole('button', {name: 'Confirm revocation'}).click();
  expect((await revocation).method()).toBe('DELETE');
});

test('creates a workflow from the DAG builder', async ({page}) => {
  await page.route('**/v1/workflows', async (route) => {
    if (route.request().method() === 'POST') {
      return json(
        route,
        {id: 'workflow-123456', name: 'visual-pipeline', status: 'pending', steps: []},
        201,
      );
    }
    return json(route, {items: []});
  });
  await connect(page);
  await page.getByRole('button', {name: 'DAG Builder'}).click();
  await page.getByLabel('Step key').fill('prepare');
  await page.getByRole('button', {name: 'Add step'}).click();

  const creation = page.waitForRequest(
    (request) => request.method() === 'POST' && new URL(request.url()).pathname === '/v1/workflows',
  );
  await page.getByRole('button', {name: 'Run workflow'}).click();
  const body = (await creation).postDataJSON();
  expect(body.steps).toHaveLength(1);
  expect(body.steps[0]).toMatchObject({key: 'prepare', type: 'echo'});
  await expect(page.getByText('Workflow workflow started.', {exact: true})).toBeVisible();
});

test('creates a recurring schedule', async ({page}) => {
  await page.route('**/v1/schedules', async (route) => {
    if (route.request().method() === 'POST') {
      return json(route, {id: 'schedule-1'}, 201);
    }
    return json(route, []);
  });
  await connect(page);
  await page.getByRole('button', {name: 'Schedules'}).click();

  await page.getByLabel('Name').fill('nightly-maintenance');
  await page.getByLabel('Interval (seconds)').fill('86400');
  const creation = page.waitForRequest(
    (request) => request.method() === 'POST' && new URL(request.url()).pathname === '/v1/schedules',
  );
  await page.getByRole('button', {name: 'Create schedule'}).click();
  expect((await creation).postDataJSON()).toMatchObject({
    name: 'nightly-maintenance',
    interval_seconds: 86400,
  });
  await expect(page.getByText('Schedule created.', {exact: true})).toBeVisible();
});

test('shows authentication failures without losing responsive navigation', async ({page}) => {
  await page.route('**/v1/session', async (route) => {
    if (route.request().method() === 'POST') return json(route, {detail: 'API key rejected'}, 401);
    return json(route, {detail: 'no session'}, 401);
  });
  await page.goto('./');
  await page.getByLabel('API key').fill('bad-key');
  await page.getByRole('button', {name: 'Connect'}).click();
  await expect(page.getByRole('alert')).toHaveText('API key rejected');
  await expect(page.getByRole('navigation', {name: 'Primary navigation'})).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});
