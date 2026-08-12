import { useEffect, useMemo, useState } from 'react';
import { API } from 'utils/api';
import { showError, copy } from 'utils/common';
import { Box, Card, Stack, alpha, Tooltip, IconButton, Typography } from '@mui/material';
import Label from 'ui-component/Label';
import { useTranslation } from 'react-i18next';
import { ExpandMore, ExpandLess } from '@mui/icons-material';
import { useSelector } from 'react-redux';
import IconWrapper from 'ui-component/IconWrapper';
import { groupAvailableModels } from 'utils/availableModels';

const SupportModels = () => {
  const [models, setModels] = useState({});
  const [expanded, setExpanded] = useState(true);
  const { t } = useTranslation();
  const ownedby = useSelector((state) => state.siteInfo?.ownedby);
  const modelGroups = useMemo(() => groupAvailableModels(models, ownedby, t('dashboard_index.unknown')), [models, ownedby, t]);

  const fetchModels = async () => {
    try {
      const res = await API.get(`/api/available_model`);
      const { data, success } = res.data;
      if (!success) return;

      setModels(data || {});
    } catch (error) {
      showError(error.message);
    }
  };

  useEffect(() => {
    fetchModels();
  }, []);

  const getIconByName = (name) => {
    const owner = (ownedby || []).find((item) => item.name === name);
    return owner?.icon;
  };

  return (
    <Card sx={{ alignSelf: 'start', minWidth: 0 }}>
      <Box sx={{ p: 2 }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
          <Typography variant="subtitle1" sx={{ color: 'text.secondary' }}>
            {t('dashboard_index.model_price')}
          </Typography>
          <Tooltip title={expanded ? t('dashboard_index.collapse_models') : t('dashboard_index.expand_models')}>
            <IconButton
              size="small"
              onClick={() => setExpanded(!expanded)}
              aria-label={expanded ? t('dashboard_index.collapse_models') : t('dashboard_index.expand_models')}
              sx={{ color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
            >
              {expanded ? <ExpandLess sx={{ width: 20 }} /> : <ExpandMore sx={{ width: 20 }} />}
            </IconButton>
          </Tooltip>
        </Stack>

        {expanded && (
          <Stack spacing={2} sx={{ mt: 2 }}>
            {modelGroups.map(({ provider, models: providerModels }) => (
              <Box key={provider}>
                <Typography
                  variant="subtitle2"
                  sx={{
                    color: 'text.secondary',
                    display: 'block',
                    mb: 1,
                    fontWeight: 'bold'
                  }}
                >
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <IconWrapper url={getIconByName(provider)} />
                    <span>{provider}</span>
                  </Stack>
                </Typography>
                <Box
                  sx={{
                    display: 'flex',
                    flexWrap: 'wrap',
                    gap: 1,
                    pl: 1
                  }}
                >
                  {providerModels.map((model) => (
                    <Label
                      key={model}
                      variant="soft"
                      color="primary"
                      onClick={() => copy(model, t('dashboard_index.model_name'))}
                      sx={{
                        cursor: 'pointer',
                        '&:hover': {
                          bgcolor: (theme) => alpha(theme.palette.primary.main, 0.16)
                        }
                      }}
                    >
                      {model}
                    </Label>
                  ))}
                </Box>
              </Box>
            ))}
          </Stack>
        )}

        {!expanded && (
          <Box
            sx={{
              display: 'flex',
              gap: 2.5,
              minWidth: 0,
              mt: 1.5,
              pb: 0.75,
              overflowX: 'auto',
              '&::-webkit-scrollbar': { height: 4 },
              '&::-webkit-scrollbar-thumb': { bgcolor: 'divider', borderRadius: 4 }
            }}
          >
            {modelGroups.map(({ provider, models: providerModels }) => (
              <Stack key={provider} direction="row" alignItems="center" spacing={1} flexShrink={0}>
                <Stack direction="row" alignItems="center" spacing={0.75}>
                  <IconWrapper url={getIconByName(provider)} />
                  <Typography variant="subtitle2" color="text.secondary" fontWeight="bold" whiteSpace="nowrap">
                    {provider}
                  </Typography>
                </Stack>
                {providerModels.map((model) => (
                  <Label
                    key={model}
                    variant="soft"
                    color="primary"
                    onClick={() => copy(model, t('dashboard_index.model_name'))}
                    sx={{
                      cursor: 'pointer',
                      whiteSpace: 'nowrap',
                      '&:hover': { bgcolor: (theme) => alpha(theme.palette.primary.main, 0.16) }
                    }}
                  >
                    {model}
                  </Label>
                ))}
              </Stack>
            ))}
          </Box>
        )}
      </Box>
    </Card>
  );
};

export default SupportModels;
