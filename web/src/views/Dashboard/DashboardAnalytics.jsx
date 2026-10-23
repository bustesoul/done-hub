import PropTypes from 'prop-types';
import { useEffect, useState } from 'react';
import {
  Box,
  Button,
  ButtonGroup,
  Checkbox,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Divider,
  FormControlLabel,
  IconButton,
  MenuItem,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Alert
} from '@mui/material';
import { alpha } from '@mui/material/styles';
import { IconX, IconDownload, IconBookmark } from '@tabler/icons-react';
import { DAY, labels, money, compact, latency, datetime, dateinput, modelName, keyName, csv } from './analyticsData';
import { RequestDrawer } from './DashboardRequests';
import { showSuccess } from 'utils/common';
import { outputSpeedExplanation, responseTimeExplanation } from 'utils/outputSpeed';

export const AnalyticsSection = ({ title, subtitle, action, children, pad = true }) => (
  <Paper variant="outlined" sx={{ borderRadius: 3, overflow: 'hidden', height: '100%' }}>
    <Stack direction="row" alignItems="flex-start" justifyContent="space-between" spacing={2} sx={{ px: 2.5, pt: 2.4, pb: pad ? 0 : 2 }}>
      <Box>
        <Typography variant="h3" sx={{ fontWeight: 720, letterSpacing: '-.025em' }}>
          {title}
        </Typography>
        {subtitle && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {subtitle}
          </Typography>
        )}
      </Box>
      {action}
    </Stack>
    <Box sx={{ px: pad ? 2.5 : 0, pb: pad ? 2.5 : 0 }}>{children}</Box>
  </Paper>
);
const select = (label, value, onChange, items, width = 150) => (
  <TextField
    select
    label={label}
    size="small"
    value={value}
    onChange={(e) => onChange(e.target.value)}
    sx={{ minWidth: width }}
    inputProps={{ 'aria-label': label }}
  >
    {(!items.some(([v]) => v === value) ? [...items, [value, value || '未记录']] : items).map(([v, l]) => (
      <MenuItem value={v} key={v}>
        {l}
      </MenuItem>
    ))}
  </TextField>
);
export function AnalyticsControls({ analytics: p }) {
  return (
    <Stack
      spacing={1.5}
      sx={{ '& .MuiButton-root': { whiteSpace: 'nowrap', flexShrink: 0 }, '& .MuiFormControl-root': { maxWidth: '100%' } }}
    >
      <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap" useFlexGap>
        {select(
          '模型',
          p.options.model,
          (v) => p.update('model', v),
          [['all', '全部模型'], ...p.usage.filters.models.map((m) => [m, modelName(m)])],
          185
        )}
        {select(
          'Key 名称',
          p.options.key,
          (v) => p.update('key', v),
          [['all', '全部 Key'], ...p.usage.filters.tokens.map((k) => [k, keyName(k)])],
          180
        )}
        {select(
          '请求类型',
          p.options.stream,
          (v) => p.update('stream', v),
          [
            ['all', '全部'],
            ['stream', '流式'],
            ['nonstream', '非流式']
          ],
          125
        )}
        <Button size="small" onClick={() => p.setDialog({ type: 'filters' })}>
          更多筛选{p.options.effort !== 'all' ? ' · 1' : ''}
        </Button>
        {(p.options.model !== 'all' || p.options.key !== 'all' || p.options.stream !== 'all' || p.options.effort !== 'all') && (
          <Button size="small" onClick={() => p.setOptions((o) => ({ ...o, model: 'all', key: 'all', stream: 'all', effort: 'all' }))}>
            清除
          </Button>
        )}
        <Box sx={{ flex: 1 }} />
        <FormControlLabel
          sx={{ mr: 0 }}
          control={<Checkbox size="small" checked={p.options.compare} onChange={(e) => p.update('compare', e.target.checked)} />}
          label={<Typography variant="caption">对比上期</Typography>}
        />
        <Button size="small" startIcon={<IconBookmark size={15} />} onClick={() => p.setDialog({ type: 'views' })}>
          保存视图
        </Button>
        <Button
          size="small"
          disabled={p.loading || !!p.error}
          startIcon={<IconDownload size={15} />}
          onClick={() => p.setDialog({ type: 'export' })}
        >
          导出
        </Button>
      </Stack>
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <Typography variant="caption" color="text.secondary">
          {datetime(p.usage.range.start, p.options)} — {datetime(p.usage.range.end, p.options)} ·{' '}
          {p.options.zone === 'UTC' ? 'UTC' : 'UTC+8'} · {p.usage.unit < DAY ? '按小时' : p.usage.unit === DAY ? '按天' : '按周'}聚合
        </Typography>
        <Button size="small" sx={{ minWidth: 0, p: 0, fontSize: 11 }} onClick={() => p.setDialog({ type: 'date' })}>
          调整范围
        </Button>
        <Box sx={{ flex: 1 }} />
        <FormControlLabel
          control={<Checkbox size="small" checked={p.options.auto} onChange={(e) => p.update('auto', e.target.checked)} />}
          label={
            <Typography variant="caption" color="text.secondary">
              每 30 秒刷新
            </Typography>
          }
        />
      </Stack>
      {!p.loading && !p.usage.previousComplete && p.options.compare && (
        <Alert severity="info">上期暂无可比记录或超出查询范围。统计仅包含仍保留的日志。</Alert>
      )}
      {!p.usage.errors_tracked && <Alert severity="warning">错误采集不完整，成功率与错误率不参与比较。</Alert>}
      {!p.usage.consume_tracked && <Alert severity="warning">消费日志记录已关闭，当前统计可能不完整。</Alert>}
      <Typography variant="caption" color="text.secondary">
        Key 按日志名称分组，同名合并；历史改名按当时名称统计。成功率仅反映已记录的请求。
      </Typography>
    </Stack>
  );
}
export function Comparison({ analytics: p, field }) {
  if (!p.options.compare) return null;
  const a = p.usage.totals[field],
    b = p.usage.previous[field];
  let text = '暂无可比样本',
    color = 'text.secondary';
  if (
    p.usage.previousComplete &&
    a != null &&
    b != null &&
    (field !== 'average_latency_ms' || (p.usage.totals.duration_samples && p.usage.previous.duration_samples))
  ) {
    const points = field === 'cache_hit_rate';
    const n = points ? a - b : b ? ((a - b) / b) * 100 : 0;
    text = !b ? (a ? '新增用量' : '与上期持平') : `${n >= 0 ? '↑' : '↓'} ${Math.abs(n).toFixed(1)}${points ? ' pp' : '%'} · 较上期`;
    if (field === 'average_latency_ms') color = n > 0 ? 'warning.main' : 'success.main';
  }
  return (
    <Typography variant="caption" sx={{ display: 'block', mt: 0.8, color }}>
      {p.usage.previousComplete ? text : '上期暂无可比记录'}
    </Typography>
  );
}
export function TrendExtra({ analytics: p }) {
  return (
    <Button size="small" onClick={() => p.setDialog({ type: 'chart' })}>
      图表设置
    </Button>
  );
}
export function ModelActions({ analytics: p }) {
  return (
    <Stack spacing={0.5} alignItems="flex-end" sx={{ flexShrink: 0 }}>
      <ButtonGroup size="small" sx={{ whiteSpace: 'nowrap' }}>
        {[
          ['cost', '消费'],
          ['performance', '性能'],
          ['key', 'Key']
        ].map(([v, l]) => (
          <Button key={v} variant={p.rank === v ? 'contained' : 'text'} onClick={() => p.setRank(v)}>
            {l}
          </Button>
        ))}
      </ButtonGroup>
      <Button size="small" onClick={() => p.setDialog({ type: 'models' })}>
        完整分析
      </Button>
    </Stack>
  );
}
export function ModelInline({ analytics: p, model }) {
  if (p.rank !== 'performance') return null;
  return (
    <Stack direction="row" spacing={1.2} sx={{ mt: 0.7 }} flexWrap="wrap">
      <Typography variant="caption" color="text.secondary">
        首响应 P95{' '}
        <Box component="b" sx={{ color: 'text.primary' }}>
          {latency(model.p95)}
        </Box>
      </Typography>
      <Typography variant="caption" color="text.secondary">
        {model.tps == null ? '—' : model.tps.toFixed(2) + ' t/s'}
      </Typography>
      <Typography variant="caption" color={model.samples < 20 ? 'warning.main' : 'text.secondary'}>
        首响应 {model.samples} 样本
      </Typography>
      <Typography variant="caption" color={model.speedSamples < 20 ? 'warning.main' : 'text.secondary'}>
        TPS {model.speedSamples} 样本
      </Typography>
    </Stack>
  );
}
export function CacheBenefit({ analytics: p }) {
  return (
    <>
      <Divider sx={{ my: 2 }} />
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Box>
          <Typography variant="caption" color="text.secondary">
            缓存净节省 · 估算
          </Typography>
          <Typography variant="h3" color="success.main" sx={{ mt: 0.5 }}>
            {money(p.usage.totals.saved)}
          </Typography>
        </Box>
        <Box sx={{ textAlign: 'right' }}>
          <Typography variant="caption" color="text.secondary">
            假设无缓存总费用 {money(p.usage.totals.baseline)}
          </Typography>
          <Button size="small" sx={{ display: 'block', ml: 'auto' }} onClick={() => p.setDialog({ type: 'cache' })}>
            查看计算与模型分布
          </Button>
        </Box>
      </Stack>
    </>
  );
}
function changes(p) {
  const a = p.usage.totals,
    items = [];
  const slow = p.usage.models
    .filter((m) => m.previous)
    .map((m) => ({ m, prev: m.previous }))
    .find(({ m, prev }) => m.samples >= 20 && prev.samples >= 20 && m.p50 > prev.p50 * 1.25);
  if (p.usage.previousComplete && slow)
    items.push({
      title: modelName(slow.m.id) + ' 首响应变慢',
      detail: `P50 ${latency(slow.m.p50)}，较上期 +${((slow.m.p50 / slow.prev.p50 - 1) * 100).toFixed(0)}%，${slow.m.samples} 个样本。`,
      filter: { model: slow.m.id, minFirst: slow.m.p95 }
    });
  if (a.errors && p.usage.errors_tracked)
    items.push({ title: `${a.errors} 次请求失败`, detail: '点击查看限流、超时和上游错误。', filter: { error: true } });
  const key = p.usage.keys[0];
  if (key)
    items.push({
      title: keyName(key.id),
      detail: `占当前消费 ${((key.cost / (a.cost || 1)) * 100).toFixed(1)}%，${money(key.cost)}。`,
      filter: { key: key.id }
    });
  return items;
}
export function QualityAdditions({ analytics: p }) {
  return (
    <>
      <Divider />
      <Stack direction="row" justifyContent="space-between">
        <Box>
          <Typography variant="caption" color="text.secondary">
            首响应 P95
          </Typography>
          <Typography fontWeight={650}>{latency(p.usage.totals.p95)}</Typography>
        </Box>
        <Box sx={{ textAlign: 'right' }}>
          <Typography variant="caption" color="text.secondary">
            输出 TPS · 中位数
          </Typography>
          <Typography fontWeight={650}>{p.usage.totals.tps == null ? '—' : p.usage.totals.tps.toFixed(2) + ' t/s'}</Typography>
        </Box>
      </Stack>
      <Stack direction="row" justifyContent="space-between">
        <Button size="small" onClick={() => p.setDialog({ type: 'changes' })}>
          查看 {changes(p).length} 项变化
        </Button>
        <Button size="small" onClick={() => p.openLogs({ error: true })} disabled={!p.usage.errors_tracked}>
          失败请求
        </Button>
      </Stack>
    </>
  );
}
export function createAnalyticsTrendChart(p, metric, theme) {
  const split = p.options.composition && ['quota', 'tokens', 'requests'].includes(metric);
  const data = p.usage.daily,
    modelDefs = p.usage.models.map((m, i) => ({
      id: m.id,
      name: m.model_name,
      color: ['#165DFF', '#14B8A6', '#7C3AED', '#F59E0B', '#EC4899', '#64748B'][i % 6]
    })),
    statsKey =
      { quota: 'cost', tokens: 'tokens', requests: 'count', p50: 'p50', p95: 'p95', error: 'error', cache: 'cache' }[metric] || 'cost';
  let series;
  if (split) series = modelDefs.map((m) => ({ name: m.name, data: data.map((d) => d.models[m.id]?.[statsKey] || 0) }));
  else if (metric === 'tokens')
    series = [
      { name: '未缓存输入', data: data.map((d) => d.uncached) },
      { name: '缓存读取', data: data.map((d) => d.read) },
      { name: '缓存写入', data: data.map((d) => d.write) },
      { name: '输出', data: data.map((d) => d.output) }
    ];
  else
    series = [
      {
        name: { quota: '消费', requests: '请求', p50: '首响应 P50', p95: '首响应 P95', error: '失败率', cache: '缓存命中率' }[metric],
        data: data.map((d) =>
          ['p50', 'p95', 'error', 'cache'].includes(metric) && !d.count
            ? null
            : metric === 'error' && !p.usage.rates_tracked
              ? null
              : d[statsKey]
        )
      }
    ];
  if (p.options.compare && p.usage.previousComplete && !split && metric !== 'tokens')
    series.push({
      name: '上一周期',
      data: data.map((d) =>
        ['p50', 'p95', 'error', 'cache'].includes(metric) && !d.previous.count
          ? null
          : metric === 'error' && !p.usage.rates_tracked
            ? null
            : d.previous[statsKey]
      )
    });
  const format = (v) =>
    v == null
      ? '—'
      : ['p50', 'p95'].includes(metric)
        ? latency(v)
        : ['error', 'cache'].includes(metric)
          ? v.toFixed(1) + '%'
          : metric === 'quota'
            ? money(v)
            : compact(v);
  const click = (_, chart, config) => {
    const d = data[config.dataPointIndex];
    const previous = series[config.seriesIndex]?.name === '上一周期';
    if (d && (!previous || d.previousStart))
      p.openLogs({
        start: previous ? d.previousStart : d.start,
        end: previous ? d.previousEnd : d.end,
        ...(split && config.seriesIndex >= 0 ? { model: modelDefs[config.seriesIndex]?.id } : {})
      });
  };
  return {
    series,
    options: {
      chart: {
        animations: { enabled: false },
        toolbar: { show: false },
        zoom: { enabled: false },
        fontFamily: 'Inter, Public Sans, sans-serif',
        background: 'transparent',
        stacked: split,
        events: { dataPointSelection: click, markerClick: click }
      },
      colors: split
        ? modelDefs.map((m) => m.color)
        : ['#165DFF', ...(metric === 'tokens' ? ['#14B8A6', '#93C5FD', '#7C3AED'] : ['#A9B8D3'])],
      dataLabels: { enabled: false },
      stroke: { curve: 'smooth', width: 2.5, dashArray: series.map((s) => (s.name === '上一周期' ? 5 : 0)) },
      fill: { type: 'gradient', gradient: { shadeIntensity: 0, opacityFrom: 0.25, opacityTo: 0.02, stops: [0, 90, 100] } },
      grid: { borderColor: alpha(theme.palette.text.primary, 0.07), strokeDashArray: 4, padding: { left: 4, right: 10 } },
      xaxis: {
        categories: data.map((d) => d.label),
        axisBorder: { show: false },
        axisTicks: { show: false },
        labels: { style: { colors: theme.palette.text.secondary } },
        tickAmount: Math.min(8, data.length)
      },
      yaxis: { labels: { style: { colors: theme.palette.text.secondary }, formatter: format } },
      legend: { show: series.length > 1, position: 'top', horizontalAlign: 'right', labels: { colors: theme.palette.text.secondary } },
      markers: { size: split ? 0 : 3, hover: { size: 5 } },
      tooltip: { theme: theme.palette.mode, shared: false, intersect: true, x: { show: true }, y: { formatter: format } },
      plotOptions: { bar: { columnWidth: '55%', borderRadius: 3 } }
    }
  };
}

