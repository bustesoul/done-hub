import PropTypes from 'prop-types';
import {
  Box,
  Button,
  ButtonGroup,
  Chip,
  CircularProgress,
  Drawer,
  IconButton,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  TextField,
  Typography,
  Alert
} from '@mui/material';
import { IconArrowLeft, IconDownload, IconX } from '@tabler/icons-react';
import QuotaWithDetailContent from '../Log/component/QuotaWithDetailContent';
import { calculateTokenBilling } from '../Log/component/tokenBilling';
import { useDashboardRequests } from './useDashboardAnalytics';
import { requestParams, datetime, money, compact, latency, csv } from './analyticsData';
import { outputSpeed, outputSpeedDetail, responseDuration, firstResponse } from 'utils/outputSpeed';

function RequestStatus({ row }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color={row.type === 2 ? 'success' : 'warning'}
      label={row.type === 2 ? '成功' : row.status ? `${row.status} 失败` : '失败'}
    />
  );
}
function firstTime(row) {
  return firstResponse(row);
}
function RequestTable({ data, options, onSelect }) {
  return (
    <TableContainer sx={{ flex: 1 }}>
      <Table size="small" sx={{ minWidth: 630 }}>
        <TableHead>
          <TableRow>
            {['时间 / 日志 ID', '模型 / Key 名称', '状态', '首响应', '消费'].map((t) => (
              <TableCell key={t} sx={{ whiteSpace: 'nowrap' }}>
                {t}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {data.items.map((row) => (
            <TableRow
              hover
              key={row.id}
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onSelect(row);
              }}
              onClick={() => onSelect(row)}
              sx={{ cursor: 'pointer' }}
            >
              <TableCell sx={{ whiteSpace: 'nowrap' }}>
                {datetime(row.created_at * 1000, options)}
                <Typography variant="caption" display="block" color="text.secondary">
                  {row.id}
                </Typography>
              </TableCell>
              <TableCell>
                {row.model_name || '未记录模型'}
                <Typography variant="caption" display="block" color="text.secondary">
                  {row.token_name || '未记录 Key'}
                </Typography>
              </TableCell>
              <TableCell>
                <RequestStatus row={row} />
              </TableCell>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>{latency(firstTime(row))}</TableCell>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>{money(row.quota, 4)}</TableCell>
            </TableRow>
          ))}
          {!data.items.length && (
            <TableRow>
              <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                {data.loading ? <CircularProgress size={24} /> : data.error ? '请求加载失败' : '没有匹配的请求'}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

export function AnalyticsDetails({ analytics: p }) {
  const recent = p.detailTab === 'requests',
    days = [...p.usage.dayRows].reverse(),
    params = { ...requestParams(p), page: p.page, size: 7 };
  const data = useDashboardRequests(params, recent && !p.loading && !p.error);
  const count = recent ? data.total : days.length,
    page = Math.min(p.page, Math.max(0, Math.ceil(count / 7) - 1));
  return (
    <Box sx={{ border: '1px solid', borderColor: 'divider', bgcolor: 'background.paper', borderRadius: 3, overflow: 'hidden' }}>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        flexWrap="wrap"
        useFlexGap
        spacing={1}
        sx={{ px: 2.5, py: 2.4 }}
      >
        <Typography variant="h3" fontWeight={720}>
          {recent ? '最近请求' : '逐日明细'}
        </Typography>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <ButtonGroup size="small">
            <Button
              variant={!recent ? 'contained' : 'text'}
              onClick={() => {
                p.setDetailTab('daily');
                p.setPage(0);
              }}
            >
              逐日
            </Button>
            <Button
              variant={recent ? 'contained' : 'text'}
              onClick={() => {
                p.setDetailTab('requests');
                p.setPage(0);
              }}
            >
              请求
            </Button>
          </ButtonGroup>
          <Button size="small" disabled={p.loading || !!p.error} onClick={() => p.openLogs()}>
            全部请求
          </Button>
        </Stack>
      </Stack>
      {recent ? (
        <>
          {data.error && <Alert severity="error">{data.error}</Alert>}
          <RequestTable data={data} options={p.options} onSelect={(request) => p.setDrawer({ type: 'request', request })} />
        </>
      ) : (
        <TableContainer>
          <Table size="small" sx={{ minWidth: 900 }}>
            <TableHead>
              <TableRow>
                {['日期', '成功请求', '消费', '输入', '输出', '缓存读取', '平均延迟', '错误'].map((t, i) => (
                  <TableCell key={t} align={i ? 'right' : 'left'}>
                    {t}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {days.slice(page * 7, page * 7 + 7).map((d) => (
                <TableRow
                  key={d.date}
                  hover
                  tabIndex={0}
                  sx={{ cursor: 'pointer' }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') p.openLogs({ start: d.start, end: d.end });
                  }}
                  onClick={() => p.openLogs({ start: d.start, end: d.end })}
                >
                  <TableCell>{d.date}</TableCell>
                  <TableCell align="right">{d.success}</TableCell>
                  <TableCell align="right">{money(d.quota, 3)}</TableCell>
                  <TableCell align="right">{compact(d.input)}</TableCell>
                  <TableCell align="right">{compact(d.output)}</TableCell>
                  <TableCell align="right">{compact(d.read)}</TableCell>
                  <TableCell align="right">{d.duration_samples ? latency(d.average_latency_ms) : '—'}</TableCell>
                  <TableCell align="right">
                    <Chip size="small" variant="outlined" label={p.usage.errors_tracked ? d.errors : '—'} />
                  </TableCell>
                </TableRow>
              ))}
              {!days.length && (
                <TableRow>
                  <TableCell colSpan={8} align="center" sx={{ py: 4 }}>
                    {p.loading ? '加载中…' : p.error ? '无法获取记录' : '当前周期暂无用量'}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      <Stack direction="row" alignItems="center" justifyContent="space-between" flexWrap="wrap" useFlexGap sx={{ px: 2 }}>
        <Button
          size="small"
          sx={{ whiteSpace: 'nowrap', flexShrink: 0 }}
          disabled={p.loading || !!p.error || p.exporting}
          startIcon={<IconDownload size={15} />}
          onClick={() =>
            recent
              ? p.exportRequests()
              : csv(
                  'daily',
                  ['日期', '成功请求', '消费额度', '输入', '输出', '缓存读取', '平均耗时ms', '错误'],
                  days.map((d) => [
                    d.date,
                    d.success,
                    d.quota,
                    d.input,
                    d.output,
                    d.read,
                    d.duration_samples ? d.average_latency_ms : '',
                    p.usage.errors_tracked ? d.errors : ''
                  ])
                )
          }
        >
          导出
        </Button>
        <TablePagination
          component="div"
          count={count}
          page={page}
          rowsPerPage={7}
          rowsPerPageOptions={[7]}
          onPageChange={(_, v) => p.setPage(v)}
          labelDisplayedRows={({ from, to, count }) => `${from}–${to} / ${count}`}
        />
      </Stack>
    </Box>
  );
}

export function RequestDrawer({ analytics: p }) {
  if (!p.drawer) return null;
  return <OpenRequestDrawer analytics={p} />;
}
function OpenRequestDrawer({ analytics: p }) {
  const d = p.drawer,
    close = () => p.setDrawer(null),
    filter = { ...d.filter, status: d.status, search: d.search },
    params = { ...requestParams(p, filter), page: d.page || 0, size: 10 };
  const data = useDashboardRequests(params, d.type === 'logs'),
    r = d.request,
    first = r ? firstTime(r) : null;
  const speed = outputSpeed(r)?.tps;
  const update = (value) => p.setDrawer((v) => ({ ...v, ...value }));
  return (
    <Drawer
      anchor="right"
      open
      onClose={close}
      sx={{ zIndex: 1400 }}
      PaperProps={{
        sx: { width: { xs: '100%', md: 900 }, p: { xs: 2, md: 3 }, '& .MuiButton-root': { whiteSpace: 'nowrap', flexShrink: 0 } }
      }}
    >
      <Stack spacing={2} sx={{ minHeight: 0, height: '100%' }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between">
          <Stack direction="row" alignItems="center" spacing={1}>
            {d.back && (
              <IconButton aria-label="返回请求列表" onClick={() => p.setDrawer(d.back)}>
                <IconArrowLeft size={20} />
              </IconButton>
            )}
            <Typography variant="h3">{d.type === 'logs' ? '请求明细' : '请求详情'}</Typography>
          </Stack>
          <IconButton aria-label="关闭请求侧栏" onClick={close}>
            <IconX size={20} />
          </IconButton>
        </Stack>
        {d.type === 'logs' ? (
          <>
            <Typography variant="body2" color="text.secondary">
              {datetime(params.start * 1000, p.options)} — {datetime(params.end * 1000, p.options)} · {data.total} 条请求
              {filter.minFirst ? ' · 首响应 ≥ ' + latency(filter.minFirst) : ''}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              继承工作台筛选{params.model ? ' · ' + params.model : ''}
              {params.token ? ' · ' + params.token : ''}
            </Typography>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <TextField
                size="small"
                label="日志 ID"
                value={d.search || ''}
                onChange={(e) => update({ search: e.target.value, page: 0 })}
                sx={{ flex: '1 1 130px' }}
              />
              <TextField
                select
                size="small"
                label="状态"
                value={d.status || 'all'}
                onChange={(e) => update({ status: e.target.value, page: 0 })}
                sx={{ minWidth: 130 }}
              >
                {[
                  ['all', '全部'],
                  ['success', '成功'],
                  ['error', '失败'],
                  ['429', '429 限流'],
                  ['504', '504 超时']
                ].map(([v, l]) => (
                  <MenuItem key={v} value={v}>
                    {l}
                  </MenuItem>
                ))}
              </TextField>
              <Button
                startIcon={<IconDownload size={16} />}
                disabled={data.loading || !!data.error || p.exporting}
                onClick={() => p.exportRequests(filter)}
              >
                导出
              </Button>
            </Stack>
            {data.error && <Alert severity="error">{data.error}</Alert>}
            <RequestTable data={data} options={p.options} onSelect={(request) => p.setDrawer({ type: 'request', request, back: d })} />
            <TablePagination
              component="div"
              count={data.total}
              page={data.loading ? 0 : Math.min(d.page, Math.max(0, Math.ceil(data.total / 10) - 1))}
              rowsPerPage={10}
              rowsPerPageOptions={[10]}
              onPageChange={(_, page) => update({ page })}
              labelDisplayedRows={({ from, to, count }) => `${from}–${to} / ${count}`}
            />
          </>
        ) : (
          <Box sx={{ overflowY: 'auto', pb: 3 }}>
            <Typography variant="body2" color="text.secondary">
              日志 {r.id} · {datetime(r.created_at * 1000, p.options)} · {r.token_name || '未记录 Key'}
            </Typography>
            <Stack direction="row" justifyContent="space-between" sx={{ mt: 2 }}>
              <Typography fontWeight={650}>{r.model_name}</Typography>
              <RequestStatus row={r} />
            </Stack>
            <Stack direction="row" spacing={3} flexWrap="wrap" useFlexGap sx={{ py: 3 }}>
              {[
                ['首响应', latency(first)],
                ['输出 TPS', speed == null ? '—' : speed.toFixed(2) + ' t/s'],
                ['TPS 口径', outputSpeedDetail(r)],
                ['耗时', latency(responseDuration(r))],
                ['请求总耗时', r.request_time > 0 ? latency(r.request_time) : '—']
              ].map(([label, value]) => (
                <Box key={label}>
                  <Typography variant="caption" color="text.secondary">
                    {label}
                  </Typography>
                  <Typography variant="h3" sx={{ mt: 1 }}>
                    {value}
                  </Typography>
                </Box>
              ))}
            </Stack>
            <Typography variant="body2" color="text.secondary">
              {r.is_stream ? '流式' : '非流式'} · 思考档位 {r.metadata?.reasoning_effort || '默认 / 未记录'}
            </Typography>
            {r.type === 2 ? (
              <QuotaWithDetailContent item={r} tokenBilling={calculateTokenBilling(r)} userIsAdmin={false} />
            ) : (
              <Alert severity="warning" sx={{ my: 2 }}>
                <Typography variant="body2">失败请求 · {r.metadata?.error_type || '无错误类型'}</Typography>
                {r.metadata?.error_message && (
                  <Typography variant="body2" sx={{ mt: 1, overflowWrap: 'anywhere' }}>
                    {r.metadata.error_message}
                  </Typography>
                )}
                {r.metadata?.attempt_count != null && (
                  <Typography variant="caption" display="block">
                    尝试次数：{r.metadata.attempt_count}
                  </Typography>
                )}
                <Typography variant="caption">失败日志不计入成功请求性能统计。</Typography>
              </Alert>
            )}
            <Button
              variant="outlined"
              onClick={() => {
                p.update('model', r.model_name);
                close();
                p.setDialog(null);
              }}
            >
              工作台只看此模型
            </Button>
          </Box>
        )}
      </Stack>
    </Drawer>
  );
}

RequestStatus.propTypes = { row: PropTypes.object.isRequired };
RequestTable.propTypes = { data: PropTypes.object.isRequired, options: PropTypes.object.isRequired, onSelect: PropTypes.func.isRequired };
AnalyticsDetails.propTypes = { analytics: PropTypes.object.isRequired };
RequestDrawer.propTypes = { analytics: PropTypes.object.isRequired };
OpenRequestDrawer.propTypes = { analytics: PropTypes.object.isRequired };
