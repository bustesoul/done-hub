import { useRef, useState } from 'react';
import PropTypes from 'prop-types';
import {
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  TableCell,
  TextField,
  Tooltip,
  Typography,
  Alert
} from '@mui/material';
import { Icon } from '@iconify/react';
import { API } from 'utils/api';
import Label from 'ui-component/Label';

// Manual probes are kept separately from the historical usage metrics.
export default function BindingActions({ route, model, result, onResult, onSave, disabled }) {
  const [editing, setEditing] = useState(false);
  const [priority, setPriority] = useState('');
  const [weight, setWeight] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const testing = useRef(false);
  const isWildcard = model.id.includes('*');
  const valid =
    priority.trim() !== '' &&
    weight.trim() !== '' &&
    Number.isSafeInteger(Number(priority)) &&
    Number.isSafeInteger(Number(weight)) &&
    Number(weight) >= 1 &&
    Number(weight) <= 1000000;
  async function test() {
    if (testing.current) return;
    testing.current = true;
    onResult({ testing: true });
    try {
      const { data } = await API.post(
        '/api/admin/models/probe',
        { model: model.id, channel_id: route.id },
        { timeout: 65000, validateStatus: () => true, skipErrorToast: true }
      );
      onResult({
        success: data.success,
        latency: data.data?.latency_ms ?? data.error?.latency_ms,
        message: data.message || (data.success ? '测试成功' : '测试失败'),
        at: Date.now()
      });
    } catch (e) {
      onResult({
        success: false,
        latency: e.response?.data?.error?.latency_ms,
        message: e.response?.data?.message || e.message || '测试失败',
        at: Date.now()
      });
    } finally {
      testing.current = false;
    }
  }
  async function save() {
    if (!valid || saving) return;
    setSaving(true);
    setError('');
    try {
      await onSave(model.id, route.id, Number(priority), Number(weight));
      setEditing(false);
    } catch (e) {
      setError(e.response?.data?.message || e.message || '保存失败');
    } finally {
      setSaving(false);
    }
  }
  return (
    <>
      <TableCell data-metric="manual-test">
        {result?.testing ? (
          <Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
            <CircularProgress size={14} />
            <Typography variant="caption">测试中…</Typography>
          </Stack>
        ) : result ? (
          <Tooltip title={`${result.message} · ${new Date(result.at).toLocaleString()} · 本页最近一次手动测试`}>
            <Stack alignItems="center" spacing={0.5}>
              <Label color={result.success ? 'success' : 'error'}>{result.success ? '成功' : '失败'}</Label>
              <Typography variant="caption">
                {result.latency != null ? `${(result.latency / 1000).toFixed(2)} 秒 · 总耗时` : '悬停查看原因'}
              </Typography>
            </Stack>
          </Tooltip>
        ) : (
          <Typography variant="caption" color="text.secondary">
            未测试
          </Typography>
        )}
      </TableCell>
      <TableCell>
        <Stack direction="row" spacing={0.5} justifyContent="center">
          <Tooltip title={isWildcard ? '通配绑定无法直接测试' : `测试 ${model.name} · ${route.name}`}>
            <span>
              <IconButton
                size="small"
                sx={{ color: 'info.main' }}
                aria-label={`测试 ${model.id} 渠道 ${route.id}`}
                disabled={isWildcard || result?.testing}
                onClick={test}
              >
                <Icon icon="mdi:speedometer" width={20} />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="编辑此模型的优先级和权重">
            <span>
              <IconButton
                size="small"
                aria-label={`编辑 ${model.id} 渠道 ${route.id} 调度`}
                disabled={disabled || saving}
                onClick={() => {
                  setPriority(String(route.priority));
                  setWeight(String(route.weight));
                  setError('');
                  setEditing(true);
                }}
              >
                <Icon icon="solar:pen-bold" width={18} />
              </IconButton>
            </span>
          </Tooltip>
        </Stack>
        <Dialog open={editing} onClose={() => !saving && setEditing(false)} maxWidth="xs" fullWidth>
          <DialogTitle>模型渠道调度</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ pt: 1 }}>
              <Typography variant="body2">
                {model.name} · {route.name}
              </Typography>
              <TextField
                label="优先级"
                type="number"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                inputProps={{ step: 1 }}
                helperText="数值越大越优先；同一优先级内按权重选择。"
              />
              <TextField
                label="权重"
                type="number"
                value={weight}
                onChange={(e) => setWeight(e.target.value)}
                inputProps={{ min: 1, max: 1000000, step: 1 }}
                helperText="1–1000000 的整数；仅影响当前模型。"
              />
              <Typography variant="caption">适用于此绑定的全部用户组和兼容协议。保存不会改变启停状态。</Typography>
              {error && <Alert severity="error">{error}</Alert>}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button disabled={saving} onClick={() => setEditing(false)}>
              取消
            </Button>
            <Button variant="contained" disabled={!valid || saving} onClick={save}>
              {saving ? '保存中…' : '保存'}
            </Button>
          </DialogActions>
        </Dialog>
      </TableCell>
    </>
  );
}

BindingActions.propTypes = {
  route: PropTypes.object.isRequired,
  model: PropTypes.object.isRequired,
  result: PropTypes.object,
  onResult: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired,
  disabled: PropTypes.bool
};