export function AnalyticsDialogs({ analytics: p }) {
  const [form, setForm] = useState({}),
    [error, setError] = useState('');
  useEffect(() => {
    setError('');
    setForm({
      start: dateinput(p.usage.range.start, p.options),
      end: dateinput(p.usage.range.end, p.options),
      name: labels[p.period] + ' · ' + (p.options.model === 'all' ? '全部模型' : modelName(p.options.model)),
      effort: p.options.effort,
      zone: p.options.zone,
      granularity: p.options.granularity
    });
    // Initialize the form when opening; background refresh must not erase edits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p.dialog]);
  const close = () => p.setDialog(null),
    type = p.dialog?.type;
  const saveViews = p.saveViews;
  const headings = {
    date: '时间范围',
    filters: '更多筛选',
    chart: '图表设置',
    views: '保存与管理视图',
    export: '导出当前分析',
    models: '完整模型分析',
    cache: '缓存收益分析',
    changes: '值得注意的变化',
    about: '统计口径'
  };
  const exportModels = () =>
    csv(
      'models',
      [
        '模型',
        '请求',
        '成功率%',
        '首响应P50ms',
        '首响应P95ms',
        '耗时P95ms',
        '输出tokens/s',
        '首响应样本',
        'TPS样本',
        '输入',
        '输出',
        '缓存命中率%',
        '缓存净节省额度',
        '消费额度'
      ],
      p.usage.models.map((m) => [
        m.model_name,
        m.count,
        p.usage.rates_tracked ? m.rate : '',
        m.p50,
        m.p95,
        m.duration95,
        m.tps,
        m.samples,
        m.speedSamples,
        m.input,
        m.output,
        m.cache,
        m.saved,
        m.cost
      ])
    );
  const exportTrend = () =>
    csv(
      'trend',
      ['时间', '请求', '消费额度', '输入', '输出', '首响应P50ms', '首响应P95ms', '错误', '缓存命中率%'],
      p.usage.daily.map((d) => [d.date, d.count, d.cost, d.input, d.output, d.p50, d.p95, p.usage.errors_tracked ? d.errors : '', d.cache])
    );
  const modelSorted = [...p.usage.models].sort((a, b) =>
    a[p.sort.field] == null ? 1 : b[p.sort.field] == null ? -1 : (a[p.sort.field] - b[p.sort.field]) * p.sort.direction
  );
  const sort = (field) => p.setSort((s) => ({ field, direction: s.field === field ? -s.direction : -1 }));
  const dialog = (
    <Dialog
      open={Boolean(type)}
      onClose={close}
      fullWidth
      sx={{ '& .MuiButton-root': { whiteSpace: 'nowrap', flexShrink: 0 }, '& .MuiTableCell-root': { whiteSpace: 'nowrap' } }}
      maxWidth={type === 'models' ? 'lg' : 'sm'}
    >
      <DialogTitle>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          {headings[type]}
          <IconButton aria-label="关闭弹窗" onClick={close}>
            <IconX size={20} />
          </IconButton>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        {type === 'date' && (
          <Stack spacing={2.5}>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              {['24h', 7, 30, 90].map((period) => (
                <Button
                  key={period}
                  variant={p.period === period ? 'contained' : 'outlined'}
                  size="small"
                  onClick={() => {
                    p.setPeriod(period);
                    close();
                  }}
                >
                  {labels[period]}
                </Button>
              ))}
            </Stack>
            <TextField
              label="开始时间"
              type="datetime-local"
              value={form.start || ''}
              onChange={(e) => setForm((f) => ({ ...f, start: e.target.value }))}
              InputLabelProps={{ shrink: true }}
            />
            <TextField
              label="结束时间"
              type="datetime-local"
              value={form.end || ''}
              onChange={(e) => setForm((f) => ({ ...f, end: e.target.value }))}
              InputLabelProps={{ shrink: true }}
            />
            <Typography variant="caption" color="text.secondary">
              {p.options.zone === 'UTC' ? 'UTC' : 'UTC+8'} · 最多查询最近 180 天。24h 对比相邻 24h，其他周期对比上一周期同时段。
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
          </Stack>
        )}
        {type === 'filters' && (
          <Stack spacing={2.5}>
            {select('思考档位', form.effort || 'all', (v) => setForm((f) => ({ ...f, effort: v })), [
              ['all', '全部'],
              ...p.usage.filters.efforts.map((e) => [e, e === 'default' ? '未指定 / 默认' : e])
            ])}
            {select('统计时区', form.zone || 'Asia/Shanghai', (v) => setForm((f) => ({ ...f, zone: v })), [
              ['Asia/Shanghai', 'UTC+8 · 上海'],
              ['UTC', 'UTC']
            ])}
            <Typography variant="body2" color="text.secondary">
              不同输入、输出长度与思考档位会影响耗时。性能比较只描述当前个人样本，不代表模型能力排名。
            </Typography>
          </Stack>
        )}
        {type === 'chart' && (
          <Stack spacing={2}>
            {select('时间粒度', p.options.granularity, (v) => p.update('granularity', v), [
              ['auto', '自动'],
              ['hour', '小时'],
              ['day', '天'],
              ['week', '周']
            ])}
            <FormControlLabel
              control={<Checkbox checked={p.options.composition} onChange={(e) => p.update('composition', e.target.checked)} />}
              label="按模型拆分消费 / Tokens / 请求趋势"
            />
            <Typography variant="caption" color="text.secondary">
              点选图表的数据点查看对应请求。超过 168 个小时分桶时自动改为按天。
            </Typography>
            <Button disabled={p.loading || !!p.error} onClick={exportTrend} startIcon={<IconDownload size={16} />}>
              导出图表数据
            </Button>
          </Stack>
        )}
        {type === 'views' && (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              保留当前时间、模型、Key、请求类型和图表偏好，仅存储在本地浏览器。
            </Typography>
            <TextField
              label="视图名称"
              value={form.name || ''}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              inputProps={{ maxLength: 40 }}
            />
            <Button
              variant="contained"
              onClick={() => {
                const name = form.name.trim();
                if (!name) {
                  setError('请输入视图名称');
                  return;
                }
                if (p.saved.length >= 30) {
                  setError('最多保存 30 个视图');
                  return;
                }
                if (p.saved.some((v) => v.name === name)) {
                  setError('视图名称已存在');
                  return;
                }
                saveViews([...p.saved, { name, period: p.period, options: p.options, rank: p.rank, metric: p.metric }]);
                showSuccess('已保存视图');
                setError('');
              }}
            >
              保存当前视图
            </Button>
            {error && <Alert severity="error">{error}</Alert>}
            <Divider />
            {p.saved.map((v, i) => (
              <Stack direction="row" alignItems="center" key={i} spacing={1} flexWrap="wrap" useFlexGap>
                <Button
                  sx={{ mr: 'auto' }}
                  onClick={() => {
                    p.setPeriod(v.period);
                    p.setOptions((o) => ({ ...o, ...v.options }));
                    p.setRank(v.rank);
                    p.setMetric(v.metric || 'quota');
                    close();
                  }}
                >
                  {v.name}
                </Button>
                <Button
                  size="small"
                  onClick={() => {
                    const name = form.name?.trim();
                    if (!name) {
                      setError('请在上方填写新名称');
                      return;
                    }
                    if (p.saved.some((x, j) => j !== i && x.name === name)) {
                      setError('视图名称已存在');
                      return;
                    }
                    saveViews(p.saved.map((x, j) => (j === i ? { ...x, name } : x)));
                    setError('');
                  }}
                >
                  使用上方名称重命名
                </Button>
                <Button size="small" color="error" onClick={() => saveViews(p.saved.filter((_, j) => j !== i))}>
                  删除
                </Button>
              </Stack>
            ))}
            {!p.saved.length && (
              <Typography variant="caption" color="text.secondary">
                还没有保存的视图
              </Typography>
            )}
          </Stack>
        )}
        {type === 'export' && (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              CSV 保留当前时间、模型、Key 和请求类型筛选。聚合导出金额以额度计，逐次请求导出包含美元费用。
            </Typography>
            <Button variant="outlined" startIcon={<IconDownload />} disabled={p.loading || !!p.error} onClick={exportTrend}>
              趋势数据
            </Button>
            <Button variant="outlined" startIcon={<IconDownload />} disabled={p.loading || !!p.error} onClick={exportModels}>
              模型分析
            </Button>
            <Button
              disabled={p.exporting || p.loading || !!p.error}
              variant="outlined"
              startIcon={<IconDownload />}
              onClick={() => p.exportRequests()}
            >
              请求记录 · {p.usage.totals.count} 条
            </Button>
          </Stack>
        )}
        {type === 'models' && (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              点击列标题排序；点击模型查看请求。首响应与 TPS 分别统计有效计时和输出样本。两项分别统计样本数，少于 20 个样本仅供参考。
            </Typography>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>模型</TableCell>
                    {[
                      ['count', '请求'],
                      ['rate', '成功率'],
                      ['p50', '首响应 P50'],
                      ['p95', '首响应 P95'],
                      ['tps', '输出 t/s'],
                      ['duration95', '耗时 P95'],
                      ['cache', '缓存命中'],
                      ['cost', '消费']
                    ].map(([f, t]) => (
                      <TableCell align="right" key={f}>
                        <Button size="small" onClick={() => sort(f)}>
                          {t}
                          {p.sort.field === f ? (p.sort.direction < 0 ? ' ↓' : ' ↑') : ''}
                        </Button>
                      </TableCell>
                    ))}
                    <TableCell>下钻</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {modelSorted.map((m) => (
                    <TableRow hover key={m.id}>
                      <TableCell>
                        <Button size="small" onClick={() => p.openLogs({ model: m.id })}>
                          {m.model_name}
                        </Button>
                        <Typography variant="caption" color={m.samples < 20 ? 'warning.main' : 'text.secondary'} display="block">
                          首响应 {m.samples} 个样本{m.samples < 20 ? ' · 样本不足' : ''}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">{m.count}</TableCell>
                      <TableCell align="right">{p.usage.rates_tracked && m.rate != null ? m.rate.toFixed(1) + '%' : '—'}</TableCell>
                      <TableCell align="right">{latency(m.p50)}</TableCell>
                      <TableCell align="right">{latency(m.p95)}</TableCell>
                      <TableCell align="right">
                        {m.tps?.toFixed(2) || '—'}
                        <Typography variant="caption" color={m.speedSamples < 20 ? 'warning.main' : 'text.secondary'} display="block">
                          {m.speedSamples} 个 TPS 样本{m.speedSamples < 20 ? ' · 样本不足' : ''}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">{latency(m.duration95)}</TableCell>
                      <TableCell align="right">{m.cache.toFixed(1)}%</TableCell>
                      <TableCell align="right">{money(m.cost)}</TableCell>
                      <TableCell>
                        <Button size="small" disabled={m.p95 == null} onClick={() => p.openLogs({ model: m.id, minFirst: m.p95 })}>
                          慢请求
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
            <Button startIcon={<IconDownload size={16} />} disabled={p.loading || !!p.error} onClick={exportModels}>
              导出模型分析
            </Button>
          </Stack>
        )}
        {type === 'cache' && (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              按每条日志当时保存的价格和倍率，估算相对无缓存的净节省，已扣除缓存写入溢价；缺少计价快照、免费或按次计费请求不参与估算。
            </Typography>
            <Stack direction="row" justifyContent="space-between">
              <Typography>假设无缓存总费用</Typography>
              <Typography>{money(p.usage.totals.baseline, 4)}</Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography>实际费用</Typography>
              <Typography>{money(p.usage.totals.cost, 4)}</Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography>缓存净节省</Typography>
              <Typography color="success.main">{money(p.usage.totals.saved, 4)}</Typography>
            </Stack>
            <Typography variant="caption" color="text.secondary">
              计价覆盖 {p.usage.totals.cache_priced_requests || 0} / {p.usage.totals.cache_requests || 0}{' '}
              次缓存请求；覆盖不完整时仅汇总已知收益，不推算总费用。
            </Typography>
            <Divider />
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>模型</TableCell>
                  <TableCell align="right">缓存命中</TableCell>
                  <TableCell align="right">净节省</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {p.usage.models.map((m) => (
                  <TableRow hover key={m.id} onClick={() => p.openLogs({ model: m.id })} sx={{ cursor: 'pointer' }}>
                    <TableCell>{m.model_name}</TableCell>
                    <TableCell align="right">{m.cache.toFixed(1)}%</TableCell>
                    <TableCell align="right">{money(m.saved)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Stack>
        )}
        {type === 'changes' && (
          <Stack spacing={2}>
            {changes(p).map((item, i) => (
              <Paper variant="outlined" sx={{ p: 2 }} key={i}>
                <Typography fontWeight={650}>{item.title}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ my: 1 }}>
                  {item.detail}
                </Typography>
                <Button size="small" onClick={() => p.openLogs(item.filter)}>
                  查看相关请求
                </Button>
              </Paper>
            ))}
            {!changes(p).length && <Typography color="text.secondary">当前样本不足以判断变化。</Typography>}
            <Typography variant="caption" color="text.secondary">
              {p.options.model === 'all' &&
              p.options.key === 'all' &&
              p.options.effort === 'all' &&
              p.options.stream === 'all' &&
              p.usage.totals.cost > 0
                ? '按当前周期日均消费估算，余额可用约 ' +
                  Math.floor(p.balance / (p.usage.totals.cost / ((p.usage.range.end - p.usage.range.start) / DAY))) +
                  ' 天；实际取决于后续用量。'
                : '余额预测需要全部模型、Key 和请求类型的用量。'}
            </Typography>
          </Stack>
        )}
        {type === 'about' && (
          <Stack spacing={2}>
            <Alert severity="info">仅统计当前账户已记录并仍保留的日志，清理历史记录或关闭采集会影响完整性。</Alert>
            <Typography variant="body2">{responseTimeExplanation} P50/P95 和平均耗时仅使用成功请求的有效样本。</Typography>
            <Typography variant="body2">{outputSpeedExplanation} 仪表盘展示有效请求的 TPS 中位数，支持流式和非流式请求。</Typography>
            <Typography variant="body2">
              24h 对比前一个 24h；7/30/90D
              对比上个周期同时段，自定义对比相邻等长时段。当前小时或日期可能尚未结束。缓存收益是计价快照推算的估值，不是返还金额。
            </Typography>
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        {type === 'date' ? (
          <Button
            variant="contained"
            onClick={() => {
              const suffix = p.options.zone === 'UTC' ? 'Z' : '+08:00',
                start = Date.parse(form.start + suffix),
                end = Date.parse(form.end + suffix);
              if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
                setError('结束时间必须晚于开始时间');
                return;
              }
              if (start < Date.now() - 180 * DAY || end > Date.now()) {
                setError('请选择最近 180 天内的时间');
                return;
              }
              p.setOptions((o) => ({ ...o, start, end }));
              p.setPeriod('custom');
              close();
            }}
          >
            应用范围
          </Button>
        ) : type === 'filters' ? (
          <Button
            variant="contained"
            onClick={() => {
              p.setOptions((o) => ({ ...o, effort: form.effort, zone: form.zone }));
              close();
            }}
          >
            应用筛选
          </Button>
        ) : (
          <Button onClick={close}>关闭</Button>
        )}
      </DialogActions>
    </Dialog>
  );
  return (
    <>
      {dialog}
      <RequestDrawer analytics={p} />
    </>
  );
}

AnalyticsControls.propTypes = { analytics: PropTypes.object.isRequired };

Comparison.propTypes = { analytics: PropTypes.object.isRequired, field: PropTypes.string.isRequired };

TrendExtra.propTypes = { analytics: PropTypes.object.isRequired };

ModelActions.propTypes = { analytics: PropTypes.object.isRequired };

CacheBenefit.propTypes = { analytics: PropTypes.object.isRequired };

QualityAdditions.propTypes = { analytics: PropTypes.object.isRequired };

AnalyticsDialogs.propTypes = { analytics: PropTypes.object.isRequired };

ModelInline.propTypes = { analytics: PropTypes.object.isRequired, model: PropTypes.object.isRequired };
AnalyticsSection.propTypes = {
  title: PropTypes.string,
  subtitle: PropTypes.string,
  action: PropTypes.node,
  children: PropTypes.node,
  pad: PropTypes.bool
};
