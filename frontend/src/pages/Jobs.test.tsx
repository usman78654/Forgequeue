import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, it, vi} from 'vitest';
import type {ApiRequest} from '../api-client';
import type {Notify} from '../types';
import {Jobs} from './Jobs';

function apiMock(implementation: ReturnType<typeof vi.fn>): ApiRequest {
  return implementation as unknown as ApiRequest;
}

describe('Jobs', () => {
  it('moves from loading to an empty state', async () => {
    let resolveRequest: ((value: {items: []}) => void) | undefined;
    const request = new Promise<{items: []}>((resolve) => {
      resolveRequest = resolve;
    });
    const api = apiMock(vi.fn().mockReturnValue(request));
    render(<Jobs api={api} notify={vi.fn() as Notify} />);

    expect(screen.getByRole('status')).toHaveTextContent('Loading jobs');
    resolveRequest?.({items: []});
    expect(await screen.findByText('No jobs found')).toBeVisible();
  });

  it('reports API failures through the shared notification channel', async () => {
    const notify = vi.fn();
    const api = apiMock(vi.fn().mockRejectedValue(new Error('jobs unavailable')));
    render(<Jobs api={api} notify={notify as Notify} />);

    await vi.waitFor(() => expect(notify).toHaveBeenCalledWith('error', 'jobs unavailable'));
  });

  it('validates malformed submission JSON before sending a request', async () => {
    const user = userEvent.setup();
    const request = vi.fn().mockResolvedValue({items: []});
    const api = apiMock(request);
    render(<Jobs api={api} notify={vi.fn() as Notify} />);
    await screen.findByText('No jobs found');

    const openDialog = screen.getByRole('button', {name: 'Submit job'});
    openDialog.focus();
    await user.keyboard('{Enter}');
    const payload = screen.getByLabelText('Payload');
    await user.clear(payload);
    await user.type(payload, 'not-json');
    const submit = screen.getByRole('button', {name: 'Submit'});
    submit.focus();
    await user.keyboard('{Enter}');

    expect(screen.getByRole('alert')).toHaveTextContent('Payload must be valid JSON.');
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('requires confirmation before cancelling a job', async () => {
    const user = userEvent.setup();
    const job = {
      id: 'job-123456789',
      type: 'echo',
      queue: 'jobs.default',
      status: 'queued',
      attempt: 0,
      max_attempts: 3,
      created_at: '2030-01-01T00:00:00Z',
    };
    const request = vi.fn().mockResolvedValue({items: [job]});
    const api = apiMock(request);
    render(
      <Jobs
        api={api}
        notify={vi.fn() as Notify}
        principal={{id: '1', name: 'Operator', role: 'operator'}}
      />,
    );
    await screen.findByRole('button', {name: 'Cancel'});

    await user.click(screen.getByRole('button', {name: 'Cancel'}));
    expect(screen.getByRole('dialog', {name: 'Cancel job?'})).toBeVisible();
    expect(request).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: 'Confirm cancellation'}));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3));
    expect(request.mock.calls[1]?.[0]).toBe('/v1/jobs/job-123456789/cancel');
  });
});
