import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Box,
  Button,
  ButtonGroup,
  Chip,
  CircularProgress,
  Divider,
  LinearProgress,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
  useMediaQuery
} from '@mui/material';
import { alpha, useTheme } from '@mui/material/styles';
import { IconActivity, IconBolt, IconClock, IconCoins, IconDatabase, IconRefresh } from '@tabler/icons-react';
import Chart from 'react-apexcharts';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';

import SupportModels from './component/SupportModels';
import QuickStartCard from './component/QuickStartCard';
import StatusPanel from './component/StatusPanel';
import { API } from 'utils/api';
import { calculateQuota, renderQuota, showError } from 'utils/common';
import { exactNumber, formatCompact, formatLatency, formatPercent } from './dashboardData';

const periods = [7, 30, 90];

const Dashboard = () => {
  const theme = useTheme();
  const compactLayout = useMediaQuery(theme.breakpoints.down('md'));
  const { i18n } = useTranslation();
  const zh = i18n.language?.toLowerCase().startsWith('zh');
  const siteInfo = useSelector((state) => state.siteInfo);
  const [period, setPeriod] = useState(30);
  const [metric, setMetric] = useState('quota');
  const [usage, setUsage] = useState(null);
  const [rate, setRate] = useState(null);
  const [loading, setLoading] = useState(true);
  const [lastUpdated, setLastUpdated] = useState(null);
  const [currentTab, setCurrentTab] = useState(0);

  const copy = zh
    ? {
        title: '用量工作台',
        subtitle: '看清花费、缓存效率与运行质量',
        dashboard: '用量',
        status: '服务状态',
        spend: '消费',
        requests: '成功请求',
        tokens: '总 Tokens',
        cache: '缓存命中率',
        latency: '平均延迟',
        trend: '用量趋势',
        quota: '消费',
        tokenMetric: 'Tokens',
        requestMetric: '请求',
        models: '模型排行',
        modelHint: '按消费排序，快速识别主要用量来源',
        cacheTitle: '缓存效率',
        cacheRead: '缓存读取',
        cacheWrite: '缓存写入',
        prompt: '输入 Tokens',
        saved: '命中输入占比',
        quality: '运行质量',
        errors: '错误率',
        liveRpm: '实时 RPM',
        tracked: '错误日志已启用',
        untracked: '错误日志未完整记录',
        details: '逐日明细',
        date: '日期',
        output: '输出',
        errorCount: '错误',
        noData: '当前周期暂无用量',
        refreshed: '刚刚更新',
        utility: '快速入口',
        refresh: '刷新',
        last: '更新于'
      }
    : {
        title: 'Usage workspace',
        subtitle: 'Understand spend, cache efficiency, and runtime quality',
        dashboard: 'Usage',
        status: 'Status',
        spend: 'Spend',
        requests: 'Successful requests',
        tokens: 'Total tokens',
        cache: 'Cache hit rate',
        latency: 'Avg latency',
        trend: 'Usage trend',
        quota: 'Spend',
        tokenMetric: 'Tokens',
        requestMetric: 'Requests',
        models: 'Model ranking',
        modelHint: 'Sorted by spend to reveal primary usage',
        cacheTitle: 'Cache efficiency',
        cacheRead: 'Cache read',
        cacheWrite: 'Cache write',
        prompt: 'Input tokens',
        saved: 'Input hit share',
        quality: 'Runtime quality',
        errors: 'Error rate',
        liveRpm: 'Live RPM',
        tracked: 'Error logging enabled',
        untracked: 'Errors are not fully tracked',
        details: 'Daily detail',
        date: 'Date',
        output: 'Output',
        errorCount: 'Errors',
        noData: 'No usage in this period',
        refreshed: 'Updated just now',
        utility: 'Quick access',
        refresh: 'Refresh',
        last: 'Updated'
      };

  const loadDashboard = useCallback(async () => {
    setLoading(true);
    try {
      const [usageResponse, rateResponse] = await Promise.all([
        API.get(`/api/user/dashboard/usage?days=${period}`),
        API.get('/api/user/dashboard/rate')
      ]);
      if (!usageResponse.data.success) {
        showError(usageResponse.data.message);
        return;
      }
      setUsage(usageResponse.data.data);
      if (rateResponse.data.success) setRate(rateResponse.data.data);
      setLastUpdated(new Date());
    } catch (error) {
      showError(error?.message || 'Dashboard request failed');
    } finally {
      setLoading(false);
    }
  }, [period]);

  useEffect(() => {
    loadDashboard();
  }, [loadDashboard]);

  const totals = useMemo(() => usage?.totals || {}, [usage]);
  const daily = useMemo(() => usage?.daily || [], [usage]);
  const models = useMemo(() => usage?.models || [], [usage]);
  const totalTokens = (totals.prompt_tokens || 0) + (totals.completion_tokens || 0);

  const trendChart = useMemo(() => createTrendChart(daily, metric, theme), [daily, metric, theme]);
  const kpis = [
    { label: copy.spend, value: renderQuota(totals.quota || 0, 2), exact: renderQuota(totals.quota || 0, 6), icon: IconCoins },
    { label: copy.requests, value: formatCompact(totals.requests || 0), exact: exactNumber(totals.requests || 0), icon: IconActivity },
    { label: copy.tokens, value: formatCompact(totalTokens, ' Tokens'), exact: `${exactNumber(totalTokens)} Tokens`, icon: IconBolt },
    {
      label: copy.cache,
      value: formatPercent(totals.cache_hit_rate),
      exact: `${exactNumber(totals.cached_read_tokens || 0)} cached tokens`,
      icon: IconDatabase
    },
    {
      label: copy.latency,
      value: formatLatency(totals.average_latency_ms),
      exact: `${exactNumber(totals.average_latency_ms || 0)} ms`,
      icon: IconClock
    }
  ];

  const usageContent = (
    <Stack spacing={2.5}>
      <Box
        sx={{
          display: 'flex',
          alignItems: compactLayout ? 'flex-start' : 'center',
          justifyContent: 'space-between',
          flexDirection: compactLayout ? 'column' : 'row',
          gap: 2
        }}
      >
        <Box>
          <Stack direction="row" spacing={1.25} alignItems="center">
            <Typography variant="h2" sx={{ fontWeight: 750, letterSpacing: '-0.035em' }}>
              {copy.title}
            </Typography>
            <Chip
              label={`${period}D`}
              size="small"
              sx={{ fontWeight: 700, bgcolor: alpha(theme.palette.primary.main, 0.1), color: 'primary.main' }}
            />
          </Stack>
          <Typography color="text.secondary" sx={{ mt: 0.5 }}>
            {copy.subtitle}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          {lastUpdated && (
            <Typography variant="caption" color="text.secondary">
              {copy.last} {lastUpdated.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </Typography>
          )}
          <ButtonGroup size="small" aria-label="dashboard period">
            {periods.map((value) => (
              <Button key={value} variant={period === value ? 'contained' : 'outlined'} onClick={() => setPeriod(value)}>
                {value}D
              </Button>
            ))}
          </ButtonGroup>
          <Tooltip title={copy.refresh}>
            <span>
              <Button aria-label={copy.refresh} variant="outlined" onClick={loadDashboard} disabled={loading} sx={{ minWidth: 38, px: 1 }}>
                <IconRefresh size={18} />
              </Button>
            </span>
          </Tooltip>
        </Stack>
      </Box>

      <Paper variant="outlined" sx={{ overflow: 'hidden', borderRadius: 3, bgcolor: alpha(theme.palette.background.paper, 0.78) }}>
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, 1fr)', md: 'repeat(5, 1fr)' } }}>
          {kpis.map((item, index) => {
            const Icon = item.icon;
            return (
              <Tooltip key={item.label} title={item.exact} arrow>
                <Box
                  sx={{
                    px: { xs: 2, lg: 2.5 },
                    py: 2.4,
                    borderRight: { md: index < kpis.length - 1 ? `1px solid ${theme.palette.divider}` : 0 },
                    borderBottom: { xs: index < 4 ? `1px solid ${theme.palette.divider}` : 0, md: 0 }
                  }}
                >
                  <Stack direction="row" alignItems="center" spacing={1} color="text.secondary">
                    <Icon size={17} />
                    <Typography variant="caption" fontWeight={650}>
                      {item.label}
                    </Typography>
                  </Stack>
                  <Typography sx={{ mt: 1.1, fontSize: { xs: '1.35rem', lg: '1.55rem' }, fontWeight: 760, letterSpacing: '-0.035em' }}>
                    {item.value}
                  </Typography>
                </Box>
              </Tooltip>
            );
          })}
        </Box>
      </Paper>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1.7fr) minmax(300px, .7fr)' }, gap: 2.5 }}>
        <Section
          title={copy.trend}
          action={
            <ButtonGroup size="small">
              {[
                ['quota', copy.quota],
                ['tokens', copy.tokenMetric],
                ['requests', copy.requestMetric]
              ].map(([value, label]) => (
                <Button key={value} variant={metric === value ? 'contained' : 'text'} onClick={() => setMetric(value)}>
                  {label}
                </Button>
              ))}
            </ButtonGroup>
          }
        >
          <Box sx={{ minHeight: 330, mt: 1 }}>
            {loading ? (
              <CenteredLoader />
            ) : daily.length ? (
              <Chart options={trendChart.options} series={trendChart.series} type="area" height={330} />
            ) : (
              <EmptyState text={copy.noData} />
            )}
          </Box>
        </Section>

        <Section title={copy.models} subtitle={copy.modelHint}>
          <Stack spacing={2.1} sx={{ mt: 2.2 }}>
            {models.slice(0, 6).map((model, index) => {
              const maximum = models[0]?.quota || 1;
              return (
                <Box key={model.model_name}>
                  <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                    <Stack direction="row" alignItems="center" spacing={1.2} minWidth={0}>
                      <Typography variant="caption" color="text.secondary" sx={{ width: 18 }}>
                        {String(index + 1).padStart(2, '0')}
                      </Typography>
                      <Typography fontWeight={650} noWrap>
                        {model.model_name}
                      </Typography>
                    </Stack>
                    <Tooltip title={renderQuota(model.quota, 6)}>
                      <Typography fontWeight={700}>{renderQuota(model.quota, 2)}</Typography>
                    </Tooltip>
                  </Stack>
                  <LinearProgress
                    variant="determinate"
                    value={Math.max(2, (model.quota * 100) / maximum)}
                    sx={{ height: 4, borderRadius: 3, mt: 1, bgcolor: alpha(theme.palette.primary.main, 0.08) }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    {formatCompact(model.requests)} {copy.requests.toLowerCase()} ·{' '}
                    {formatCompact((model.prompt_tokens || 0) + (model.completion_tokens || 0), ' Tokens')}
                  </Typography>
                </Box>
              );
            })}
            {!loading && !models.length && <EmptyState text={copy.noData} />}
          </Stack>
        </Section>
      </Box>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1.25fr .75fr' }, gap: 2.5 }}>
        <Section title={copy.cacheTitle}>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(3, 1fr)' }, gap: 2, mt: 2 }}>
            <Metric
              label={copy.cacheRead}
              value={formatCompact(totals.cached_read_tokens || 0, ' Tokens')}
              exact={exactNumber(totals.cached_read_tokens || 0)}
            />
            <Metric
              label={copy.cacheWrite}
              value={formatCompact(totals.cached_write_tokens || 0, ' Tokens')}
              exact={exactNumber(totals.cached_write_tokens || 0)}
            />
            <Metric
              label={copy.prompt}
              value={formatCompact(totals.prompt_tokens || 0, ' Tokens')}
              exact={exactNumber(totals.prompt_tokens || 0)}
            />
          </Box>
          <Stack direction="row" justifyContent="space-between" sx={{ mt: 3, mb: 1 }}>
            <Typography variant="body2" color="text.secondary">
              {copy.saved}
            </Typography>
            <Typography fontWeight={750} color="primary.main">
              {formatPercent(totals.cache_hit_rate)}
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={Math.min(100, totals.cache_hit_rate || 0)}
            sx={{ height: 8, borderRadius: 8, bgcolor: alpha(theme.palette.primary.main, 0.08) }}
          />
        </Section>

        <Section title={copy.quality}>
          <Stack spacing={2.2} sx={{ mt: 2 }}>
            <Stack direction="row" justifyContent="space-between" alignItems="end">
              <Box>
                <Typography variant="caption" color="text.secondary">
                  {copy.errors}
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5 }}>
                  {formatPercent(totals.error_rate)}
                </Typography>
              </Box>
              <Chip
                size="small"
                color={usage?.errors_tracked ? 'success' : 'warning'}
                variant="outlined"
                label={usage?.errors_tracked ? copy.tracked : copy.untracked}
              />
            </Stack>
            <Divider />
            <Stack direction="row" justifyContent="space-between" alignItems="end">
              <Box>
                <Typography variant="caption" color="text.secondary">
                  {copy.liveRpm}
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5 }}>
                  {formatCompact(rate?.rpm || 0)}
                </Typography>
              </Box>
              <IconActivity color={theme.palette.success.main} size={28} />
            </Stack>
          </Stack>
        </Section>
      </Box>

      <Section title={copy.details} contentPadding={false}>
        <TableContainer>
          <Table size="small" sx={{ minWidth: 900 }}>
            <TableHead>
              <TableRow>
                {[copy.date, copy.requests, copy.spend, copy.prompt, copy.output, copy.cacheRead, copy.latency, copy.errorCount].map(
                  (label, index) => (
                    <TableCell key={label} align={index ? 'right' : 'left'}>
                      {label}
                    </TableCell>
                  )
                )}
              </TableRow>
            </TableHead>
            <TableBody>
              {[...daily].reverse().map((row) => (
                <TableRow key={row.date} hover>
                  <TableCell>
                    <Typography fontWeight={650}>{row.date}</Typography>
                  </TableCell>
                  <TableCell align="right">{exactNumber(row.requests)}</TableCell>
                  <TableCell align="right">{renderQuota(row.quota, 3)}</TableCell>
                  <TableCell align="right">
                    <Tooltip title={exactNumber(row.prompt_tokens)}>
                      <span>{formatCompact(row.prompt_tokens)}</span>
                    </Tooltip>
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title={exactNumber(row.completion_tokens)}>
                      <span>{formatCompact(row.completion_tokens)}</span>
                    </Tooltip>
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title={exactNumber(row.cached_read_tokens)}>
                      <span>{formatCompact(row.cached_read_tokens)}</span>
                    </Tooltip>
                  </TableCell>
                  <TableCell align="right">{formatLatency(row.average_latency_ms)}</TableCell>
                  <TableCell align="right">
                    <Chip size="small" label={row.errors} color={row.errors ? 'warning' : 'default'} variant="outlined" />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Section>

      <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: '.12em' }}>
        {copy.utility}
      </Typography>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', lg: siteInfo.builtin_chat_enabled !== false ? '1.4fr .6fr' : '1fr' },
          gap: 2.5
        }}
      >
        <SupportModels />
        {siteInfo.builtin_chat_enabled !== false && <QuickStartCard />}
      </Box>
    </Stack>
  );

  return (
    <>
      {siteInfo.UptimeEnabled && (
        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <Button variant={currentTab === 0 ? 'contained' : 'text'} onClick={() => setCurrentTab(0)}>
            {copy.dashboard}
          </Button>
          <Button variant={currentTab === 1 ? 'contained' : 'text'} onClick={() => setCurrentTab(1)}>
            {copy.status}
          </Button>
        </Stack>
      )}
      {currentTab === 0 || !siteInfo.UptimeEnabled ? usageContent : <StatusPanel />}
    </>
  );
};

