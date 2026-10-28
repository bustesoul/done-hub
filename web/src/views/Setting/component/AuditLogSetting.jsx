import { useEffect, useState } from 'react';
import PropTypes from 'prop-types';
import { Alert, Autocomplete, Button, FormControlLabel, Stack, Switch, TextField, Typography } from '@mui/material';
import SubCard from 'ui-component/cards/SubCard';
import { API } from 'utils/api';
import { showError, showSuccess } from 'utils/common';

function TargetSelector({ kind, label, ids, onChange, disabled }) {
  const [query, setQuery] = useState('');
  const [options, setOptions] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    const timer = setTimeout(async () => {
      setLoading(true);
      try {
        const { data } = await API.get('/api/option/audit/targets', { params: { kind, q: query } });
        if (active) {
          if (!data.success) throw new Error(data.message);
          setOptions(data.data);
          setError('');
        }
      } catch (err) {
        if (active) setError(err.message || '加载失败');
      } finally {
        if (active) setLoading(false);
      }
    }, 250);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [kind, query]);

  const selected = ids.map((id) => options.find((option) => option.id === id) || { id, name: '' });
  return (
    <Autocomplete
      multiple
      options={[...options, ...selected.filter((value) => !options.some((option) => option.id === value.id))]}
      value={selected}
      loading={loading}
      disabled={disabled}
      filterOptions={(items) => items}
      isOptionEqualToValue={(option, value) => option.id === value.id}
      getOptionLabel={(option) =>
        `#${option.id}${option.name ? ` ${option.name}` : ''}${option.user_id ? `（用户 #${option.user_id}）` : ''}`
      }
      onInputChange={(_, value, reason) => {
        if (reason === 'input' || reason === 'clear') setQuery(value);
      }}
      onChange={(_, values) => onChange(values.map((value) => value.id))}
      renderInput={(params) => (
        <TextField {...params} label={label} error={Boolean(error)} helperText={error || '输入名称或 ID 搜索；留空表示不限制这一项'} />
      )}
    />
  );
}

TargetSelector.propTypes = {
  kind: PropTypes.string.isRequired,
  label: PropTypes.string.isRequired,
  ids: PropTypes.arrayOf(PropTypes.number).isRequired,
  onChange: PropTypes.func.isRequired,
  disabled: PropTypes.bool
};

export default function AuditLogSetting() {
  const [config, setConfig] = useState({ channel_ids: [], token_ids: [], archive_enabled: false });
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    let active = true;
    API.get('/api/option/audit/status')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        if (active) {
          setConfig(data.data.config);
          setStatus(data.data);
        }
      })
      .catch((error) => {
        if (active) showError(error.message || '加载审计配置失败');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const refresh = async (reloadConfig = false) => {
    setLoading(true);
    try {
      const { data } = await API.get('/api/option/audit/status');
      if (!data.success) throw new Error(data.message);
      if (reloadConfig || !dirty) {
        setConfig(data.data.config);
        setDirty(false);
      }
      setStatus(data.data);
    } catch (error) {
      showError(error.message || '刷新审计状态失败');
    } finally {
      setLoading(false);
    }
  };

  const save = async (value) => {
    setSaving(true);
    try {
      const { data } = await API.put('/api/option/', { key: 'AuditLogConfig', value: JSON.stringify(value) });
      if (!data.success) throw new Error(data.message);
      setConfig(value);
      setDirty(false);
      showSuccess('审计配置已保存，对新请求生效，无需重启');
      await refresh(true);
    } catch (error) {
      showError(error.message || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SubCard title="输出审计">
      <Stack spacing={2}>
        <Alert severity="info">
          记录客户端原始请求和上游原始响应（HTTP / SSE）。默认关闭；渠道和 API Key 都选择时取交集，两项都清空即关闭。
          审计异常不会阻止请求，日志可能因资源上限或写入失败而不完整。
        </Alert>
        <TargetSelector
          kind="channels"
          label="渠道"
          ids={config.channel_ids}
          disabled={loading || saving || !status}
          onChange={(ids) => {
            setConfig((current) => ({ ...current, channel_ids: ids }));
            setDirty(true);
          }}
        />
        <TargetSelector
          kind="tokens"
          label="用户 API Key"
          ids={config.token_ids}
          disabled={loading || saving || !status}
          onChange={(ids) => {
            setConfig((current) => ({ ...current, token_ids: ids }));
            setDirty(true);
          }}
        />
        <FormControlLabel
          control={
            <Switch
              checked={Boolean(config.archive_enabled)}
              disabled={loading || saving || !status}
              onChange={(_, checked) => {
                setConfig((current) => ({ ...current, archive_enabled: checked }));
                setDirty(true);
              }}
            />
          }
          label="归档模式（由宿主机备份服务清理历史文件）"
        />
        {config.archive_enabled && (
          <Alert severity="warning">
            请先配置宿主机备份任务。归档文件达到 100 MiB 或自首条写入满 24 小时时封口，以先到者为准。Done Hub
            不自动删除，请监控待备份积压与磁盘余量。
          </Alert>
        )}
        {status && (
          <Typography variant="body2">
            当前实例：{status.enabled ? '已启用' : '已关闭'} · 丢弃事件 {status.dropped_events} · 写入失败 {status.write_errors} · 截断{' '}
            {status.truncated_bodies} · 归档模式{status.config.archive_enabled ? '已开启' : '已关闭'}
          </Typography>
        )}
        {(status?.last_error || status?.config_error) && <Alert severity="warning">{status.config_error || status.last_error}</Alert>}
        {dirty && <Typography variant="body2">有未保存的更改，刷新状态会保留当前选择。</Typography>}
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Button variant="contained" disabled={loading || saving || !status} onClick={() => save(config)}>
            保存审计配置
          </Button>
          <Button disabled={loading || saving || !status} onClick={() => save({ ...config, channel_ids: [], token_ids: [] })}>
            关闭审计
          </Button>
          <Button disabled={loading || saving} onClick={() => refresh()}>
            刷新状态
          </Button>
        </Stack>
      </Stack>
    </SubCard>
  );
}
