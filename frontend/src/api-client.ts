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
  const inFlight = new Map<string, Promise<unknown>>();
  const cache = new Map<string, {value: unknown; expires: number}>();
  let generation = 0;
  return async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    const method = (options.method || 'GET').toUpperCase();
    if (options.body && !(options.body instanceof FormData) && !headers.has('Content-Type'))
      headers.set('Content-Type', 'application/json');
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) headers.set('X-CSRF-Token', csrfToken);
    const read = method === 'GET';
    const share = read && !options.signal && !options.headers;
    const key = `${generation}:${path}`;
    if (share && options.cache === 'force-cache') {
      const cached = cache.get(key);
      if (cached && cached.expires > Date.now()) return cached.value as T;
    }
    if (share && inFlight.has(key)) return inFlight.get(key) as Promise<T>;
    const execute = async (): Promise<T> => {
      try {
        for (let attempt = 0; ; attempt++) {
          try {
            const response = await fetch(path, {...options, credentials: 'same-origin', headers});
            const value = await parseResponse<T>(response);
            if (!read) {
              generation++;
              cache.clear();
            } else if (share && key === `${generation}:${path}`) {
              if (cache.size >= 100) cache.delete(cache.keys().next().value!);
              cache.set(key, {value, expires: Date.now() + 10_000});
            }
            return value;
          } catch (error: unknown) {
            const transient = error instanceof ApiError && [502, 503, 504].includes(error.status);
            if (!read || !transient || attempt >= 2 || options.signal?.aborted) throw error;
            await new Promise<void>((resolve) => setTimeout(resolve, 150 * 2 ** attempt));
            options.signal?.throwIfAborted();
          }
        }
      } catch (error: unknown) {
        if (error instanceof ApiError) onError?.(error);
        throw error;
      }
    };
    const promise = execute();
    if (share) inFlight.set(key, promise);
    try {
      return await promise;
    } finally {
      if (share && inFlight.get(key) === promise) inFlight.delete(key);
    }
  };
}

export async function revokeWebSession(csrfToken: string): Promise<void> {
  await createApiClient(csrfToken)('/v1/session', {method: 'DELETE'});
}
