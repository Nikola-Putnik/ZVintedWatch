// Live activity feed: loads recent events, then follows the Server-Sent
// Events stream. Shared by the dashboard and the Activity page.

import { api } from './api';
import type { ActivityEvent } from './types';

const MAX_EVENTS = 2000;

export const activity = $state({
  events: [] as ActivityEvent[],
  connected: false,
});

let source: EventSource | null = null;
let lastId = 0;

function push(e: ActivityEvent) {
  if (e.id <= lastId) return;
  lastId = e.id;
  activity.events.push(e);
  if (activity.events.length > MAX_EVENTS) {
    activity.events.splice(0, activity.events.length - MAX_EVENTS);
  }
}

export async function startActivity() {
  if (source) return;
  try {
    const recent = await api.activity(1000);
    recent.forEach(push);
  } catch {
    /* the stream below still works */
  }
  source = new EventSource('/api/activity/stream');
  source.onopen = () => (activity.connected = true);
  source.onerror = () => (activity.connected = false);
  source.onmessage = (msg) => {
    try {
      push(JSON.parse(msg.data) as ActivityEvent);
    } catch {
      /* ignore malformed events */
    }
  };
}

export function stopActivity() {
  source?.close();
  source = null;
  activity.connected = false;
}

export function clearActivity() {
  activity.events.splice(0, activity.events.length);
}
