export interface Principal {
  id: string;
  name: string;
  role: 'viewer' | 'operator' | 'admin';
}

export interface WebSession {
  principal: Principal;
  csrf_token: string;
  expires_at: string;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const problem = (await response.json().catch(() => ({}))) as {detail?: string; title?: string};
    throw new ApiError(
      problem.detail || problem.title || response.statusText || 'Request failed',
      response.status,
    );
  }
  if (response.status === 204) return null as T;
  return response.json() as Promise<T>;
}

export async function exchangeApiKey(apiKey: string): Promise<WebSession> {
  const response = await fetch('/v1/session', {
    method: 'POST',
    credentials: 'same-origin',
    headers: {Authorization: `Bearer ${apiKey}`},
  });
  return parseResponse<WebSession>(response);
}

export async function refreshWebSession(): Promise<WebSession | null> {
  const response = await fetch('/v1/session', {credentials: 'same-origin'});
  if (response.status === 401) return null;
  return parseResponse<WebSession>(response);
}

export type ApiRequest = <T = unknown>(path: string, options?: RequestInit) => Promise<T>;
export type ApiErrorHandler = (error: ApiError) => void;

export function createApiClient(csrfToken: string, onError?: ApiErrorHandler): ApiRequest {
  return async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    const method = (options.method || 'GET').toUpperCase();
    if (options.body && !(options.body instanceof FormData) && !headers.has('Content-Type'))
      headers.set('Content-Type', 'application/json');
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) headers.set('X-CSRF-Token', csrfToken);
    const response = await fetch(path, {...options, credentials: 'same-origin', headers});
    try {
      return await parseResponse<T>(response);
    } catch (error: unknown) {
      if (error instanceof ApiError) onError?.(error);
      throw error;
    }
  };
}

export async function revokeWebSession(csrfToken: string): Promise<void> {
  await createApiClient(csrfToken)('/v1/session', {method: 'DELETE'});
}
