import {afterEach, describe, expect, it, vi} from 'vitest';
import {createApiClient, exchangeApiKey, refreshWebSession} from './api-client';

const session = {
  principal: {id: 'principal-1', name: 'Operator', role: 'operator' as const},
  csrf_token: 'csrf-token',
  expires_at: '2030-01-01T00:00:00Z',
};

function response(body: unknown, status = 200) {
  return new Response(body === null ? null : JSON.stringify(body), {
    status,
    headers: {'Content-Type': 'application/json'},
  });
}

describe('session API client', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('exchanges an API key without retaining it in the returned client state', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response(session, 201));
    vi.stubGlobal('fetch', fetchMock);

    await expect(exchangeApiKey('fq_secret')).resolves.toEqual(session);
    expect(fetchMock).toHaveBeenCalledWith('/v1/session', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {Authorization: 'Bearer fq_secret'},
    });
  });

  it('adds CSRF protection only to unsafe requests', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(response({items: []}))
      .mockResolvedValueOnce(response({items: []}));
    vi.stubGlobal('fetch', fetchMock);
    const request = createApiClient('csrf-token');

    await request('/v1/jobs');
    await request('/v1/jobs', {method: 'POST', body: '{}'});

    const getHeaders = fetchMock.mock.calls[0]?.[1]?.headers as Headers;
    const postHeaders = fetchMock.mock.calls[1]?.[1]?.headers as Headers;
    expect(getHeaders.has('X-CSRF-Token')).toBe(false);
    expect(postHeaders.get('X-CSRF-Token')).toBe('csrf-token');
    expect(postHeaders.get('Content-Type')).toBe('application/json');
  });

  it('treats an absent browser session as signed out', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({detail: 'missing'}, 401)));
    await expect(refreshWebSession()).resolves.toBeNull();
  });

  it('reports typed API failures to the application error handler', async () => {
    const onError = vi.fn();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(response({detail: 'Broker unavailable'}, 503)),
    );

    await expect(createApiClient('csrf-token', onError)('/v1/jobs')).rejects.toMatchObject({
      status: 503,
      message: 'Broker unavailable',
    });
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({status: 503, message: 'Broker unavailable'}),
    );
  });
});
