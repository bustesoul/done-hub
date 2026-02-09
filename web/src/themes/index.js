import { createTheme } from '@mui/material/styles';

// assets
import colors from 'assets/scss/_themes-vars.module.scss';

// project imports
import componentStyleOverrides from './compStyleOverride';
import themePalette from './palette';
import themeTypography from './typography';
import { varAlpha, createGradient } from './utils';

/**
 * Represent theme style and structure as per Material-UI
 * @param {JsonObject} customization customization parameter object
 */

export const theme = (customization) => {
  const baseColors = colors;
  const { mode, colorOverrides } = getThemeVariant(customization.theme);
  const color = { ...baseColors, ...colorOverrides };
  const options = mode === 'light' ? GetLightOption(color) : GetDarkOption(color);
  const customGradients = {
    primary: createGradient(color.primaryMain, color.primaryDark),
    secondary: createGradient(color.secondaryMain, color.secondaryDark)
  };
  const themeOption = {
    colors: color,
    gradients: customGradients,
    ...options,
    customization
  };

  const themeOptions = {
    direction: 'ltr',
    palette: themePalette(themeOption),
    mixins: {
      toolbar: {
        minHeight: '48px',
        padding: '8px 16px',
        '@media (min-width: 600px)': {
          minHeight: '48px'
        }
      }
    },
    shape: {
      borderRadius: themeOption?.customization?.borderRadius || 12
    },
    typography: themeTypography(themeOption),
    breakpoints: {
      values: {
        xs: 0,
        sm: 600,
        md: 960,
        lg: 1280,
        xl: 1920
      }
    },
    zIndex: {
      modal: 1300,
      snackbar: 1400,
      tooltip: 1500
    }
  };

  const themes = createTheme(themeOptions);
  themes.components = componentStyleOverrides(themeOption);

  return themes;
};

export default theme;

function GetDarkOption(color) {
  return {
    mode: 'dark',
    heading: color.darkTextTitle,
    paper: '#1A1D23',
    backgroundDefault: '#13151A',
    background: '#1E2128',
    darkTextPrimary: '#E0E4EC',
    darkTextSecondary: '#A9B2C3',
    textDark: '#F8F9FC',
    menuSelected: color.primary200,
    menuSelectedBack: varAlpha(color.primaryMain, 0.12),
    divider: 'rgba(255, 255, 255, 0.1)',
    borderColor: 'rgba(255, 255, 255, 0.12)',
    menuButton: '#292D36',
    menuButtonColor: color.primaryMain,
    menuChip: '#292D36',
    headBackgroundColor: '#25282F',
    headBackgroundColorHover: varAlpha('#25282F', 0.08),
    tableBorderBottom: 'rgba(255, 255, 255, 0.08)'
  };
}

function GetLightOption(color) {
  return {
    mode: 'light',
    heading: '#202939',
    paper: '#FFFFFF',
    backgroundDefault: '#F5F7FA',
    background: '#F5F7FA',
    darkTextPrimary: '#3E4555',
    darkTextSecondary: '#6C7A92',
    textDark: '#252F40',
    menuSelected: color.primaryMain,
    menuSelectedBack: varAlpha(color.primary200, 0.08),
    divider: '#E9EDF5',
    borderColor: '#E0E6ED',
    menuButton: varAlpha(color.primary200, 0.12),
    menuButtonColor: color.primaryMain,
    menuChip: '#EEF2F6',
    headBackgroundColor: '#F5F7FA',
    headBackgroundColorHover: varAlpha('#F5F7FA', 0.12),
    tableBorderBottom: '#E9EDF5'
  };
}

function getThemeVariant(themeKey) {
  switch (themeKey) {
    case 'light-blue':
      return {
        mode: 'light',
        colorOverrides: {
          primaryLight: '#E9F2FF',
          primaryMain: '#4B95F5',
          primaryDark: '#2C6FD6',
          primary200: '#9CC5FF',
          primary800: '#1F56B8'
        }
      };
    case 'light-green':
      return {
        mode: 'light',
        colorOverrides: {
          primaryLight: '#E9FBF3',
          primaryMain: '#3BBF8B',
          primaryDark: '#23996B',
          primary200: '#93E0C2',
          primary800: '#197453'
        }
      };
    case 'light-red':
      return {
        mode: 'light',
        colorOverrides: {
          primaryLight: '#FFEDEF',
          primaryMain: '#E36C6C',
          primaryDark: '#C24C4C',
          primary200: '#F3A6A6',
          primary800: '#9F3737'
        }
      };
    case 'dark':
      return {
        mode: 'dark',
        colorOverrides: {}
      };
    case 'light':
    default:
      return {
        mode: 'light',
        colorOverrides: {}
      };
  }
}
