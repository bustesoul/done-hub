import { useEffect, useMemo, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import {
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  LinearProgress,
  Stack,
  TextField,
  Typography
} from '@mui/material';
import { API } from 'utils/api';
import { flattenAvailableModelGroups, groupAvailableModels } from 'utils/availableModels';

const CONCURRENCY = 4;
const TERMINAL_STATUSES = new Set(['success', 'failed']);

const statusColors = {
  pending: 'default',
  running: 'info',
  success: 'success',
  failed: 'error'
};

const latencyFromPayload = (payload) => {
  const latencyMs = Number(payload?.data?.latency_ms);
  if (Number.isFinite(latencyMs)) return latencyMs;
  const seconds = Number(payload?.time);
  return Number.isFinite(seconds) ? seconds * 1000 : null;
};

const errorMessage = (error) => error.response?.data?.error?.message || error.response?.data?.message || error.message;

const SelectedChannelTestDialog = ({ open, onClose, channels, onCompleted }) => {
  const { t } = useTranslation();
  const ownedBy = useSelector((state) => state.siteInfo?.ownedby);
  const [modelOptions, setModelOptions] = useState([]);
  const [selectedModel, setSelectedModel] = useState(null);
  const [loadingModels, setLoadingModels] = useState(false);
  const [running, setRunning] = useState(false);
  const [results, setResults] = useState([]);

  useEffect(() => {
    if (!open) return undefined;

    let active = true;
    setSelectedModel(null);
    setResults([]);
    setLoadingModels(true);

    API.get('/api/available_model')
      .then((response) => {
        if (!active) return;
        const { success, data } = response.data;
        if (!success) throw new Error(response.data.message);
        const groups = groupAvailableModels(data, ownedBy, t('dashboard_index.unknown'));
        setModelOptions(flattenAvailableModelGroups(groups));
      })
      .catch(() => {
        if (active) setModelOptions([]);
      })
      .finally(() => {
        if (active) setLoadingModels(false);
      });

    return () => {
      active = false;
    };
  }, [open, ownedBy, t]);

  const completedCount = useMemo(() => results.filter((result) => TERMINAL_STATUSES.has(result.status)).length, [results]);
  const successCount = useMemo(() => results.filter((result) => result.status === 'success').length, [results]);
  const failedCount = useMemo(() => results.filter((result) => result.status === 'failed').length, [results]);
  const progress = results.length ? (completedCount / results.length) * 100 : 0;

  const updateResult = (channelId, patch) => {
    setResults((current) => current.map((result) => (result.id === channelId ? { ...result, ...patch } : result)));
  };

  const testChannel = async (channel) => {
    updateResult(channel.id, { status: 'running', message: '' });
    try {
      const response = await API.post(`/api/admin/provider-connections/${channel.id}/probe`, null, {
        params: { model: selectedModel.id },
        skipErrorToast: true
      });
      const payload = response.data;
      if (!payload.success) throw new Error(payload.message || t('channel_index.selectedTestFailed'));
      updateResult(channel.id, {
        status: 'success',
        latencyMs: latencyFromPayload(payload),
        message: t('channel_index.selectedTestSucceeded')
      });
    } catch (error) {
      updateResult(channel.id, {
        status: 'failed',
        latencyMs: latencyFromPayload(error.response?.data),
        message: errorMessage(error)
      });
    }
  };

  const handleStart = async () => {
    if (!selectedModel || channels.length === 0 || running) return;

    setRunning(true);
    setResults(channels.map((channel) => ({ ...channel, status: 'pending', latencyMs: null, message: '' })));

    let nextIndex = 0;
    const worker = async () => {
      while (nextIndex < channels.length) {
        const channel = channels[nextIndex];
        nextIndex += 1;
        await testChannel(channel);
      }
    };

    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, channels.length) }, () => worker()));
    setRunning(false);
    onCompleted();
  };

  const handleClose = () => {
    if (!running) onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} fullWidth maxWidth="md" PaperProps={{ sx: { borderRadius: 3 } }}>
      <DialogTitle sx={{ pb: 1 }}>
        <Typography variant="h3">{t('channel_index.testSelectedChannels')}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {t('channel_index.selectedTestSummary', { count: channels.length, concurrency: CONCURRENCY })}
        </Typography>
      </DialogTitle>

      <DialogContent sx={{ pt: '12px !important' }}>
        <Autocomplete
          options={modelOptions}
          value={selectedModel}
          loading={loadingModels}
          disabled={running}
          groupBy={(option) => option.group}
          getOptionLabel={(option) => option.id}
          isOptionEqualToValue={(option, value) => option.id === value.id}
          onChange={(event, value) => setSelectedModel(value)}
          noOptionsText={t('channel_index.noAvailableModels')}
          renderInput={(params) => (
            <TextField
              {...params}
              label={t('channel_index.selectTestModel')}
              placeholder={t('channel_index.selectTestModelPlaceholder')}
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {loadingModels && <CircularProgress color="inherit" size={18} />}
                    {params.InputProps.endAdornment}
                  </>
                )
              }}
            />
          )}
        />

        {results.length > 0 ? (
          <Box sx={{ mt: 3 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1.5} sx={{ mb: 1.25 }}>
              <Typography variant="body2" color="text.secondary">
                {t('channel_index.selectedTestProgress', { completed: completedCount, total: results.length })}
              </Typography>
              <Stack direction="row" spacing={1}>
                <Chip size="small" color="success" variant="outlined" label={`${t('channel_index.statusSuccess')} ${successCount}`} />
                <Chip size="small" color="error" variant="outlined" label={`${t('channel_index.statusFailed')} ${failedCount}`} />
              </Stack>
            </Stack>
            <LinearProgress variant="determinate" value={progress} sx={{ height: 6, borderRadius: 4, mb: 2 }} />

            <Box sx={{ maxHeight: 390, overflowY: 'auto', borderTop: 1, borderColor: 'divider' }}>
              {results.map((result) => (
                <Box key={result.id}>
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    alignItems={{ xs: 'flex-start', sm: 'center' }}
                    justifyContent="space-between"
                    spacing={1.5}
                    sx={{ py: 1.5 }}
                  >
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Typography fontWeight={650} noWrap>
                        {result.name || `#${result.id}`}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        ID {result.id}
                      </Typography>
                      {result.message && (
                        <Typography
                          variant="body2"
                          color={result.status === 'failed' ? 'error.main' : 'text.secondary'}
                          sx={{ mt: 0.5, wordBreak: 'break-word' }}
                        >
                          {result.message}
                        </Typography>
                      )}
                    </Box>
                    <Stack direction="row" alignItems="center" spacing={1} flexShrink={0}>
                      {Number.isFinite(result.latencyMs) && (
                        <Typography variant="body2" color="text.secondary">
                          {Math.round(result.latencyMs)} ms
                        </Typography>
                      )}
                      <Chip
                        size="small"
                        color={statusColors[result.status]}
                        variant={result.status === 'pending' ? 'outlined' : 'filled'}
                        icon={result.status === 'running' ? <CircularProgress size={13} color="inherit" /> : undefined}
                        label={t(`channel_index.status${result.status.charAt(0).toUpperCase()}${result.status.slice(1)}`)}
                      />
                    </Stack>
                  </Stack>
                  <Divider />
                </Box>
              ))}
            </Box>
          </Box>
        ) : (
          <Box sx={{ py: 5, textAlign: 'center' }}>
            <Typography color="text.secondary">{t('channel_index.selectedTestPlaceholder')}</Typography>
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={handleClose} disabled={running} color="inherit">
          {t('common.close')}
        </Button>
        <Button variant="contained" onClick={handleStart} disabled={!selectedModel || loadingModels || running || channels.length === 0}>
          {running
            ? t('channel_index.testingSelectedChannels')
            : results.length
              ? t('channel_index.retestSelectedChannels')
              : t('channel_index.startSelectedTest')}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

SelectedChannelTestDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  channels: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.number.isRequired,
      name: PropTypes.string
    })
  ).isRequired,
  onCompleted: PropTypes.func.isRequired
};

export default SelectedChannelTestDialog;
