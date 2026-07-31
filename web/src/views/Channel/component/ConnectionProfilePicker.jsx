import PropTypes from 'prop-types';
import { Alert, Box, ButtonBase, Chip, FormControl, InputLabel, MenuItem, Select, Stack, Typography } from '@mui/material';

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
        主流协议
      </Typography>
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
                <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                  {profile.variants.map((variant) => (
                    <Chip key={variant.channel_type} label={variant.display_name} size="small" variant="outlined" />
                  ))}
                </Stack>
              </Stack>
            </ButtonBase>
          );
        })}
      </Box>

      {selectedFeaturedVariant && selectedProfile?.variants.length > 1 && (
        <FormControl fullWidth size="small" sx={{ mt: 1.5 }}>
          <InputLabel id="connection-profile-variant-label">实现方式</InputLabel>
          <Select
            labelId="connection-profile-variant-label"
            label="实现方式"
            value={channelType}
            disabled={disabled}
            onChange={(event) => {
              const variant = selectedProfile.variants.find((item) => item.channel_type === event.target.value);
              onChange(selectedProfile.id, event.target.value, variant, selectedProfile);
            }}
          >
            {selectedProfile.variants.map((variant) => (
              <MenuItem key={variant.channel_type} value={variant.channel_type}>
                {variant.display_name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
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
