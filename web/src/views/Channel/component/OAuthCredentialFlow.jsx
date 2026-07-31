import PropTypes from 'prop-types';
import { useEffect, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  Typography
} from '@mui/material';
import { Icon } from '@iconify/react';
import { API } from 'utils/api';
import { copy, showError, showSuccess } from 'utils/common';

const POLL_INTERVAL_MS = 2000;

const OAuthCredentialFlow = ({ definition, channelId, projectId, proxy, disabled, active, onCredential }) => {
  const oauth = definition?.oauth;
  const [working, setWorking] = useState(false);
  const [session, setSession] = useState(null);
  const [callbackValue, setCallbackValue] = useState('');
  const pollTimerRef = useRef(null);
  const expiryTimerRef = useRef(null);
  const handledRef = useRef(false);
  const popupRef = useRef(null);
  const sessionRef = useRef(null);

  const stopPolling = () => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (expiryTimerRef.current) {
      clearTimeout(expiryTimerRef.current);
      expiryTimerRef.current = null;
    }
  };

  const reset = () => {
    stopPolling();
    if (sessionRef.current?.session_id && oauth?.provider) {
      API.delete(`/api/admin/provider-connections/oauth-sessions/${oauth.provider}/${sessionRef.current.session_id}`).catch(() => {});
    }
    sessionRef.current = null;
    if (popupRef.current && !popupRef.current.closed) popupRef.current.close();
    popupRef.current = null;
    handledRef.current = false;
    setWorking(false);
    setSession(null);
    setCallbackValue('');
  };

  const finish = (data, message) => {
    if (handledRef.current) return;
    handledRef.current = true;
    stopPolling();
    if (sessionRef.current?.session_id && oauth?.provider) {
      API.delete(`/api/admin/provider-connections/oauth-sessions/${oauth.provider}/${sessionRef.current.session_id}`).catch(() => {});
    }
    sessionRef.current = null;
    setSession(null);
    if (data?.credentials) {
      onCredential(data.credentials, data);
      showSuccess(message || 'OAuth 授权成功，凭据已填充');
    } else {
      showError(message || 'OAuth 授权失败');
    }
    setWorking(false);
    if (popupRef.current && !popupRef.current.closed) popupRef.current.close();
    popupRef.current = null;
  };

  const poll = async (provider, sessionId) => {
    try {
      const response = await API.get(`/api/admin/provider-connections/oauth-sessions/${provider}/${sessionId}`);
      if (!response.data?.success) return;
      const data = response.data.data || {};
      if (data.status === 'success') finish(data, response.data.message);
      if (data.status === 'failed') finish(data, response.data.message);
    } catch (error) {
      // Transient polling errors are retried until the session expires or the dialog closes.
    }
  };

  const startPolling = (provider, sessionId) => {
    stopPolling();
    pollTimerRef.current = setInterval(() => poll(provider, sessionId), POLL_INTERVAL_MS);
  };

  const start = async () => {
    if (!oauth) return;
    try {
      reset();
      setWorking(true);
      const response = await API.post(`/api/admin/provider-connections/oauth-sessions/${oauth.provider}`, {
        channel_id: channelId || 0,
        project_id: projectId?.trim() || '',
        proxy: proxy?.trim() || ''
      });
      if (!response.data?.success) throw new Error(response.data?.message || '无法创建 OAuth 会话');

      const data = response.data.data || {};
      sessionRef.current = data;
      setSession(data);
      setWorking(false);
      if (data.auth_url) popupRef.current = window.open(data.auth_url, '_blank');
      if (oauth.flow === 'browser_callback' || oauth.flow === 'device_code') {
        startPolling(oauth.provider, data.session_id);
      }
      expiryTimerRef.current = setTimeout(
        () => {
          showError('OAuth 会话已过期，请重新授权');
          reset();
        },
        Math.max(1, data.expires_in || 600) * 1000
      );
    } catch (error) {
      setWorking(false);
      showError(`OAuth 授权失败：${error.message || error}`);
    }
  };

  const exchange = async () => {
    if (!callbackValue.trim()) {
      showError('请输入授权回调 URL 或授权码');
      return;
    }
    try {
      setWorking(true);
      const response = await API.post(`/api/admin/provider-connections/oauth-sessions/${oauth.provider}/exchange`, {
        session_id: session.session_id,
        callback_url: callbackValue.trim(),
        authorization_code: callbackValue.trim()
      });
      if (!response.data?.success) throw new Error(response.data?.message || '授权码交换失败');
      finish(response.data.data || {}, response.data.message);
    } catch (error) {
      setWorking(false);
      showError(`授权码交换失败：${error.message || error}`);
    }
  };

  useEffect(() => {
    const handleMessage = (event) => {
      if (
        event.origin === window.location.origin &&
        event.data?.type === 'provider_oauth_result' &&
        event.data?.provider === oauth?.provider
      ) {
        if (event.data.success) finish(event.data, 'OAuth 授权成功，凭据已填充');
        else finish({}, event.data.message || 'OAuth 授权失败');
      }
    };
    window.addEventListener('message', handleMessage);
    return () => {
      window.removeEventListener('message', handleMessage);
      stopPolling();
      if (popupRef.current && !popupRef.current.closed) popupRef.current.close();
      if (sessionRef.current?.session_id && oauth?.provider) {
        API.delete(`/api/admin/provider-connections/oauth-sessions/${oauth.provider}/${sessionRef.current.session_id}`).catch(() => {});
        sessionRef.current = null;
      }
    };
    // A provider change creates a fresh flow and cleanup closes the old session UI.
  }, [oauth?.provider]);

  useEffect(() => {
    if (!active) reset();
    // Closing the parent dialog must stop polling and expire the server session.
  }, [active]);

  if (!oauth) return null;

  const manual = oauth.flow === 'manual_callback';
  const device = oauth.flow === 'device_code';
  const browser = oauth.flow === 'browser_callback';
  const providerName = definition.display_name || oauth.provider;

  return (
    <Box sx={{ my: 2 }}>
      <Button
        variant="outlined"
        fullWidth
        disabled={disabled || working || Boolean(session)}
        onClick={start}
        startIcon={working ? null : <Icon icon="mdi:shield-key-outline" />}
      >
        {working ? '正在建立授权会话…' : `使用 ${providerName} OAuth 授权`}
      </Button>
      <Alert severity="info" sx={{ mt: 1 }}>
        {manual
          ? '授权后复制完整回调 URL，在统一授权窗口中完成凭据交换。'
          : device
            ? '按提示输入设备码，完成后系统会自动检测授权结果。'
            : projectId
              ? '授权完成后系统会自动接收并验证凭据。'
              : '未填写 Project ID 时将自动检测可用项目；授权完成后系统会自动验证凭据。'}
      </Alert>
      {browser && session && (
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ mt: 1 }}>
          <Button fullWidth variant="contained" onClick={() => window.open(session.auth_url, '_blank')}>
            重新打开授权页面
          </Button>
          <Button variant="outlined" onClick={() => copy(session.auth_url)}>
            复制链接
          </Button>
          <Button color="inherit" onClick={reset}>
            取消
          </Button>
        </Stack>
      )}

      <Dialog open={Boolean(session && (manual || device))} onClose={reset} maxWidth="sm" fullWidth>
        <DialogTitle>{providerName} OAuth 授权</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            {device ? (
              <>
                <Alert severity="info">打开验证页面并输入设备码。此窗口会自动轮询授权结果。</Alert>
                <Typography variant="h5" sx={{ fontFamily: 'monospace', letterSpacing: 2, textAlign: 'center' }}>
                  {session?.user_code}
                </Typography>
              </>
            ) : (
              <>
                <Alert severity="info">在授权页面完成登录后，复制浏览器地址栏中的完整回调 URL。</Alert>
                <TextField
                  fullWidth
                  multiline
                  minRows={3}
                  label="授权回调 URL 或授权码"
                  value={callbackValue}
                  onChange={(event) => setCallbackValue(event.target.value)}
                />
              </>
            )}
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
              <Button
                variant="contained"
                fullWidth
                onClick={() => window.open(device ? session?.verification_uri : session?.auth_url, '_blank')}
                startIcon={<Icon icon="mdi:open-in-new" />}
              >
                打开授权页面
              </Button>
              <Button
                variant="outlined"
                onClick={() => copy(device ? session?.user_code : session?.auth_url)}
                startIcon={<Icon icon="mdi:content-copy" />}
              >
                复制{device ? '设备码' : '链接'}
              </Button>
            </Stack>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={reset} disabled={working}>
            取消
          </Button>
          {manual && (
            <Button variant="contained" onClick={exchange} disabled={working || !callbackValue.trim()}>
              {working ? '提交中…' : '完成授权'}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Box>
  );
};

OAuthCredentialFlow.propTypes = {
  definition: PropTypes.object,
  channelId: PropTypes.number,
  projectId: PropTypes.string,
  proxy: PropTypes.string,
  disabled: PropTypes.bool,
  active: PropTypes.bool,
  onCredential: PropTypes.func.isRequired
};

export default OAuthCredentialFlow;
