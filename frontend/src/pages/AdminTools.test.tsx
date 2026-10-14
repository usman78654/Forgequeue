import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {AdminTools} from './AdminTools';
import type {ApiRequest} from '../api-client';

const principal = {id: 'admin', name: 'Admin', role: 'admin' as const};

describe('admin creation and quota safety', () => {
  it('displays a new secret once, refreshes metadata and clears it on close', async () => {
    const request = vi.fn(async (path: string) =>
      path === '/v1/admin/audit' ? {items: []} : {token: 'one-time-secret'},
    );
    const refresh = vi.fn(async () => undefined);
    const save = vi.spyOn(Storage.prototype, 'setItem');
    render(
      <AdminTools
        api={request as ApiRequest}
        notify={vi.fn()}
        principal={principal}
        onKeyCreated={refresh}
      />,
    );
    fireEvent.change(screen.getByLabelText('Principal ID'), {target: {value: 'owner'}});
    fireEvent.change(screen.getByLabelText('Key name'), {target: {value: 'deploy'}});
    fireEvent.click(screen.getByRole('button', {name: 'Create API key'}));
    expect(await screen.findByText('one-time-secret')).toBeInTheDocument();
    expect(refresh).toHaveBeenCalledOnce();
    expect(save).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {name: 'I have saved the key'}));
    expect(screen.queryByText('one-time-secret')).not.toBeInTheDocument();
    save.mockRestore();
  });

  it('does not update quotas until confirmation', async () => {
    const request = vi.fn(async () => ({items: []}));
    render(<AdminTools api={request as ApiRequest} notify={vi.fn()} principal={principal} />);
    fireEvent.change(screen.getByLabelText('Principal ID'), {target: {value: 'owner'}});
    fireEvent.click(screen.getByRole('button', {name: 'Update quotas'}));
    expect(request.mock.calls).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', {name: 'Confirm quota update'}));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        '/v1/admin/quotas/owner',
        expect.objectContaining({method: 'PUT'}),
      ),
    );
  });
});
