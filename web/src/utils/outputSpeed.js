export const outputSpeedExplanation =
  'TPS = 输出 Token（扣除已记录的推理部分）÷（耗时 − 首响应）。未提供推理明细时无法确认是否包含推理。旧流式记录沿用原请求计时；缺少首响应时按完整耗时估算。';
export const responseTimeExplanation =
  '耗时与首响应均从发起请求开始，分别到收到最后一段和第一段响应数据；首响应不保证已有可见文本。旧记录保留原计时口径。';

export function responseDuration(row) {
  if (!row) return null;
  const m = row.metadata || {};
  const value = Number(Object.prototype.hasOwnProperty.call(m, 'upstream_response_ms') ? m.upstream_response_ms : row.request_time);
  return Number.isFinite(value) && value > 0 ? value : null;
}

export function firstResponse(row) {
  if (!row) return null;
  const m = row.metadata || {};
  const current = Number(m.response_metrics_version) >= 2;
  if (!current && !row.is_stream) return null;
  const first = Number(current ? m.response_first_ms : m.first_response);
  const limit = current ? responseDuration(row) : Number(row.request_time);
  return Number.isFinite(first) && first > 0 && first <= limit ? first : null;
}

export function outputSpeed(row) {
  if (!row || Number(row.type) !== 2) return null;
  const metadata = row.metadata || {};
  const reasoning = Math.max(Number(metadata.reasoning_tokens) || 0, 0);
  const tokens = Math.max(Number(row.completion_tokens) - reasoning, 0);
  const totalMs = responseDuration(row);
  if (!Number.isFinite(tokens) || tokens <= 0 || totalMs == null) return null;
  const firstMs = firstResponse(row);
  const legacyFirst = firstMs != null && !(Number(metadata.response_metrics_version) >= 2);
  const measured = Object.prototype.hasOwnProperty.call(metadata, 'upstream_response_ms') && !legacyFirst;
  const durationMs = (legacyFirst ? Number(row.request_time) : totalMs) - (firstMs ?? 0);
  if (!Number.isFinite(durationMs) || durationMs <= 0) return null;
  return { tokens, durationMs, firstMs, tps: (tokens * 1000) / durationMs, measured };
}

export function outputSpeedDetail(row) {
  const sample = outputSpeed(row);
  if (!sample) return '无有效输出或耗时，无法计算 TPS';
  const reasoning = Object.prototype.hasOwnProperty.call(row.metadata || {}, 'reasoning_tokens')
    ? '已扣除记录中的推理 Token'
    : '推理明细未提供';
  return `${sample.tokens} 个输出 Token ÷ ${(sample.durationMs / 1000).toFixed(3)} 秒${
    sample.measured ? '响应计时' : '请求计时（兼容估算）'
  }；${reasoning}；${sample.firstMs == null ? '首响应不可用，按完整耗时估算' : '已扣除首响应时间'}`;
}
