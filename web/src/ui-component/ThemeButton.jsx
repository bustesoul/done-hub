import { useDispatch } from 'react-redux';
import { SET_THEME } from 'store/actions';
import { useTheme } from '@mui/material/styles';
import { Avatar, Box, ButtonBase, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import { Icon } from '@iconify/react';
import { useTranslation } from 'react-i18next';
import { useMemo, useState } from 'react';

export default function ThemeButton() {
  const dispatch = useDispatch();
  const { t } = useTranslation();

  const theme = useTheme();
  const [anchorEl, setAnchorEl] = useState(null);

  const themeOptions = useMemo(
    () => [
      { key: 'auto', icon: 'solar:monitor-bold-duotone', label: t('theme.auto') },
      { key: 'light', icon: 'solar:sun-2-bold-duotone', label: t('theme.light') },
      { key: 'dark', icon: 'solar:moon-bold-duotone', label: t('theme.dark') },
      { key: 'light-blue', icon: 'solar:palette-bold-duotone', label: t('theme.lightBlue') },
      { key: 'light-green', icon: 'solar:palette-bold-duotone', label: t('theme.lightGreen') },
      { key: 'light-red', icon: 'solar:palette-bold-duotone', label: t('theme.lightRed') }
    ],
    [t]
  );

  const getThemeMode = () => localStorage.getItem('theme') || 'auto';

  const activeTheme = themeOptions.find((option) => option.key === getThemeMode());

  const handleMenuOpen = (event) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleThemeSelect = (nextMode) => {
    if (nextMode === 'auto') {
      localStorage.removeItem('theme');
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      dispatch({ type: SET_THEME, theme: prefersDark ? 'dark' : 'light' });
      handleMenuClose();
      return;
    }

    localStorage.setItem('theme', nextMode);
    dispatch({ type: SET_THEME, theme: nextMode });
    handleMenuClose();
  };

  const activeIcon = activeTheme?.icon || 'solar:monitor-bold-duotone';
  const activeLabel = activeTheme?.label || t('theme.auto');

  return (
    <Box
      sx={{
        ml: 2,
        mr: 3,
        [theme.breakpoints.down('md')]: {
          mr: 2
        }
      }}
    >
      <Tooltip title={activeLabel} placement="bottom">
        <ButtonBase sx={{ borderRadius: '12px' }}>
          <Avatar
            variant="rounded"
            sx={{
              ...theme.typography.commonAvatar,
              ...theme.typography.mediumAvatar,
              ...theme.typography.menuButton,
              transition: 'all .2s ease-in-out',
              borderColor: 'transparent',
              backgroundColor: 'transparent',
              boxShadow: 'none',
              borderRadius: '50%',
              '&[aria-controls="menu-list-grow"],&:hover': {
                boxShadow: '0 0 10px rgba(0,0,0,0.2)',
                backgroundColor: 'transparent',
                borderRadius: '50%'
              }
            }}
            onClick={handleMenuOpen}
            color="inherit"
          >
            <Icon icon={activeIcon} width="1.5rem" />
          </Avatar>
        </ButtonBase>
      </Tooltip>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        {themeOptions.map((option) => (
          <MenuItem
            key={option.key}
            selected={option.key === getThemeMode() || (!localStorage.getItem('theme') && option.key === 'auto')}
            onClick={() => handleThemeSelect(option.key)}
          >
            <ListItemIcon>
              <Icon icon={option.icon} width="1.25rem" />
            </ListItemIcon>
            <ListItemText>{option.label}</ListItemText>
          </MenuItem>
        ))}
      </Menu>
    </Box>
  );
}
