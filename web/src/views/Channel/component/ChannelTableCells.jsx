import PropTypes from 'prop-types';
import { useState } from 'react';
import { Box, CircularProgress, IconButton, InputAdornment, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { Icon } from '@iconify/react';
import { useTranslation } from 'react-i18next';
import { API } from 'utils/api';
import { showError } from 'utils/common';
import Label from 'ui-component/Label';

const CHANNEL_TYPE_CODEX = 59;
const CHANNEL_TYPE_CLAUDECODE = 58;
const SUBSCRIPTION_QUOTA_TYPES = [CHANNEL_TYPE_CODEX, CHANNEL_TYPE_CLAUDECODE];
const QUOTA_CACHE_PREFIX = 'sq_v1_';

const credentialStatusMeta = {
  active: { label: '已配置', color: 'success' },
  keyless: { label: '免凭据', color: 'info' },
  pending: { label: '待验证', color: 'warning' },
  failed: { label: '验证失败', color: 'error' },
  missing: { label: '未配置', color: 'error' },
  mixed: { label: '混合状态', color: 'warning' }
};

export const CredentialStatusCell = ({ item }) => {
  const meta = credentialStatusMeta[item.credential_status] || credentialStatusMeta.missing;
  const testedAt = item.credential_tested_at ? new Date(item.credential_tested_at).toLocaleString() : '';
  return (
    <Stack spacing={0.5} alignItems="center">
      <Label color={meta.color} variant="soft">
        {meta.label}
      </Label>
      {testedAt && (
        <Typography variant="caption" color="text.secondary" noWrap title={testedAt}>
          {testedAt}
        </Typography>
      )}
    </Stack>
  );
};

CredentialStatusCell.propTypes = {
  item: PropTypes.object.isRequired
};

function getQuotaCache(channelId) {
  try {
    const raw = localStorage.getItem(QUOTA_CACHE_PREFIX + channelId);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

function setQuotaCache(channelId, windows) {
  try {
    let resetAt = null;
    for (const w of windows) {
      if (w.reset_at && (resetAt === null || w.reset_at < resetAt)) {
        resetAt = w.reset_at;
      }
    }
    localStorage.setItem(QUOTA_CACHE_PREFIX + channelId, JSON.stringify({ windows, resetAt }));
  } catch {}
}

export function statusInfo(t, status) {
  switch (status) {
    case 1:
      return t('channel_index.enabled');
    case 2:
      return t('channel_row.manual');
    case 3:
      return t('channel_row.auto');
    default:
      return t('common.unknown');
  }
}

export function SubscriptionQuotaCell({ channelId, channelType }) {
  const { t } = useTranslation();
  const [windows, setWindows] = useState(() => getQuotaCache(channelId)?.windows ?? null);
  const [loading, setLoading] = useState(false);

  const supported = SUBSCRIPTION_QUOTA_TYPES.includes(channelType);

  // 纯手动刷新：仅在用户点击时调用查询接口，打开页面只恢复上次的缓存记录，
  // 避免在用户不知情时调用查询额度接口。
  const fetchQuota = async () => {
    if (loading) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/channel/subscription_quota/${channelId}`);
      const { success, message, windows: quotaWindows } = res.data;
      if (success) {
        const wins = Array.isArray(quotaWindows) ? quotaWindows : [];
        setWindows(wins);
        setQuotaCache(channelId, wins);
      } else {
        showError(message);
      }
    } catch (err) {
      showError(err?.message || t('common.unknown'));
    } finally {
      setLoading(false);
    }
  };

  if (!supported) {
    return (
      <Typography variant="caption" sx={{ color: 'text.disabled' }}>
        -
      </Typography>
    );
  }

  if (!windows) {
    return (
      <Tooltip title={t('channel_row.showSubscriptionQuota')} placement="top">
        <span>
          <IconButton size="small" onClick={fetchQuota} disabled={loading}>
            {loading ? <CircularProgress size={16} /> : <Icon icon="mdi:chart-bar" width={17} />}
          </IconButton>
        </span>
      </Tooltip>
    );
  }

  return (
    <Stack spacing={0.35} alignItems="stretch" sx={{ width: 132, mx: 'auto' }}>
      {windows.length === 0 && (
        <Typography variant="caption" sx={{ color: 'text.secondary', textAlign: 'center' }}>
          {t('channel_row.noSubscriptionQuota')}
        </Typography>
      )}
      {windows.map((window) => {
        const used = Math.max(0, Math.min(100, Number(window.used_percent) || 0));
        const remaining = Math.max(0, Math.min(100, Number(window.remaining_percent) || 0));
        const resetTitle = window.reset_at ? new Date(window.reset_at * 1000).toLocaleString() : '';
        return (
          <Tooltip key={`${window.label}-${window.reset_at || 0}`} title={resetTitle} placement="top">
            <Stack direction="row" spacing={0.5} alignItems="center" sx={{ minWidth: 0 }}>
              <Typography variant="caption" noWrap sx={{ width: 42, color: 'text.secondary', fontSize: 10 }}>
                {window.label}
              </Typography>
              <Box
                sx={{
                  width: 48,
                  height: 5,
                  bgcolor: 'action.hover',
                  borderRadius: 0.75,
                  overflow: 'hidden',
                  flexShrink: 0
                }}
              >
                <Box
                  sx={{
                    width: `${used}%`,
                    height: '100%',
                    bgcolor: used >= 90 ? 'error.main' : used >= 70 ? 'warning.main' : 'success.main'
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ width: 34, textAlign: 'right', fontSize: 10, color: 'text.primary' }}>
                {remaining.toFixed(0)}%
              </Typography>
            </Stack>
          </Tooltip>
        );
      })}
      <Tooltip title={t('channel_row.refreshSubscriptionQuota')} placement="top">
        <span>
          <IconButton size="small" onClick={fetchQuota} disabled={loading} sx={{ alignSelf: 'center', width: 20, height: 20 }}>
            {loading ? <CircularProgress size={13} /> : <Icon icon="mdi:refresh" width={14} />}
          </IconButton>
        </span>
      </Tooltip>
    </Stack>
  );
}

SubscriptionQuotaCell.propTypes = {
  channelId: PropTypes.number,
  channelType: PropTypes.number
};

export function renderBalance(type, balance) {
  // balance 可能为 null（渠道从未更新过余额），统一兜底为 0 并保留两位小数，避免 toFixed 抛错
  const value = Number(balance) || 0;
  switch (type) {
    case 28: // Deepseek
    case 45: // Deepseek
      return <>¥{value.toFixed(2)}</>;
    default:
      return <>${value.toFixed(2)}</>;
  }
}

// 行内数字编辑器：outlined + 浮动标签，普通渠道行与标签代表行共用同一控件保证样式一致。
// tip 非空时（标签代表行）整体包一层 Tooltip 说明「作用于整组」，并由 label 后缀「·组」标明范围；普通行字段名由表头说明，无需 tip。
export function GroupInlineEditor({ label, tip, value, min, step, onChange, onCommit, disabled }) {
  const field = (
    <Box sx={{ display: 'flex' }}>
      <TextField
        type="number"
        label={label}
        variant="outlined"
        size="small"
        value={value ?? ''}
        onChange={(e) => onChange(Number(e.target.value))}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            onCommit();
          }
        }}
        onBlur={onCommit}
        inputProps={{ min, step }}
        sx={{ width: '90px' }}
        InputProps={{
          endAdornment: (
            <InputAdornment position="end">
              <IconButton size="small" color="primary" disabled={disabled} onClick={onCommit}>
                <Icon icon="mdi:check" />
              </IconButton>
            </InputAdornment>
          )
        }}
      />
    </Box>
  );
  return tip ? (
    <Tooltip title={tip} placement="top" arrow>
      {field}
    </Tooltip>
  ) : (
    field
  );
}

GroupInlineEditor.propTypes = {
  label: PropTypes.string,
  tip: PropTypes.string,
  value: PropTypes.oneOfType([PropTypes.number, PropTypes.string]),
  min: PropTypes.string,
  step: PropTypes.string,
  onChange: PropTypes.func,
  onCommit: PropTypes.func,
  disabled: PropTypes.bool
};
