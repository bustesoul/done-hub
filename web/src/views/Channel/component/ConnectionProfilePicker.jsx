import PropTypes from 'prop-types';
import {
  Alert,
  Box,
  ButtonBase,
  Chip,
  FormControl,
  FormControlLabel,
  FormHelperText,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  Typography
} from '@mui/material';

const protocolToProfile = {
  openai_chat: 'openai-chat-completions',
  openai_responses: 'openai-responses',
  claude_messages: 'anthropic-messages',
  gemini: 'google-gemini'
};

const ConnectionProfilePicker = ({ profiles, providers, profileId, channelType, error, disabled, onChange }) => {
  if (error) {
    return (
      <Alert severity="error" sx={{ mb: 2 }}>
        {error}
      </Alert>
    );
  }

  const orderedProfiles = [...profiles].sort((left, right) => left.display_order - right.display_order);
  const selectedProfile = orderedProfiles.find((profile) => profile.id === profileId);
  const featuredChannelTypes = new Set(orderedProfiles.flatMap((profile) => profile.variants.map((variant) => variant.channel_type)));
  const selectedFeaturedVariant = selectedProfile?.variants.some((variant) => variant.channel_type === channelType);
  const specializedProviders = providers
    .filter((provider) => !featuredChannelTypes.has(provider.channel_type))
    .sort((left, right) => left.display_name.localeCompare(right.display_name));
  const specializedValue = selectedFeaturedVariant ? '' : channelType || '';
  const standardVariants = selectedProfile?.variants.filter((variant) => !variant.advanced) || [];
  const advancedVariant = selectedProfile?.variants.find((variant) => variant.advanced);
  const advancedEnabled = Boolean(advancedVariant && advancedVariant.channel_type === channelType);

  const selectProfile = (profile) => {
    const currentVariant = profile.variants.find((variant) => variant.channel_type === channelType);
    const variant = currentVariant || profile.variants[0];
    if (variant) {
      onChange(profile.id, variant.channel_type, variant, profile);
    }
  };

  const selectSpecializedProvider = (providerType) => {
    const provider = providers.find((item) => item.channel_type === providerType);
    const inferredProfile = provider?.protocols?.map((protocol) => protocolToProfile[protocol]).find(Boolean) || '';
    onChange(inferredProfile, providerType, null, null);
  };

  return (
    <Box sx={{ mb: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        上游 API 协议
      </Typography>
      <Alert severity="info" sx={{ mb: 1.5 }}>
        按上游真实提供的接口选择。Base URL 是否为 OpenAI 官方地址不影响这里的选择。
      </Alert>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, minmax(0, 1fr))' },
          gap: 1
        }}
      >
        {orderedProfiles.map((profile) => {
          const selected = profile.id === profileId && profile.variants.some((variant) => variant.channel_type === channelType);
          return (
            <ButtonBase
              key={profile.id}
              disabled={disabled}
              onClick={() => selectProfile(profile)}
              sx={{
                alignItems: 'stretch',
                border: 1,
                borderColor: selected ? 'primary.main' : 'divider',
                borderRadius: 2,
                justifyContent: 'flex-start',
                p: 1.5,
                textAlign: 'left',
                bgcolor: selected ? 'action.selected' : 'background.paper'
              }}
            >
              <Stack spacing={0.5}>
                <Typography variant="subtitle2">{profile.display_name}</Typography>
                <Typography variant="caption" color="text.secondary">
                  {profile.description}
                </Typography>
                <Box>
                  <Chip label={profile.probe?.request_path || profile.protocol} size="small" variant="outlined" />
                </Box>
              </Stack>
            </ButtonBase>
          );
        })}
      </Box>

      {selectedFeaturedVariant && standardVariants.length > 1 && (
        <FormControl fullWidth size="small" sx={{ mt: 1.5 }}>
          <InputLabel id="connection-profile-variant-label">上游平台</InputLabel>
          <Select
            labelId="connection-profile-variant-label"
            label="上游平台"
            value={channelType}
            disabled={disabled}
            onChange={(event) => {
              const variant = standardVariants.find((item) => item.channel_type === event.target.value);
              onChange(selectedProfile.id, event.target.value, variant, selectedProfile);
            }}
          >
            {standardVariants.map((variant) => (
              <MenuItem key={variant.channel_type} value={variant.channel_type}>
                {variant.display_name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      )}

      {selectedFeaturedVariant && advancedVariant && (
        <Box sx={{ mt: 1.5 }}>
          <FormControlLabel
            control={
              <Switch
                checked={advancedEnabled}
                disabled={disabled}
                onChange={(event) => {
                  const variant = event.target.checked ? advancedVariant : standardVariants[0];
                  onChange(selectedProfile.id, variant.channel_type, variant, selectedProfile);
                }}
              />
            }
            label="高级兼容模式"
          />
          <FormHelperText sx={{ ml: 0 }}>
            {advancedVariant.description || '仅用于非标准 OpenAI 兼容实现；普通兼容 Base URL 无需开启。'}
          </FormHelperText>
        </Box>
      )}

      <FormControl fullWidth size="small" sx={{ mt: 2 }}>
        <InputLabel id="specialized-provider-label">厂商专用与扩展</InputLabel>
        <Select
          labelId="specialized-provider-label"
          label="厂商专用与扩展"
          value={specializedValue}
          disabled={disabled}
          onChange={(event) => selectSpecializedProvider(event.target.value)}
        >
          {specializedProviders.map((provider) => (
            <MenuItem key={provider.channel_type} value={provider.channel_type}>
              <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                <Typography variant="body2">{provider.display_name}</Typography>
                {provider.capabilities?.length > 0 && (
                  <Typography variant="caption" color="text.secondary">
                    {provider.capabilities.join(' · ')}
                    {provider.auth_modes?.length > 0 ? ` ｜认证：${provider.auth_modes.join(' / ')}` : ''}
                  </Typography>
                )}
              </Box>
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </Box>
  );
};

ConnectionProfilePicker.propTypes = {
  profiles: PropTypes.array.isRequired,
  providers: PropTypes.array.isRequired,
  profileId: PropTypes.string,
  channelType: PropTypes.number,
  error: PropTypes.string,
  disabled: PropTypes.bool,
  onChange: PropTypes.func.isRequired
};

export default ConnectionProfilePicker;
