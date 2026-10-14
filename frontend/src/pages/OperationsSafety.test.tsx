import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, it, vi} from 'vitest';
import type {ApiRequest} from '../api-client';
import type {Notify} from '../types';
import {Administration} from './Administration';
import {Jobs} from './Jobs';
import {Queues} from './Queues';
import {Schedules} from './Schedules';
import {Workflows} from './Workflows';

const operator = {id: 'operator-1', name: 'Operator', role: 'operator' as const};
const admin = {id: 'admin-1', name: 'Admin', role: 'admin' as const};
const notify = vi.fn() as Notify;

function apiMock(implementation: ReturnType<typeof vi.fn>): ApiRequest {
  return implementation as unknown as ApiRequest;
}

describe('destructive operation safety', () => {
  it('requires confirmation before pausing a queue', async () => {
    const user = userEvent.setup();
    const request = vi.fn(async (path: string) =>
      path === '/v1/queues'
        ? {items: [{queue: 'jobs.default', paused: false, max_concurrency: 4, running: 2}]}
        : undefined,
    );
    render(<Queues api={apiMock(request)} notify={notify} principal={operator} />);

    await user.click(await screen.findByRole('button', {name: 'Pause'}));
    expect(screen.getByRole('dialog', {name: 'Pause queue?'})).toBeVisible();
    expect(request).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: 'Confirm change'}));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3));
    expect(request.mock.calls[1]).toEqual([
      '/v1/queues/jobs.default',
      {method: 'PUT', body: JSON.stringify({paused: true, max_concurrency: 4})},
    ]);
  });

  it('requires confirmation before revoking an API key', async () => {
    const user = userEvent.setup();
    const record = {
      key: {
        id: 'key-1',
        name: 'worker-key',
        prefix: 'fq_1234567',
        expires_at: null,
        last_used_at: null,
        revoked_at: null,
        created_at: '2030-01-01T00:00:00Z',
      },
      principal: admin,
    };
    const request = vi.fn(async (path: string) =>
      path === '/v1/admin/keys' ? {items: [record]} : {items: []},
    );
    render(<Administration api={apiMock(request)} notify={notify} principal={admin} />);

    await user.click(await screen.findByRole('button', {name: 'Revoke'}));
    expect(screen.getByRole('dialog', {name: 'Revoke API key?'})).toBeVisible();
    expect(request.mock.calls.filter(([path]) => path === '/v1/admin/keys/key-1')).toHaveLength(0);
    await user.click(screen.getByRole('button', {name: 'Confirm revocation'}));
    await vi.waitFor(() =>
      expect(request).toHaveBeenCalledWith('/v1/admin/keys/key-1', {method: 'DELETE'}),
    );
  });

  it('confirms and requeues multiple selected dead jobs', async () => {
    const user = userEvent.setup();
    const jobs = ['dead-11111111', 'dead-22222222'].map((id) => ({
      id,
      type: 'echo',
      queue: 'jobs.default',
      status: 'dead',
      attempt: 3,
      max_attempts: 3,
      created_at: '2030-01-01T00:00:00Z',
    }));
    const request = vi.fn(async (path: string) =>
      path.startsWith('/v1/jobs?') ? {items: jobs} : undefined,
    );
    render(<Jobs api={apiMock(request)} notify={notify} principal={operator} />);

    await user.click(await screen.findByRole('checkbox', {name: 'Select job dead-111'}));
    await user.click(screen.getByRole('checkbox', {name: 'Select job dead-222'}));
    await user.click(screen.getByRole('button', {name: 'Retry selected (2)'}));
    expect(screen.getByRole('dialog', {name: 'Retry selected dead jobs?'})).toBeVisible();
    expect(request).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: 'Confirm retry of 2'}));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(4));
    expect(request).toHaveBeenCalledWith('/v1/jobs/dead-11111111/requeue', {method: 'POST'});
    expect(request).toHaveBeenCalledWith('/v1/jobs/dead-22222222/requeue', {method: 'POST'});
  });

  it('requires confirmation before cancelling a workflow', async () => {
    const user = userEvent.setup();
    const workflow = {
      id: 'workflow-1',
      name: 'release',
      status: 'running',
      steps: [],
    };
    const request = vi.fn(async (path: string) => {
      if (path === '/v1/workflows?limit=50') return {items: [workflow]};
      if (path === '/v1/workflows/workflow-1') return workflow;
      return undefined;
    });
    render(<Workflows api={apiMock(request)} notify={notify} principal={operator} />);

    await user.click(await screen.findByRole('button', {name: 'Cancel workflow'}));
    expect(request).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: 'Confirm cancellation'}));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3));
    expect(request.mock.calls[1]).toEqual([
      '/v1/workflows/workflow-1/cancel',
      {method: 'POST', body: '{}'},
    ]);
  });

  it('requires confirmation before disabling a schedule', async () => {
    const user = userEvent.setup();
    const schedule = {
      id: 'schedule-1',
      name: 'nightly',
      type: 'echo',
      interval_seconds: 3600,
      enabled: true,
      next_run_at: '2030-01-01T00:00:00Z',
    };
    const request = vi.fn(async (path: string) =>
      path === '/v1/schedules' ? [schedule] : undefined,
    );
    render(<Schedules api={apiMock(request)} notify={notify} principal={operator} />);

    await user.click(await screen.findByRole('button', {name: 'Disable'}));
    expect(request).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: 'Confirm disable'}));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3));
    expect(request.mock.calls[1]).toEqual(['/v1/schedules/schedule-1/disable', {method: 'POST'}]);
  });
});
