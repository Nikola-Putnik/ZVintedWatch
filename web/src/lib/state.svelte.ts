// Global application state shared by every page (Svelte 5 runes).

import { api, ApiError, setUnauthorizedHandler } from './api';
import type { AuthStatus, Meta, ProviderInfo, Status } from './types';

export const app = $state({
  ready: false,
  bootError: '',
  needsLogin: false,
  auth: null as AuthStatus | null,
  meta: null as Meta | null,
  status: null as Status | null,
});

setUnauthorizedHandler(() => {
  app.needsLogin = true;
});

/** Loads the data every page needs. Shows the login page when required. */
export async function bootstrap() {
  try {
    app.auth = await api.authStatus();
    if (app.auth.required && !app.auth.authenticated) {
      app.needsLogin = true;
      app.ready = true;
      return;
    }
    app.needsLogin = false;
    const [meta, status] = await Promise.all([api.meta(), api.status()]);
    app.meta = meta;
    app.status = status;
    app.bootError = '';
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 401)) {
      app.bootError = e instanceof Error ? e.message : String(e);
    }
  } finally {
    app.ready = true;
  }
}

/** Refreshes the dashboard figures (health, counters). */
export async function refreshStatus() {
  if (app.needsLogin) return;
  try {
    app.status = await api.status();
  } catch {
    /* transient errors are shown by the pages that need the data */
  }
}

let statusTimer: ReturnType<typeof setInterval> | undefined;

/** Polls /api/status every few seconds while the app is open. */
export function startStatusPolling() {
  stopStatusPolling();
  statusTimer = setInterval(refreshStatus, 5000);
}

export function stopStatusPolling() {
  if (statusTimer) clearInterval(statusTimer);
}

export function providerInfo(id: string): ProviderInfo | undefined {
  return app.meta?.providers.find((p) => p.id === id);
}

// --- Toast notifications ----------------------------------------------------

export interface Toast {
  id: number;
  kind: 'success' | 'error' | 'info';
  text: string;
}

export const toasts = $state<Toast[]>([]);
let toastSeq = 0;

export function toast(text: string, kind: Toast['kind'] = 'success') {
  const id = ++toastSeq;
  toasts.push({ id, kind, text });
  setTimeout(() => dismissToast(id), kind === 'error' ? 7000 : 3500);
}

export function dismissToast(id: number) {
  const i = toasts.findIndex((t) => t.id === id);
  if (i >= 0) toasts.splice(i, 1);
}

/** Shows an error toast for a failed action. */
export function toastError(e: unknown) {
  toast(e instanceof Error ? e.message : String(e), 'error');
}
