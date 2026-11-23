// Types mirroring the JSON returned by the Go API (internal/web).

export type MatchMode = 'all' | 'any';

export interface TextFilter {
  include: string[];
  includeMode: MatchMode;
  exclude: string[];
}

export interface Alert {
  id: number;
  provider: string;
  name: string;
  enabled: boolean;
  searchInput: string;
  domain: string;
  title: TextFilter;
  description: TextFilter;
  priceMin: number | null;
  priceMax: number | null;
  intervalMinSec: number;
  intervalMaxSec: number;
  destinationIds: number[];
  createdAt?: string;
  updatedAt?: string;
}

export interface AlertView extends Alert {
  searchLabel: string;
  searchKey: string;
  searchUrl: string;
  searchId: number;
  searchError?: string;
  sharedWith: number;
}

export interface ProviderInfo {
  id: string;
  name: string;
  color: string;
  website: string;
  domains: string[];
  defaultDomain: string;
  minIntervalSec: number;
  recommendedIntervalSec: number;
  descriptionInSearch: boolean;
  searchPlaceholder: string;
  searchHelp: string;
}

export interface Meta {
  appName: string;
  version: string;
  providers: ProviderInfo[];
  loopback: boolean;
}

export interface LaneHealth {
  label: string;
  coolingDown: boolean;
  cooldownUntil: string;
  reason: string;
}

export interface Health {
  state: 'ok' | 'degraded' | 'cooldown' | 'error';
  cooldownUntil: string;
  reason: string;
  requestsLastHour: number;
  lanes: LaneHealth[];
}

export interface ProviderStatus {
  id: string;
  name: string;
  color: string;
  /** False when the whole site is paused by the user. */
  enabled: boolean;
  health: Health;
  alerts: number;
  enabledAlerts: number;
  searches: number;
  requestsPerHour: number;
  matchesToday: number;
}

export interface Status {
  providers: ProviderStatus[];
  /** Alerts whose site module is not installed in this version. */
  orphanAlerts: number;
  telegram: { configured: boolean; botUsername: string; destinations: number };
}

export interface Listing {
  provider: string;
  id: string;
  title: string;
  description?: string;
  hasDescription: boolean;
  price: number;
  currency: string;
  brand?: string;
  size?: string;
  condition?: string;
  category?: string;
  location?: string;
  url: string;
  photoUrl?: string;
  promoted: boolean;
}

export interface Match {
  id: number;
  alertId: number;
  alertName: string;
  provider: string;
  listing: Listing;
  matchedAt: string;
  delivery: 'pending' | 'sent' | 'failed' | 'none';
}

export interface AlertRef {
  id: number;
  name: string;
}

export interface SearchStatus {
  id: number;
  provider: string;
  key: string;
  label: string;
  webUrl: string;
  seeded: boolean;
  lastPolledAt: string;
  lastStatus: string;
  lastError: string;
  alerts: AlertRef[];
  minIntervalSec: number;
  maxIntervalSec: number;
  nextRun: string;
  running: boolean;
  lastNew: number;
  lastResults: number;
  pendingItems: number;
  requestsPerHour: number;
}

export interface SearchInfo {
  label: string;
  webUrl: string;
  key: string;
  sharedWith: AlertRef[];
}

export interface PreviewItem {
  listing: Listing;
  priceOk: boolean;
  titleOk: boolean;
  match: boolean;
}

export interface PreviewResult {
  label: string;
  webUrl: string;
  cached: boolean;
  fetchedAt: string;
  descriptionRules: boolean;
  matches: number;
  items: PreviewItem[];
}

export interface Destination {
  id: number;
  name: string;
  chatId: string;
  createdAt?: string;
}

export interface TelegramSettings {
  configured: boolean;
  botUsername: string;
  tokenHint: string;
  systemDestinationId: number;
}

export interface ChatInfo {
  id: string;
  type: string;
  title: string;
}

export interface NetworkSettings {
  minGapMs: number;
  jitterMs: number;
  timeoutSec: number;
  proxies: string[];
}

export type EventKind = 'request' | 'poll' | 'match' | 'notify' | 'system' | 'health';
export type EventLevel = 'debug' | 'info' | 'warn' | 'error';

export interface ActivityEvent {
  id: number;
  time: string;
  level: EventLevel;
  kind: EventKind;
  provider?: string;
  searchId?: number;
  search?: string;
  message: string;
  http?: {
    method: string;
    url: string;
    status: number;
    durationMs: number;
    bytes: number;
    lane?: string;
    error?: string;
  };
  details?: string[];
}

export interface AuthStatus {
  required: boolean;
  authenticated: boolean;
  loopback: boolean;
}
