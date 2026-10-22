export const estimateExplanation =
  '近24小时计费日志数 ÷（计费日志数 + 错误日志数）。计费日志不等于成功请求；部分失败请求会同时留下两类日志，中途重试错误也可能未记录，因此仅作日志口径估算。';
export const eligible = (r) => r.global && r.enabled && !r.cool;
export function normalizeModel(m) {
  const context = m.info?.context_length;
  return {
    ...m,
    context: context
      ? context >= 1000000
        ? (context / 1000000).toFixed(1).replace('.0', '') + 'M'
        : context >= 1000
          ? Math.round(context / 1000) + 'K'
          : context
      : '',
    kind: (m.info?.input_modalities || []).map((x) => ({ text: '文本', image: '图像', audio: '音频', video: '视频' })[x] || x).join(' · '),
    routes: m.routes.map((r) => ({ ...r, number: r.id }))
  };
}
const empty = {
  consumeCount: 0,
  errorCount: 0,
  quota: 0,
  costQuota: 0,
  costSampleCount: 0,
  firstResponseMs: null,
  firstSampleCount: 0,
  tps: null,
  speedSampleCount: 0,
  latencyMs: null,
  promptTokens: 0,
  completionTokens: 0,
  ratio: null,
  ambiguous: 0,
  recentErrors: []
};
export function normalizeHistory(usage, model) {
  function stats(s) {
    return {
      ...empty,
      ...s,
      consumeLogDisabled: !usage.consumeTracked,
      errorLogDisabled: !usage.errorsTracked,
      recentErrors: (s?.recentErrors || []).map((e) => ({ ...e, metadata: { status_code: e.status, error_message: e.message } }))
    };
  }
  const byChannel = Object.fromEntries(model.routes.map((r) => [r.id, stats(usage.byModel[model.id]?.[r.id])]));
  const total = stats(usage.totals[model.id]);
  return {
    byChannel,
    asOf: usage.asOf,
    total: {
      ...total,
      attributed: Object.values(byChannel).filter((x) => !x.ambiguous).length,
      bindings: model.routes.length,
      incomplete: !usage.consumeTracked || !usage.errorsTracked
    }
  };
}
export function missingReason(s) {
  if (s.ambiguous) return s.consumeCount || s.errorCount ? '部分历史日志归属不明，仅统计可归属记录' : '历史模型归属不明';
  if (s.consumeLogDisabled) return '计费日志未开启';
  if (s.errorLogDisabled) return '错误日志未开启';
  if (!s.consumeCount && !s.errorCount) return '暂无调用数据';
  return null;
}
