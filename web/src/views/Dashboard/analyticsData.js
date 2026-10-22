import { API } from 'utils/api';
import { renderQuota, showError } from 'utils/common';
import { formatCompact } from './dashboardData';

export const DAY = 86400000;
export const defaults = {
  model: 'all',
  key: 'all',
  stream: 'all',
  effort: 'all',
  compare: true,
  composition: false,
  granularity: 'auto',
  zone: 'Asia/Shanghai',
  auto: false
};
export const labels = { '24h': '24h', 7: '7D', 30: '30D', 90: '90D', custom: '自定义' };
export const money = (n, d = 2) => (n == null ? '—' : renderQuota(n, d));
export const compact = formatCompact;
export const latency = (n) => (n == null || !Number.isFinite(n) ? '—' : n >= 1000 ? (n / 1000).toFixed(2) + ' s' : Math.round(n) + ' ms');
export const datetime = (t, o = { zone: 'Asia/Shanghai' }) =>
  new Intl.DateTimeFormat('zh-CN', {
    timeZone: o.zone,
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23'
  }).format(t);
export const dateinput = (t, o) => new Date(t + (o.zone === 'UTC' ? 0 : 8 * 3600000)).toISOString().slice(0, 16);
export const modelName = (id) => id || '未记录模型';
export const keyName = (id) => id || '未记录 Key';
export function queryParams(period, o) {
  return {
    range: String(period),
    timezone: o.zone,
    granularity: o.granularity,
    compare: o.compare,
    ...(period === 'custom' ? { start: Math.floor(o.start / 1000), end: Math.floor(o.end / 1000) } : {}),
    ...Object.fromEntries(
      [
        ['model', o.model],
        ['token', o.key],
        ['stream', o.stream],
        ['effort', o.effort]
      ].filter(([, v]) => v !== 'all')
    )
  };
}
export function normalizeStats(s = {}) {
  const count = (s.requests || 0) + (s.errors || 0);
  return {
    ...s,
    count,
    success: s.requests || 0,
    errors: s.errors || 0,
    cost: s.quota || 0,
    input: s.prompt_tokens || 0,
    output: s.completion_tokens || 0,
    tokens: (s.prompt_tokens || 0) + (s.completion_tokens || 0),
    read: s.cached_read_tokens || 0,
    write: s.cached_write_tokens || 0,
    uncached: Math.max(0, (s.prompt_tokens || 0) - (s.cached_read_tokens || 0) - (s.cached_write_tokens || 0)),
    cache: s.cache_hit_rate || 0,
    error: s.error_rate || 0,
    rate: count ? ((s.requests || 0) / count) * 100 : null,
    p50: s.first_p50_ms ?? null,
    p95: s.first_p95_ms ?? null,
    duration95: s.duration_p95_ms ?? null,
    tps: s.output_tokens_per_second ?? null,
    samples: s.performance_samples || 0,
    speedSamples: s.speed_samples || 0,
    saved: s.cache_requests && !s.cache_priced_requests ? null : s.cache_savings_quota || 0,
    baseline: s.cache_requests === s.cache_priced_requests ? (s.quota || 0) + (s.cache_savings_quota || 0) : null,
    previous: s.previous ? normalizeStats(s.previous) : null
  };
}
export function normalizeUsage(u, o) {
  return {
    ...u,
    rates_tracked: u.errors_tracked && u.consume_tracked,
    range: { start: u.range.start * 1000, end: u.range.end * 1000 },
    prev: { start: u.previous_range.start * 1000, end: u.previous_range.end * 1000 },
    totals: normalizeStats(u.totals),
    previous: normalizeStats(u.previous || {}),
    previousComplete: !!u.previous && (u.previous.requests || 0) + (u.previous.errors || 0) > 0,
    unit: u.granularity === 'hour' ? 3600000 : u.granularity === 'week' ? 7 * DAY : DAY,
    models: u.models.map((m) => ({ ...normalizeStats(m), id: m.model_name })),
    keys: u.keys.map((k) => ({ ...normalizeStats(k), id: k.token_name ?? k.model_name })),
    dayRows: u.daily.map((d) => ({ ...normalizeStats(d), date: d.date, start: d.start * 1000, end: d.end * 1000 })),
    daily: u.buckets.map((d) => ({
      ...normalizeStats(d),
      start: d.start * 1000,
      end: d.end * 1000,
      date: datetime(d.start * 1000, o),
      label: u.granularity === 'hour' ? datetime(d.start * 1000, o).slice(-5) : datetime(d.start * 1000, o).split(' ')[0],
      previous: normalizeStats(d.previous || {}),
      previousStart: d.previous_start * 1000,
      previousEnd: d.previous_end * 1000,
      models: Object.fromEntries(Object.entries(d.models || {}).map(([name, s]) => [name, normalizeStats(s)]))
    }))
  };
}
export function emptyUsage() {
  return {
    totals: normalizeStats(),
    previous: normalizeStats(),
    range: { start: Date.now() - 30 * DAY, end: Date.now() },
    daily: [],
    dayRows: [],
    models: [],
    keys: [],
    filters: { models: [], tokens: [], efforts: [] },
    errors_tracked: true,
    rates_tracked: true,
    consume_tracked: true,
    unit: DAY
  };
}
export function requestParams(p, filter = {}) {
  const o = p.options,
    range = p.usage.range;
  return {
    ...queryParams('custom', { ...o, start: filter.start ?? range.start, end: filter.end ?? range.end }),
    compare: false,
    ...(filter.model !== undefined ? { model: filter.model } : {}),
    ...(filter.key !== undefined ? { token: filter.key } : {}),
    ...(filter.error ? { status: 'error' } : {}),
    ...(filter.minFirst ? { min_first: filter.minFirst } : {}),
    ...(filter.status && filter.status !== 'all' ? { status: filter.status } : {}),
    ...(filter.search ? { search: filter.search } : {})
  };
}
export function csv(name, headers, rows) {
  const q = (v) => {
    let s = String(v ?? '');
    if (/^[=+@\t\r]/.test(s) || /^-[^\d]/.test(s)) s = "'" + s;
    return '"' + s.replaceAll('"', '""') + '"';
  };
  downloadBlob(
    new Blob(['\uFEFF' + [headers, ...rows].map((r) => r.map(q).join(',')).join('\r\n')], { type: 'text/csv;charset=utf-8' }),
    'dashboard-' + name + '.csv'
  );
}
export function downloadBlob(blob, name) {
  const url = URL.createObjectURL(blob),
    a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export async function exportRequests(params) {
  try {
    const res = await API.get('/api/user/dashboard/requests/export', { params, responseType: 'blob', skipErrorToast: true });
    if (!res.headers['content-type']?.includes('text/csv')) {
      const body = JSON.parse(await res.data.text());
      throw new Error(body.message || '导出失败');
    }
    downloadBlob(res.data, 'dashboard-requests.csv');
  } catch (e) {
    showError(e.message || '导出失败');
  }
}
