import { useCallback, useEffect, useRef, useState } from 'react';
import { Link as RouterLink, Navigate } from 'react-router-dom';
import { Icon } from '@iconify/react';
import { alpha } from '@mui/material/styles';
import {
  Alert,
  Box,
  Button,
  Card,
  Collapse,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Divider,
  Drawer,
  FormControlLabel,
  IconButton,
  InputAdornment,
  List,
  ListItemButton,
  ListItemText,
  MenuItem,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableRow,
  TextField,
  Tooltip,
  Typography
} from '@mui/material';
import { useSnackbar } from 'notistack';
import { renderQuota, useIsAdmin } from 'utils/common';
import { API } from 'utils/api';
import { groupAvailableModels } from 'utils/availableModels';
import AdminContainer from 'ui-component/AdminContainer';
import FilterCollapse from 'ui-component/FilterCollapse';
import Label from 'ui-component/Label';
import TableHead from 'ui-component/TableHead';
import { normalizeModel, normalizeHistory, eligible, missingReason, estimateExplanation } from './data';
const number = (n) => n.toLocaleString('en-US');
const seconds = (n) => (n === null ? '—' : (n / 1000).toFixed(2) + ' 秒');
const percent = (n) => (n === null ? '—' : n.toFixed(2) + '%');
const billingAmount = (s) =>
  (s.ambiguous && !s.consumeCount) || !(s.consumeCount + s.errorCount) || s.consumeLogDisabled ? '—' : renderQuota(s.quota, 4);
const costAmount = (s) => (!s.costSampleCount || s.consumeLogDisabled ? '—' : renderQuota(s.costQuota, 4));
const costHint = (s) =>
  s.ambiguous && !s.consumeCount
    ? '历史模型归属不明'
    : !s.consumeCount
      ? '暂无计费记录'
      : !s.costSampleCount
        ? '未记录有效成本'
        : s.costSampleCount < s.consumeCount
          ? `部分成本 · ${number(s.costSampleCount)}/${number(s.consumeCount)} 条`
          : `已记录成本 · ${number(s.costSampleCount)} 条`;
const icon = (name, size = 20) => <Icon icon={name} width={size} />;
const protocolName = {
  openai_chat: 'Chat Completions',
  claude_messages: 'Messages',
  openai_responses: 'Responses',
  gemini: 'Gemini 原生',
  native: '其他原生协议'
};
export default function ModelExplorer() {
  const isAdmin = useIsAdmin();
  const [data, setData] = useState(null),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(false);
  const active = useRef(true);
  const requestSequence = useRef(0);
  const reload = useCallback(async () => {
    const requestID = ++requestSequence.current;
    const current = () => active.current && requestID === requestSequence.current;
    setLoading(true);
    setError('');
    try {
      const [catalog, usage] = await Promise.all([API.get('/api/admin/models'), API.get('/api/admin/models/usage')]);
      if (!catalog.data.success || !usage.data.success) throw new Error(catalog.data.message || usage.data.message || '读取失败');
      if (current()) setData({ models: catalog.data.data.map(normalizeModel), usage: usage.data.data });
    } catch (e) {
      if (current()) setError(e.message || '读取模型失败');
    } finally {
      if (current()) setLoading(false);
    }
  }, []);
  useEffect(() => {
    active.current = true;
    if (isAdmin) reload();
    return () => {
      active.current = false;
    };
  }, [isAdmin, reload]);
  async function updateBinding(model, channel, enabled) {
    const response = await API.put('/api/admin/models/binding', { model, channel_id: channel, enabled });
    if (!response.data.success) throw new Error(response.data.message || '更新失败');
    const catalog = await API.get('/api/admin/models');
    if (!catalog.data.success) throw new Error(catalog.data.message || '刷新模型状态失败');
    if (active.current) setData((d) => ({ ...d, models: catalog.data.data.map(normalizeModel) }));
  }
  if (!isAdmin) return <Navigate to="/panel/404" replace />;
  if (!data)
    return (
      <AdminContainer>
        <Typography variant="h2" mb={3}>
          模型
        </Typography>
        {error ? (
          <Alert severity="error" action={<Button onClick={reload}>重试</Button>}>
            {error}
          </Alert>
        ) : (
          <Typography>正在读取模型和近24小时统计…</Typography>
        )}
      </AdminContainer>
    );
  if (!data.models.length)
    return (
      <AdminContainer>
        <Typography variant="h2" mb={3}>
          模型
        </Typography>
        <Alert severity="info">暂无模型绑定，请先在渠道页配置模型。</Alert>
        <Button onClick={reload}>刷新</Button>
      </AdminContainer>
    );
  return (
    <ModelExplorerContent
      models={data.models}
      usage={data.usage}
      loading={loading}
      error={error}
      reload={reload}
      updateBinding={updateBinding}
    />
  );
}