const Section = ({ title, subtitle, action, children, contentPadding = true }) => (
  <Paper variant="outlined" sx={{ borderRadius: 3, overflow: 'hidden', height: '100%' }}>
    <Stack
      direction="row"
      alignItems="flex-start"
      justifyContent="space-between"
      spacing={2}
      sx={{ px: 2.5, pt: 2.4, pb: contentPadding ? 0 : 2 }}
    >
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
    <Box sx={{ px: contentPadding ? 2.5 : 0, pb: contentPadding ? 2.5 : 0 }}>{children}</Box>
  </Paper>
);

const Metric = ({ label, value, exact }) => (
  <Tooltip title={exact}>
    <Box>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="h3" sx={{ mt: 0.7 }}>
        {value}
      </Typography>
    </Box>
  </Tooltip>
);
const CenteredLoader = () => (
  <Box sx={{ minHeight: 300, display: 'grid', placeItems: 'center' }}>
    <CircularProgress size={28} />
  </Box>
);
const EmptyState = ({ text }) => (
  <Box sx={{ minHeight: 180, display: 'grid', placeItems: 'center' }}>
    <Typography color="text.secondary">{text}</Typography>
  </Box>
);

Section.propTypes = {
  title: PropTypes.string.isRequired,
  subtitle: PropTypes.string,
  action: PropTypes.node,
  children: PropTypes.node,
  contentPadding: PropTypes.bool
};

