// Thin client for the ZVintedWatch JSON API.

import type {
  Alert,
  AlertView,
  ActivityEvent,
  AuthStatus,
  ChatInfo,
  Destination,
  Match,
  Meta,
  NetworkSettings,
  PreviewResult,
  SearchInfo,
  SearchStatus,
  Status,
  TelegramSettings,
  TextFilter,
  Health,
} from './types';

/** Error returned by the API, carrying the HTTP status and server message. */
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/** Called when the server answers 401, so the app can show the login page. */
let onUnauthorized: () => void = () => {};
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (method !== 'GET') headers['X-ZVW'] = '1'; // CSRF protection header
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
    });
  } catch {
    throw new ApiError(0, 'Cannot reach ZVintedWatch. Is it still running?');
  }

  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/auth/login') onUnauthorized();
    throw new ApiError(res.status, (data as { error?: string }).error ?? `HTTP ${res.status}`);
  }
  return data as T;
}

const get = <T>(path: string) => request<T>('GET', path);
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {});
const put = <T>(path: string, body: unknown) => request<T>('PUT', path, body);
const del = <T>(path: string) => request<T>('DELETE', path);

export const api = {
  // Authentication
  authStatus: () => get<AuthStatus>('/api/auth/status'),
  login: (password: string) => post<{ authenticated: boolean }>('/api/auth/login', { password }),
  logout: () => post('/api/auth/logout'),
  changePassword: (current: string, next: string) =>
    put<{ required: boolean }>('/api/auth/password', { current, new: next }),

  // Overview
  meta: () => get<Meta>('/api/meta'),
  status: () => get<Status>('/api/status'),

  // Alerts
  alerts: (provider?: string) =>
    get<AlertView[]>('/api/alerts' + (provider ? `?provider=${encodeURIComponent(provider)}` : '')),
  createAlert: (a: Alert) => post<Alert>('/api/alerts', a),
  updateAlert: (a: Alert) => put<Alert>(`/api/alerts/${a.id}`, a),
  deleteAlert: (id: number) => del(`/api/alerts/${id}`),
  setAlertEnabled: (id: number, enabled: boolean) => post(`/api/alerts/${id}/enabled`, { enabled }),
  previewAlert: (a: Alert) => post<PreviewResult>('/api/alerts/preview', a),
  describeSearch: (provider: string, input: string, domain: string, alertId: number) =>
    post<SearchInfo>('/api/searches/describe', { provider, input, domain, alertId }),
  testMatcher: (filter: TextFilter, text: string) =>
    post<{ match: boolean }>('/api/matcher/test', { filter, text }),

  // Searches and matches
  searches: (provider?: string) =>
    get<SearchStatus[]>('/api/searches' + (provider ? `?provider=${encodeURIComponent(provider)}` : '')),
  refreshSearch: (id: number) => post(`/api/searches/${id}/refresh`),
  matches: (params: { provider?: string; alertId?: number; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (params.provider) q.set('provider', params.provider);
    if (params.alertId) q.set('alertId', String(params.alertId));
    if (params.limit) q.set('limit', String(params.limit));
    return get<Match[]>('/api/matches?' + q.toString());
  },

  // Telegram
  destinations: () => get<Destination[]>('/api/destinations'),
  createDestination: (d: Partial<Destination>) => post<Destination>('/api/destinations', d),
  updateDestination: (d: Destination) => put<Destination>(`/api/destinations/${d.id}`, d),
  deleteDestination: (id: number) => del(`/api/destinations/${id}`),
  telegram: () => get<TelegramSettings>('/api/telegram'),
  updateTelegram: (body: { token?: string; systemDestinationId?: number }) =>
    put<TelegramSettings>('/api/telegram', body),
  discoverChats: () => get<ChatInfo[]>('/api/telegram/chats'),
  testTelegram: (chatId: string) => post('/api/telegram/test', { chatId }),

  // Network
  network: () => get<Record<string, NetworkSettings>>('/api/settings/network'),
  updateNetwork: (provider: string, s: NetworkSettings) =>
    put<NetworkSettings>(`/api/settings/network/${provider}`, s),
  resetCooldown: (provider: string) => post<Health>(`/api/providers/${provider}/reset-cooldown`),

  // Activity
  activity: (limit = 500) => get<ActivityEvent[]>(`/api/activity?limit=${limit}`),
};
