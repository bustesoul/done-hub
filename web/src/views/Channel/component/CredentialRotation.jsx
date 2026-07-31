import PropTypes from 'prop-types';
import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Stack,
  TextField,
  Typography
} from '@mui/material';
import { Icon } from '@iconify/react';
import { API } from 'utils/api';
import { showError, showSuccess } from 'utils/common';

const statusColor = {
  active: 'success',
  pending: 'warning',
  failed: 'error',
  retired: 'default',
  revoked: 'default'
};

const CredentialRotation = ({ channelId, testModel, secret, onSecretChange }) => {
  const [credentials, setCredentials] = useState([]);
  const [working, setWorking] = useState('');

  const loadCredentials = useCallback(async () => {
    try {
      const response = await API.get(`/api/admin/provider-connections/${channelId}/credentials`);
      if (response.data?.success) {
        setCredentials(response.data.data || []);
      }
    } catch (error) {
      showError(error.response?.data?.message || error.message);
    }
  }, [channelId]);

  useEffect(() => {
    loadCredentials();
  }, [loadCredentials]);

  const candidate = credentials.find((item) => item.status === 'pending' || item.status === 'failed');
  const active = credentials.find((item) => item.status === 'active');

  const createCandidate = async () => {
    if (!secret?.trim()) {
      showError('请先输入新凭据');
      return;
    }
    setWorking('create');
    try {
      const response = await API.post(`/api/admin/provider-connections/${channelId}/credentials`, {
        secret
      });
      if (!response.data?.success) {
        throw new Error(response.data?.message || '创建候选凭据失败');
      }
      onSecretChange('');
      await loadCredentials();
      showSuccess('候选凭据已加密保存，当前流量仍使用原凭据');
    } catch (error) {
      showError(error.response?.data?.message || error.message);
    } finally {
      setWorking('');
    }
  };

  const testCandidate = async () => {
    if (!candidate) return;
    setWorking('test');
    try {
      const response = await API.post(
        `/api/admin/provider-connections/${channelId}/credentials/${candidate.id}/test`,
        null,
        { params: { model: testModel } }
      );
      if (!response.data?.success) {
        throw new Error(response.data?.message || '凭据测试失败');
      }
      await loadCredentials();
      showSuccess(`凭据测试通过（${response.data.data?.latency_ms ?? '-'} ms）`);
    } catch (error) {
      await loadCredentials();
      showError(error.response?.data?.error?.message || error.response?.data?.message || error.message);
    } finally {
      setWorking('');
    }
  };

  const activateCandidate = async () => {
    if (!candidate) return;
    setWorking('activate');
    try {
      const response = await API.post(
        `/api/admin/provider-connections/${channelId}/credentials/${candidate.id}/activate`
      );
      if (!response.data?.success) {
        throw new Error(response.data?.message || '激活凭据失败');
      }
      await loadCredentials();
      showSuccess('新凭据已原子激活，旧凭据已退休');
    } catch (error) {
      showError(error.response?.data?.message || error.message);
    } finally {
      setWorking('');
    }
  };

  const revokeCredential = async (credential) => {
    setWorking(`revoke-${credential.id}`);
    try {
      const response = await API.post(
        `/api/admin/provider-connections/${channelId}/credentials/${credential.id}/revoke`
      );
      if (!response.data?.success) {
        throw new Error(response.data?.message || '撤销凭据失败');
      }
      await loadCredentials();
      showSuccess(`凭据 v${credential.secret_version} 已撤销`);
    } catch (error) {
      showError(error.response?.data?.message || error.message);
    } finally {
      setWorking('');
    }
  };

  return (
    <Box sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
        <Box>
          <Typography variant="subtitle1">凭据与零中断轮换</Typography>
          <Typography variant="caption" color="text.secondary">
            凭据只写不回显。新凭据通过真实请求测试后才可切换，测试失败不会影响当前流量。
          </Typography>
        </Box>
        {active && <Chip size="small" color="success" label={`当前版本 v${active.secret_version}`} />}
      </Stack>

      <Divider sx={{ my: 1.5 }} />

      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
        <TextField
          fullWidth
          size="small"
          type="password"
          autoComplete="new-password"
          label="新 API Key / OAuth 凭据"
          value={secret || ''}
          onChange={(event) => onSecretChange(event.target.value)}
        />
        <Button
          variant="outlined"
          disabled={Boolean(working) || !secret?.trim()}
          onClick={createCandidate}
          startIcon={working === 'create' ? <CircularProgress size={16} /> : <Icon icon="mdi:key-plus" />}
          sx={{ whiteSpace: 'nowrap' }}
        >
          建立候选
        </Button>
      </Stack>

      {candidate && (
        <Alert severity={candidate.status === 'failed' ? 'error' : 'warning'} sx={{ mt: 1.5 }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} justifyContent="space-between" spacing={1}>
            <Box>
              候选版本 v{candidate.secret_version}
              {candidate.tested_at ? ' 已通过测试，可激活' : candidate.status === 'failed' ? ' 上次测试失败' : ' 尚未测试'}
            </Box>
            <Stack direction="row" spacing={1}>
              <Button size="small" disabled={Boolean(working)} onClick={testCandidate}>
                {working === 'test' ? <CircularProgress size={16} /> : '测试候选'}
              </Button>
              <Button
                size="small"
                variant="contained"
                disabled={Boolean(working) || !candidate.tested_at}
                onClick={activateCandidate}
              >
                {working === 'activate' ? <CircularProgress size={16} /> : '激活'}
              </Button>
            </Stack>
          </Stack>
        </Alert>
      )}

      {credentials.length > 0 && (
        <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap" sx={{ mt: 1.5 }}>
          {credentials.slice(0, 5).map((item) => (
            <Chip
              key={item.id}
              size="small"
              variant={item.status === 'active' ? 'filled' : 'outlined'}
              color={statusColor[item.status] || 'default'}
              label={`v${item.secret_version} · ${item.status}`}
              onDelete={
                item.status !== 'active' && item.status !== 'revoked' && !working
                  ? () => revokeCredential(item)
                  : undefined
              }
            />
          ))}
        </Stack>
      )}
    </Box>
  );
};

CredentialRotation.propTypes = {
  channelId: PropTypes.oneOfType([PropTypes.number, PropTypes.string]).isRequired,
  testModel: PropTypes.string,
  secret: PropTypes.string,
  onSecretChange: PropTypes.func.isRequired
};

export default CredentialRotation;