Metric.propTypes = {
  label: PropTypes.string.isRequired,
  value: PropTypes.string.isRequired,
  exact: PropTypes.string.isRequired
};

EmptyState.propTypes = {
  text: PropTypes.string.isRequired
};

function createTrendChart(daily, metric, theme) {
  const categories = daily.map((row) => row.date.slice(5));
  let series;
  if (metric === 'tokens') {
    series = [
      { name: 'Input', data: daily.map((row) => row.prompt_tokens || 0) },
      { name: 'Output', data: daily.map((row) => row.completion_tokens || 0) },
      { name: 'Cache read', data: daily.map((row) => row.cached_read_tokens || 0) }
    ];
  } else {
    series = [
      {
        name: metric === 'quota' ? 'Spend' : 'Requests',
        data: daily.map((row) => (metric === 'quota' ? Number(calculateQuota(row.quota || 0, 4)) : row.requests || 0))
      }
    ];
  }
  return {
    series,
    options: {
      chart: {
        toolbar: { show: false },
        zoom: { enabled: false },
        fontFamily: 'Inter, Public Sans, sans-serif',
        background: 'transparent'
      },
      colors: ['#165DFF', '#7C3AED', '#14B8A6'],
      dataLabels: { enabled: false },
      stroke: { curve: 'smooth', width: metric === 'tokens' ? [2.5, 2, 2] : 2.5 },
      fill: { type: 'gradient', gradient: { shadeIntensity: 0, opacityFrom: 0.25, opacityTo: 0.02, stops: [0, 90, 100] } },
      grid: { borderColor: alpha(theme.palette.text.primary, 0.07), strokeDashArray: 4, padding: { left: 4, right: 10 } },
      xaxis: {
        categories,
        axisBorder: { show: false },
        axisTicks: { show: false },
        labels: { style: { colors: theme.palette.text.secondary } },
        tickAmount: Math.min(8, categories.length)
      },
      yaxis: { labels: { style: { colors: theme.palette.text.secondary }, formatter: (value) => formatCompact(value) } },
      legend: { show: metric === 'tokens', position: 'top', horizontalAlign: 'right', labels: { colors: theme.palette.text.secondary } },
      tooltip: { theme: theme.palette.mode, x: { show: true } }
    }
  };
}

export default Dashboard;
