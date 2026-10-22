export const outputSpeedExplanation =
  'TPS = 非推理输出 Token ÷ 上游请求总耗时（发起请求到收到响应最后一段数据），不扣首响应。缺少上游计时的历史记录按请求总耗时估算。';

export function outputSpeed(row) {
  if (!row || Number(row.type) !== 2) return null;
  const metadata = row.metadata || {};
  const reasoning = Math.max(Number(metadata.reasoning_tokens) || 0, 0);
  const tokens = Math.max(Number(row.completion_tokens) - reasoning, 0);
  const measured = Object.prototype.hasOwnProperty.call(metadata, 'upstream_response_ms');
  const durationMs = Number(measured ? metadata.upstream_response_ms : row.request_time);
  if (!Number.isFinite(tokens) || !Number.isFinite(durationMs) || tokens <= 0 || durationMs <= 0) return null;
  return { tokens, durationMs, tps: (tokens * 1000) / durationMs, measured };
}

export function outputSpeedDetail(row) {
  const sample = outputSpeed(row);
  if (!sample) return '无有效输出或耗时，无法计算 TPS';
  return `${sample.tokens} 个非推理输出 Token ÷ ${(sample.durationMs / 1000).toFixed(3)} 秒${
    sample.measured ? '上游总耗时' : '请求总耗时（无上游计时，兼容估算）'
  }；不扣首响应时间`;
}
