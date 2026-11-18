// Display helpers.

const currencySymbols: Record<string, string> = {
  EUR: '€', GBP: '£', USD: '$', PLN: 'zł', CZK: 'Kč', SEK: 'kr', DKK: 'kr', HUF: 'Ft', RON: 'lei',
};

export function formatPrice(price: number, currency: string): string {
  if (!price || price <= 0) return '—';
  const symbol = currencySymbols[currency] ?? currency;
  const amount = price.toFixed(2).replace(/\.00$/, '');
  return currency === 'GBP' || currency === 'USD' ? `${symbol}${amount}` : `${amount} ${symbol}`;
}

/** Formats a duration in seconds: 45 → "45 s", 90 → "1 min 30 s", 3600 → "1 h". */
export function formatSeconds(sec: number): string {
  if (sec < 60) return `${sec} s`;
  if (sec < 3600) {
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return s ? `${m} min ${s} s` : `${m} min`;
  }
  const h = Math.floor(sec / 3600);
  const m = Math.round((sec % 3600) / 60);
  return m ? `${h} h ${m} min` : `${h} h`;
}

/** Formats a refresh interval: fixed or a random range. */
export function formatInterval(min: number, max: number): string {
  return min === max ? `every ${formatSeconds(min)}` : `every ${formatSeconds(min)} – ${formatSeconds(max)}`;
}

export function isZeroTime(iso?: string): boolean {
  return !iso || iso.startsWith('0001-');
}

/** "just now", "in 42 s", "3 min ago", "2 h ago", or a date for older times. */
export function relativeTime(iso: string | undefined, now = Date.now()): string {
  if (isZeroTime(iso)) return 'never';
  const t = new Date(iso!).getTime();
  const diff = Math.round((t - now) / 1000);
  const abs = Math.abs(diff);
  if (abs < 5) return 'just now';
  let text: string;
  if (abs < 60) text = `${abs} s`;
  else if (abs < 3600) text = `${Math.round(abs / 60)} min`;
  else if (abs < 86400) text = `${Math.round(abs / 3600)} h`;
  else return new Date(t).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
  return diff > 0 ? `in ${text}` : `${text} ago`;
}

export function clockTime(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number, l = 2) => String(n).padStart(l, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10240 ? 1 : 0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function plural(n: number, word: string, pluralWord = word + 's'): string {
  return `${n} ${n === 1 ? word : pluralWord}`;
}

/** Hostname + path of a URL, for compact display. */
export function shortUrl(url: string): string {
  try {
    const u = new URL(url);
    return u.host + u.pathname + (u.search.length > 1 ? u.search : '');
  } catch {
    return url;
  }
}