function ModelExplorerContent({ models, usage, loading, error, reload, updateBinding }) {
  const [selected, setSelected] = useState(models[0].id),
    [search, setSearch] = useState(''),
    [group, setGroup] = useState('all'),
    [protocol, setProtocol] = useState('all'),
    [rulesOpen, setRulesOpen] = useState(false),
    [attention, setAttention] = useState(false),
    [familyClosed, setFamilyClosed] = useState({}),
    [localOpen, setLocalOpen] = useState(true),
    [globalOpen, setGlobalOpen] = useState(false),
    [detail, setDetail] = useState(null),
    [pending, setPending] = useState(null),
    [saving, setSaving] = useState(false);
  const families = groupAvailableModels(Object.fromEntries(models.map((m) => [m.id, { owned_by: m.family }]))).map((g) => ({
    id: g.provider,
    name: g.provider,
    provider: ''
  }));
  const groups = [...new Set(models.flatMap((m) => m.routes.flatMap((r) => r.groups)))].sort();
  const { enqueueSnackbar: enqueue } = useSnackbar();
  const enqueueSnackbar = (message, options) =>
    enqueue(message, { ...options, anchorOrigin: { vertical: 'bottom', horizontal: 'right' }, autoHideDuration: 3000 });
  const model = models.find((m) => m.id === selected) || models[0],
    all = model.routes,
    scoped = all.filter((r) => (group === 'all' || r.groups.includes(group)) && (protocol === 'all' || r.protocols.includes(protocol))),
    active = scoped.filter((r) => r.global && r.enabled),
    ready = scoped.filter(eligible),
    paused = scoped.filter((r) => r.global && !r.enabled),
    globalOff = scoped.filter((r) => !r.global),
    first = ready.length ? Math.max(...ready.map((r) => r.priority)) : null,
    priorities = [...new Set(active.map((r) => r.priority))].sort((a, b) => b - a);
  const history = normalizeHistory(usage, model),
    stats = history.total;
  const summary = (m) => {
    const rs = m.routes,
      n = rs.filter(eligible).length;
    return { n, total: rs.length, warn: n < 2 || rs.some((r) => r.cool) };
  };
  function toggle(r) {
    if (!r.global || saving) return;
    if (r.enabled && eligible(r) && ready.length === 1) {
      setPending(r);
      return;
    }
    apply(r);
  }
  async function apply(r, enabled = !r.enabled) {
    setSaving(true);
    try {
      await updateBinding(model.id, r.id, enabled);
      enqueueSnackbar(`${r.name} 已${enabled ? '恢复当前模型绑定' : '在当前模型中停用'}`, { variant: 'success' });
    } catch (e) {
      enqueueSnackbar(e.message || '更新失败，请刷新后重试', { variant: 'error' });
    } finally {
      setSaving(false);
    }
  }
  const status = (r) =>
    !r.global
      ? { label: '渠道已关闭', color: 'default', sub: '总开关关闭' }
      : !r.enabled
        ? { label: '模型内停用', color: 'default', sub: '仅停用此模型' }
        : r.cool
          ? { label: '冷却中', color: 'warning', sub: '临时退出候选' }
          : { label: '可路由', color: 'success', sub: '符合路由条件' };
  const heads = [
    { id: 'channel', label: '渠道 / 协议', minWidth: 145 },
    { id: 'status', label: '状态', minWidth: 80 },
    { id: 'success', label: '成功率（估算）', minWidth: 125, tooltip: estimateExplanation },
    { id: 'performance', label: '首响应 / 速度', minWidth: 110 },
    {
      id: 'fees',
      label: '计费 / 渠道成本',
      minWidth: 125,
      tooltip: '近24小时此模型在该渠道的历史计费金额与已记录成本；成本按配置倍率计算，零值不代表免费。'
    },
    { id: 'weight', label: '权重 / 参考占比', minWidth: 100 },
    { id: 'enabled', label: '模型内启用', minWidth: 90 }
  ].map((h) => ({ ...h, disableSort: true }));
  function routeRow(r) {
    const obs = history.byChannel[r.id],
      reason = missingReason(obs),
      info = status(r),
      peers = ready.filter((c) => c.priority === r.priority),
      weight = peers.reduce((n, c) => n + c.weight, 0),
      share = eligible(r) && weight ? Math.round((r.weight / weight) * 100) + '%' : '—';
    return (
      <TableRow key={r.id} data-route={r.id} sx={!r.global ? { opacity: 0.6 } : undefined}>
        <TableCell>
          <Stack spacing={0.5} alignItems="center">
            <Button size="small" data-detail={r.id} sx={{ p: 0, minHeight: 24, fontSize: 12 }} onClick={() => setDetail(r.id)}>
              {r.name}
            </Button>
            <Typography variant="caption">
              #{r.number} · {r.protocols.map((p) => (p === 'openai_chat' ? 'Chat' : protocolName[p])).join(' / ')}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell>
          <Stack spacing={0.5} alignItems="center">
            <Label color={info.color}>{info.label}</Label>
            <Typography variant="caption">{info.sub}</Typography>
          </Stack>
        </TableCell>
        <TableCell data-metric="ratio">
          <Stack spacing={0.5} alignItems="center">
            <Tooltip title={reason || estimateExplanation}>
              <Typography variant="body1" color={obs.ratio === null ? 'text.secondary' : obs.ratio >= 99 ? 'success.main' : 'warning.main'}>
                {percent(obs.ratio)}
              </Typography>
            </Tooltip>
            {obs.ambiguous && !obs.consumeCount && !obs.errorCount ? (
              <Typography variant="caption">{reason}</Typography>
            ) : (
              <>
                <Typography variant="caption">
                  计费 {number(obs.consumeCount)} · 错误 {obs.errorLogDisabled ? '—' : number(obs.errorCount)}
                </Typography>
                {reason && <Typography variant="caption">{reason}</Typography>}
              </>
            )}
          </Stack>
        </TableCell>
        <TableCell data-metric="first-response">
          <Stack spacing={0.5} alignItems="center">
            <Label color={obs.firstResponseMs === null ? 'default' : obs.firstResponseMs <= 1000 ? 'success' : 'primary'}>
              {seconds(obs.firstResponseMs)}
            </Label>
            <Typography variant="caption">{obs.tps === null ? '速度 —' : '约 ' + obs.tps.toFixed(1) + ' tok/s'}</Typography>
            <Typography variant="caption">
              {obs.firstSampleCount
                ? '首响应样本 ' + number(obs.firstSampleCount)
                : obs.ambiguous
                  ? '归属不明'
                  : obs.consumeCount
                    ? '未记录首响应'
                    : '暂无有效样本'}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell data-metric="fees">
          <Stack spacing={0.5} alignItems="center">
            <Typography variant="body1" sx={{ fontVariantNumeric: 'tabular-nums' }}>
              {billingAmount(obs)}
            </Typography>
            <Typography variant="caption">成本 {costAmount(obs)}</Typography>
            <Typography variant="caption">{costHint(obs)}</Typography>
          </Stack>
        </TableCell>
        <TableCell>
          <Typography variant="body1">{r.weight}</Typography>
          <Typography variant="caption">{!r.global || !r.enabled ? '原优先级 ' + r.priority : '同层 ' + share}</Typography>
        </TableCell>
        <TableCell>
          <Tooltip title={!r.global ? '渠道总开关关闭，无法在此开启' : '只影响当前模型与此渠道的绑定，跨组别和协议生效'}>
            <Stack alignItems="center">
              <Switch
                size="small"
                checked={r.global && r.enabled}
                disabled={!r.global || saving || loading}
                onChange={() => toggle(r)}
                inputProps={{ 'aria-label': `${r.name} 在 ${model.name} 中启用`, 'data-toggle': r.id }}
              />
              <Typography variant="caption" color={r.global && r.enabled ? 'success.main' : 'text.secondary'}>
                {!r.global ? '已锁定' : r.enabled ? '启用' : '停用'}
              </Typography>
            </Stack>
          </Tooltip>
        </TableCell>
      </TableRow>
    );
  }
  function routeTable(rs) {
    return (
      <TableContainer>
        <Table size="small">
          <TableHead headLabel={heads} />
          <TableBody>{rs.map(routeRow)}</TableBody>
        </Table>
      </TableContainer>
    );
  }
  const d = detail ? all.find((r) => r.id === detail) : null;
  const detailStats = d ? history.byChannel[d.id] : null;
  return (
    <AdminContainer>
      <Stack direction="row" alignItems="center" justifyContent="space-between" mb={5} gap={2} flexWrap="wrap">
        <Stack spacing={1}>
          <Typography variant="h2">模型</Typography>
          <Typography variant="subtitle1" color="text.secondary">
            Models
          </Typography>
        </Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems={{ xs: 'flex-start', sm: 'center' }}>
          <Typography variant="caption">统计截至 {new Date(usage.asOf * 1000).toLocaleString()}</Typography>
          <Button component={RouterLink} to="/panel/channel">
            渠道页
          </Button>
          <Button variant="contained" disabled={loading || saving} onClick={reload}>
            {loading ? '刷新中…' : '刷新'}
          </Button>
        </Stack>
      </Stack>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          刷新失败：{error}。当前显示上次读取的数据。
        </Alert>
      )}
      {(!usage.consumeTracked || !usage.errorsTracked) && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          日志记录未完整开启，相关指标可能缺失；成功率暂不估算。
        </Alert>
      )}
      {usage.unattributed > 0 && (
        <Alert severity="info" sx={{ mb: 2 }}>
          近24小时有 {number(usage.unattributed)} 条日志无法唯一归入当前模型绑定，未计入以下金额与性能统计。
        </Alert>
      )}
      <Alert severity="info" sx={{ mb: 2 }}>
        <Typography variant="body2">
          从模型查看渠道：高优先级先尝试，同层按权重分流。模型内开关独立生效；渠道总开关关闭时，此处无法开启。
        </Typography>
      </Alert>
      <Card>
        <FilterCollapse>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ p: 3 }}>
            <TextField
              size="small"
              fullWidth
              label="模型名称"
              placeholder="搜索名称或模型 ID"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              inputProps={{ 'aria-label': '搜索模型' }}
              InputProps={{ startAdornment: <InputAdornment position="start">{icon('solar:box-bold-duotone')}</InputAdornment> }}
            />
            <TextField
              select
              size="small"
              label="路由用户组"
              value={group}
              onChange={(e) => setGroup(e.target.value)}
              sx={{ minWidth: 130 }}
              SelectProps={{ inputProps: { 'aria-label': '用户组' } }}
            >
              <MenuItem value="all">全部用户组</MenuItem>
              {groups.map((g) => (
                <MenuItem key={g} value={g}>
                  {g}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              select
              size="small"
              label="路由协议"
              value={protocol}
              onChange={(e) => setProtocol(e.target.value)}
              sx={{ minWidth: 185 }}
              SelectProps={{ inputProps: { 'aria-label': '请求协议' } }}
            >
              <MenuItem value="all">全部协议</MenuItem>
              {Object.entries(protocolName).map(([id, label]) => (
                <MenuItem key={id} value={id}>
                  {label}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              size="small"
              label="历史统计"
              value="近 24 小时"
              sx={{ minWidth: 145 }}
              InputProps={{ readOnly: true }}
              inputProps={{ 'aria-label': '历史统计时间' }}
            />
          </Stack>
        </FilterCollapse>
        <Divider />
        <Box sx={{ display: 'flex', flexDirection: { xs: 'column', lg: 'row' } }}>
          <Box sx={(t) => ({ width: { xs: '100%', lg: 238 }, flexShrink: 0, borderRight: { lg: `1px solid ${t.palette.divider}` } })}>
            <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 2, py: 1.5 }}>
              <Typography variant="subtitle1">模型家族</Typography>
              <Label>{models.length}</Label>
            </Stack>
            <Box sx={{ px: 2, pb: 1 }}>
              <FormControlLabel
                sx={{ m: 0 }}
                control={
                  <Switch
                    size="small"
                    checked={attention}
                    onChange={(e) => setAttention(e.target.checked)}
                    inputProps={{ 'aria-label': '仅看需要关注' }}
                  />
                }
                label={<Typography variant="body2">仅看需要关注</Typography>}
              />
            </Box>
            <Box sx={{ maxHeight: { xs: 260, lg: 760 }, overflow: 'auto' }}>
              {families.map((f) => {
                const items = models.filter(
                  (m) =>
                    m.family === f.id &&
                    (m.name + ' ' + m.id).toLowerCase().includes(search.toLowerCase()) &&
                    (!attention || summary(m).warn)
                );
                return items.length ? (
                  <Box key={f.id}>
                    <ListItemButton
                      data-family={f.id}
                      onClick={() => setFamilyClosed((s) => ({ ...s, [f.id]: !s[f.id] }))}
                      sx={{ py: 1, px: 2 }}
                    >
                      <Typography variant="subtitle2" sx={{ flex: 1 }}>
                        {f.name}
                      </Typography>
                      {icon(familyClosed[f.id] ? 'solar:alt-arrow-down-linear' : 'solar:alt-arrow-up-linear', 16)}
                    </ListItemButton>
                    <Collapse in={!familyClosed[f.id]}>
                      <List disablePadding sx={{ px: 1, pb: 1 }}>
                        {items.map((m) => {
                          const s = summary(m);
                          return (
                            <ListItemButton
                              data-model={m.id}
                              key={m.id}
                              selected={model.id === m.id}
                              onClick={() => setSelected(m.id)}
                              sx={{
                                borderRadius: 2,
                                mb: 0.5,
                                px: 1.5,
                                py: 1.25,
                                '&.Mui-selected': { bgcolor: (t) => alpha(t.palette.primary.main, 0.1), color: 'primary.main' }
                              }}
                            >
                              <ListItemText
                                primary={
                                  <Typography variant="subtitle1" color="inherit">
                                    {m.name}
                                  </Typography>
                                }
                                secondary={
                                  <Typography variant="caption">
                                    {s.n} / {s.total} 渠道可用
                                  </Typography>
                                }
                              />
                              {s.warn && icon('solar:danger-circle-linear', 16)}
                            </ListItemButton>
                          );
                        })}
                      </List>
                    </Collapse>
                  </Box>
                ) : null;
              })}
              {!models.some(
                (m) => (m.name + ' ' + m.id).toLowerCase().includes(search.toLowerCase()) && (!attention || summary(m).warn)
              ) && (
                <Typography sx={{ p: 3 }} color="text.secondary">
                  没有匹配的模型
                </Typography>
              )}
            </Box>
          </Box>
          <Box sx={(t) => ({ flex: 1, minWidth: 0, borderTop: { xs: `1px solid ${t.palette.divider}`, lg: 0 } })}>
            <Stack direction="row" justifyContent="space-between" gap={2} alignItems="center" sx={{ p: 3 }}>
              <Box>
                <Stack direction="row" alignItems="center" spacing={1.5} flexWrap="wrap">
                  <Typography variant="h3">{model.name}</Typography>
                  <Label color={ready.length === 0 ? 'error' : ready.length === 1 ? 'warning' : 'success'}>
                    {ready.length === 0 ? '无可用路由' : ready.length === 1 ? '单路可用' : '多路可用'}
                  </Label>
                </Stack>
                <Stack direction="row" spacing={1} alignItems="center" mt={1}>
                  <Typography variant="caption">{model.id}</Typography>
                  <Tooltip title="复制模型 ID">
                    <IconButton
                      size="small"
                      aria-label="复制模型 ID"
                      onClick={() =>
                        navigator.clipboard
                          .writeText(model.id)
                          .then(() => enqueueSnackbar('已复制模型 ID', { variant: 'success' }))
                          .catch(() => enqueueSnackbar('复制失败', { variant: 'error' }))
                      }
                    >
                      {icon('solar:copy-line-duotone', 16)}
                    </IconButton>
                  </Tooltip>
                </Stack>
              </Box>
              <Stack spacing={1} alignItems="flex-end">
                {model.context && (
                  <Label variant="outlined" color="primary">
                    {model.context} 上下文
                  </Label>
                )}
                <Typography variant="caption">{model.kind}</Typography>
              </Stack>
            </Stack>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2,1fr)', sm: 'repeat(3,1fr)' }, px: 3, pb: 2, gap: 2 }}>
              {[
                ['计费金额', billingAmount(stats), '近24小时 · 可归属计费合计'],
                ['渠道成本', costAmount(stats), costHint(stats)],
                ['可路由渠道', `${ready.length} / ${scoped.length}`, '当前路由筛选范围'],
                ['成功率（估算）', percent(stats.ratio), stats.incomplete ? '日志不完整，暂不估算' : '近 24 小时 · 日志口径'],
                ['平均首响应', seconds(stats.firstResponseMs), `有效样本 ${number(stats.firstSampleCount)}`],
                [
                  '计费 / 错误记录',
                  `${number(stats.consumeCount)} / ${stats.incomplete ? '—' : number(stats.errorCount)}`,
                  '近 24 小时 · 非去重请求数'
                ]
              ].map(([label, value, sub]) => (
                <Box key={label} data-summary={label}>
                  <Typography variant="caption">{label}</Typography>
                  <Typography variant="h3" sx={{ my: 1 }}>
                    {value}
                  </Typography>
                  <Typography variant="caption">{sub}</Typography>
                </Box>
              ))}
            </Box>
            <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1} sx={{ px: 3, pb: 2 }}>
              <Typography variant="caption">
                历史指标覆盖此模型当前全部绑定、全部用户组和协议；不随路由筛选变化。
                {stats.attributed < stats.bindings
                  ? ` 有 ${stats.bindings - stats.attributed} 条绑定存在历史归属缺失，仅计入可归属记录。`
                  : ''}
              </Typography>
              <Button size="small" data-action="metric-rules" onClick={() => setRulesOpen(true)} sx={{ flexShrink: 0 }}>
                统计口径
              </Button>
            </Stack>
            <Divider />
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ px: 3, py: 1.5 }}>
              <Typography variant="subtitle1">
                渠道路由{' '}
                <Typography component="span" variant="caption">
                  {' '}
                  / {scoped.length} 条绑定
                </Typography>
              </Typography>
              <Typography variant="caption">优先级、权重只读</Typography>
            </Stack>
            {ready.length < 2 && (
              <Alert severity="warning" sx={{ mx: 2, mb: 2 }}>
                {!scoped.length
                  ? '没有匹配当前用户组和协议的渠道。'
                  : ready.length === 0
                    ? '当前范围内没有可用渠道。'
                    : '当前只有一条可用渠道，暂无故障转移路径。'}
              </Alert>
            )}
            {active.length > 0 && (
              <TableContainer>
                <Table size="small">
                  <TableHead headLabel={heads} />
                  <TableBody>
                    {priorities.map((p) => [
                      <TableRow key={'tier' + p}>
                        <TableCell
                          colSpan={heads.length}
                          sx={{ textAlign: 'left', py: 1, px: 2, bgcolor: (t) => alpha(t.palette.primary.main, 0.045) }}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            <Label color="primary">优先级 {p}</Label>
                            <Typography variant="caption">
                              {p === first ? '当前首选层' : first === null || p > first ? '当前层暂不可用' : '后备层 · 更高层不可用时尝试'}
                            </Typography>
                          </Stack>
                        </TableCell>
                      </TableRow>,
                      ...active.filter((r) => r.priority === p).map(routeRow)
                    ])}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
            {paused.length > 0 && (
              <Box>
                <Button
                  fullWidth
                  onClick={() => setLocalOpen(!localOpen)}
                  data-collapse="local"
                  startIcon={icon(localOpen ? 'solar:alt-arrow-down-linear' : 'solar:alt-arrow-right-linear', 16)}
                  sx={{ justifyContent: 'flex-start', px: 2, py: 1.5, color: 'text.secondary', borderRadius: 0 }}
                >
                  模型内已停用（{paused.length}）
                  <Typography variant="caption" sx={{ ml: 'auto' }}>
                    可在这里恢复
                  </Typography>
                </Button>
                <Collapse in={localOpen}>{routeTable(paused)}</Collapse>
              </Box>
            )}
            {globalOff.length > 0 && (
              <Box>
                <Divider />
                <Button
                  fullWidth
                  onClick={() => setGlobalOpen(!globalOpen)}
                  data-collapse="global"
                  startIcon={icon(globalOpen ? 'solar:alt-arrow-down-linear' : 'solar:alt-arrow-right-linear', 16)}
                  sx={{ justifyContent: 'flex-start', px: 2, py: 1.5, color: 'text.secondary', borderRadius: 0 }}
                >
                  渠道已关闭（{globalOff.length}）
                  <Stack direction="row" gap={0.5} alignItems="center" sx={{ ml: 'auto' }}>
                    {icon('solar:lock-keyhole-linear', 14)}
                    <Typography variant="caption">总开关锁定</Typography>
                  </Stack>
                </Button>
                <Collapse in={globalOpen}>{routeTable(globalOff)}</Collapse>
              </Box>
            )}
            <Box sx={{ p: 2, bgcolor: (t) => t.headBackgroundColor }}>
              <Typography variant="caption" display="block">
                模型内开关跨组别和协议生效，不修改渠道总开关，也不影响其他模型。
              </Typography>
              <Typography variant="caption" display="block" mt={0.75}>
                权重占比仅作同层参考，不代表实际流量。通配模型按配置的绑定规则统计和启停。冷却状态来自当前实例。
              </Typography>
            </Box>
          </Box>
        </Box>
      </Card>
      <Dialog open={!!pending} onClose={() => setPending(null)} maxWidth="xs" fullWidth>
        <DialogTitle>停用最后一条可用渠道？</DialogTitle>
        <DialogContent>
          <Typography>
            停用 {pending?.name} 后，{model.name} 在当前筛选范围内将没有可用路由。该模型绑定在其他组别和协议下也会停用。
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPending(null)}>取消</Button>
          <Button
            variant="contained"
            color="error"
            onClick={() => {
              apply(pending);
              setPending(null);
            }}
          >
            仍然停用
          </Button>
        </DialogActions>
      </Dialog>
      <Drawer
        sx={{ zIndex: (t) => t.zIndex.modal }}
        anchor="right"
        open={!!d}
        onClose={() => setDetail(null)}
        PaperProps={{ sx: { width: { xs: '100%', sm: 460 }, borderRadius: 0 } }}
      >
        {d && (
          <Box sx={{ p: 3 }}>
            <Stack direction="row" alignItems="center" justifyContent="space-between">
              <Typography variant="h3">渠道详情</Typography>
              <IconButton aria-label="关闭渠道详情" onClick={() => setDetail(null)}>
                {icon('solar:close-circle-linear')}
              </IconButton>
            </Stack>
            <Typography variant="h4" mt={3}>
              {d.name}
            </Typography>
            <Typography variant="subtitle2" mt={1}>
              {model.name} / #{d.number}
            </Typography>
            <Box mt={2}>
              <Label color={status(d).color}>{status(d).label}</Label>
            </Box>
            <Divider sx={{ my: 3 }} />
            {[
              ['对外模型 ID', model.id],
              ['上游模型 ID', d.upstream],
              ['用户组', d.groups.join(' / ')],
              ['请求协议', d.protocols.map((p) => protocolName[p]).join(' / ')],
              ['渠道总开关', d.global ? '启用' : '关闭'],
              ['模型绑定偏好', d.enabled ? '启用' : '停用'],
              ['有效开关状态', d.global && d.enabled ? '开启' : '关闭']
            ].map(([k, v]) => (
              <Stack key={k} direction="row" justifyContent="space-between" gap={2} mb={2}>
                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 90 }}>
                  {k}
                </Typography>
                <Typography variant="body2" sx={{ textAlign: 'right', overflowWrap: 'anywhere' }}>
                  {v}
                </Typography>
              </Stack>
            ))}
            <Divider sx={{ my: 3 }} />
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Typography variant="subtitle1">近 24 小时观测</Typography>
              <Button size="small" onClick={() => setRulesOpen(true)}>
                统计口径
              </Button>
            </Stack>
            {missingReason(detailStats) && (
              <Alert severity={detailStats.ambiguous || detailStats.errorLogDisabled ? 'warning' : 'info'} sx={{ mt: 2 }}>
                {missingReason(detailStats)}
                {detailStats.ambiguous ? '，不将无法区分的旧模型日志归入当前模型。' : ''}
              </Alert>
            )}
            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2,1fr)', gap: 3, mt: 3 }}>
              {[
                ['计费金额', billingAmount(detailStats), '此模型 × 此渠道 · 历史计费合计'],
                ['渠道成本', costAmount(detailStats), costHint(detailStats)],
                [
                  '计费占比',
                  !detailStats.consumeCount || stats.quota <= 0 ? '—' : percent((detailStats.quota / stats.quota) * 100),
                  '占此模型已归属计费金额，包含停用渠道'
                ],
                [
                  '平均单次计费',
                  !detailStats.consumeCount ? '—' : renderQuota(detailStats.quota / detailStats.consumeCount, 6),
                  '计费金额 ÷ 计费记录数'
                ],
                ['成功率（估算）', percent(detailStats.ratio), estimateExplanation],
                [
                  '计费 / 错误记录',
                  detailStats.ambiguous && !detailStats.consumeCount && !detailStats.errorCount
                    ? '—'
                    : `${number(detailStats.consumeCount)} / ${detailStats.errorLogDisabled ? '—' : number(detailStats.errorCount)}`,
                  '非去重请求数'
                ],
                ['平均首响应', seconds(detailStats.firstResponseMs), `有效流式样本 ${number(detailStats.firstSampleCount)}`],
                [
                  '近似输出速度',
                  detailStats.tps === null ? '—' : detailStats.tps.toFixed(1) + ' tok/s',
                  `有效样本 ${number(detailStats.speedSampleCount)}`
                ],
                ['平均请求耗时', seconds(detailStats.latencyMs), '仅计费日志，含等待及可能的重试'],
                [
                  '输入 / 输出 Token',
                  !detailStats.consumeCount ? '—' : `${number(detailStats.promptTokens)} / ${number(detailStats.completionTokens)}`,
                  '来自计费日志'
                ]
              ].map(([label, value, help]) => (
                <Box key={label}>
                  <Typography variant="caption">{label}</Typography>
                  <Tooltip title={help}>
                    <Typography variant="h4" mt={1}>
                      {value}
                    </Typography>
                  </Tooltip>
                  <Typography variant="caption" display="block" mt={0.5}>
                    {label === '成功率（估算）' ? '计费 /（计费 + 错误）' : help}
                  </Typography>
                </Box>
              ))}
            </Box>
            <Divider sx={{ my: 3 }} />
            <Typography variant="subtitle1">最近终态错误</Typography>
            {detailStats.recentErrors.length ? (
              detailStats.recentErrors.map((e) => (
                <Stack key={e.id} direction="row" spacing={1} alignItems="center" mt={2}>
                  <Label color="error">{e.metadata.status_code}</Label>
                  <Box>
                    <Typography variant="body2">{e.metadata.error_message}</Typography>
                    <Typography variant="caption">
                      {Math.max(1, Math.round((history.asOf - e.created_at) / 60))} 分钟前 · 终态错误日志
                    </Typography>
                  </Box>
                </Stack>
              ))
            ) : (
              <Typography variant="body2" color="text.secondary" mt={2}>
                {detailStats.ambiguous
                  ? '历史模型归属不明'
                  : detailStats.errorLogDisabled
                    ? '错误日志未开启，无法判断'
                    : detailStats.consumeCount
                      ? '近 24 小时未记录终态错误'
                      : '暂无错误记录，不能据此判断成功'}
              </Typography>
            )}
            <Alert severity="info" sx={{ mt: 3 }}>
              仅显示已有终态错误字段，不推断重试过程和恢复事件。开关变化不会改写历史指标。
            </Alert>
          </Box>
        )}
      </Drawer>
      <Dialog open={rulesOpen} onClose={() => setRulesOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>统计口径 · 近 24 小时</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <Alert severity="warning">成功率为日志口径估算，不是精确的请求成功率。</Alert>
            <Typography variant="body1">{estimateExplanation}</Typography>
            <Typography variant="body1">
              费用：仅汇总近24小时计费日志的历史计费金额和渠道成本，按站点额度单位换算；不按当前价格重算。
              错误日志本身不计费，但失败后仍结算的计费记录会保留在费用中。
            </Typography>
            <Typography variant="body1">
              渠道成本由配置的成本倍率计算，不是上游账单。零成本记录无法区分未配置或真实零成本；
              没有正成本记录显示「—」，存在零值时标注「部分成本」，不据此推算利润。
            </Typography>
            <Typography variant="body1">
              首响应：仅取流式计费记录中有效的 first_response，表示首个数据帧到达，不保证已有可见文本。未采集不计为 0。
            </Typography>
            <Typography variant="body1">
              输出速度：有效样本的输出 Token 总数 ÷ 首响应后剩余时长总和。仅使用流式记录，且输出 Token 大于 0、总耗时大于首响应时间。
            </Typography>
            <Typography variant="body1">
              历史范围：模型 ×
              渠道，最近连续24小时；按当前绑定展示，包含现已停用渠道，跨全部用户组和协议。路由筛选仅改变候选列表，不重新解释历史日志。
            </Typography>
            <Typography variant="body1">
              没有调用记录显示「暂无调用数据」，缺少字段显示「—」。模型映射归属不明的记录不合并；已知错误日志未开启时不计算成功率。
            </Typography>
            <Typography variant="caption">
              统计受日志保留时间、日志开关和历史模型映射影响。旧日志仅在当前映射下能唯一归属时计入，无法还原过去的配置变更；新日志记录对外模型及路由模型。没有完整日志不能视为
              100% 成功。
            </Typography>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRulesOpen(false)}>知道了</Button>
        </DialogActions>
      </Dialog>
    </AdminContainer>
  );
}
