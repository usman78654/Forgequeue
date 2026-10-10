import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import type {WebSession} from './api-client';
import App from './App';

const authMocks = vi.hoisted(() => ({
  createApiClient: vi.fn(),
  exchangeApiKey: vi.fn(),
  refreshWebSession: vi.fn(),
  revokeWebSession: vi.fn(),
}));

vi.mock('./api-client', () => authMocks);

const session: WebSession = {
  principal: {id: 'principal-1', name: 'Test Admin', role: 'admin'},
  csrf_token: 'csrf-token',
  expires_at: '2030-01-01T00:00:00Z',
};

describe('App authentication state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    authMocks.createApiClient.mockReturnValue(vi.fn().mockResolvedValue({items: []}));
    authMocks.revokeWebSession.mockResolvedValue(undefined);
  });

  it('restores a signed-out state without issuing protected requests', async () => {
    authMocks.refreshWebSession.mockResolvedValue(null);
    render(<App />);

    expect(screen.getByText(/Restoring secure session/)).toBeVisible();
    expect(await screen.findByRole('heading', {name: 'Connect to ForgeQueue'})).toBeVisible();
    expect(authMocks.createApiClient.mock.results[0]?.value).not.toHaveBeenCalled();
  });

  it('restores and revokes an authenticated session', async () => {
    const user = userEvent.setup();
    authMocks.refreshWebSession.mockResolvedValue(session);
    render(<App />);

    const disconnect = await screen.findByRole('button', {name: 'Disconnect'});
    expect(screen.getByText('admin', {exact: true})).toBeVisible();
    await user.click(disconnect);

    expect(authMocks.revokeWebSession).toHaveBeenCalledWith('csrf-token');
    expect(await screen.findByRole('heading', {name: 'Connect to ForgeQueue'})).toBeVisible();
  });

  it('shows degraded service state and recovers when it is dismissed', async () => {
    const user = userEvent.setup();
    authMocks.refreshWebSession.mockResolvedValue(session);
    render(<App />);

    await screen.findByRole('button', {name: 'Disconnect'});
    const onError = authMocks.createApiClient.mock.calls.at(-1)?.[1];
    act(() => onError({status: 503, message: 'Broker health check failed'}));

    expect(
      screen.getByText(/ForgeQueue is degraded:/).closest('[role="status"]'),
    ).toHaveTextContent('ForgeQueue is degraded: Broker health check failed');
    await user.click(screen.getByRole('button', {name: 'Dismiss degraded service warning'}));
    expect(screen.queryByText(/ForgeQueue is degraded/)).not.toBeInTheDocument();
  });

  it('returns to a recoverable sign-in screen when a session expires', async () => {
    authMocks.refreshWebSession.mockResolvedValue(session);
    render(<App />);

    await screen.findByRole('button', {name: 'Disconnect'});
    const onError = authMocks.createApiClient.mock.calls.at(-1)?.[1];
    act(() => onError({status: 401, message: 'expired'}));

    expect(await screen.findByRole('heading', {name: 'Connect to ForgeQueue'})).toBeVisible();
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Your session expired. Connect again to continue.',
    );
  });

  it('warns when the browser goes offline and clears the warning after reconnection', async () => {
    authMocks.refreshWebSession.mockResolvedValue(null);
    render(<App />);
    await screen.findByRole('heading', {name: 'Connect to ForgeQueue'});

    act(() => window.dispatchEvent(new Event('offline')));
    expect(screen.getByRole('alert')).toHaveTextContent('You are offline');

    act(() => window.dispatchEvent(new Event('online')));
    expect(screen.queryByText(/You are offline/)).not.toBeInTheDocument();
  });
});
