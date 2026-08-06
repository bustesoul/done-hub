import PropTypes from 'prop-types';
// material-ui
import logoLight from 'assets/images/logo.svg';
import logoDark from 'assets/images/logo-white.svg';
import logoMarkLight from 'assets/images/logo-mark.svg';
import logoMarkDark from 'assets/images/logo-mark-white.svg';
import { useSelector } from 'react-redux';
import { useTheme } from '@mui/material/styles';

/**
 * if you want to use image instead of <svg> uncomment following.
 *
 * import logoDark from 'assets/images/logo-dark.svg';
 * import logo from 'assets/images/logo.svg';
 *
 */

// ==============================|| LOGO SVG ||============================== //

const Logo = ({ isMini = false }) => {
  const siteInfo = useSelector((state) => state.siteInfo);
  const theme = useTheme();
  const defaultLogo = isMini
    ? theme.palette.mode === 'light'
      ? logoMarkLight
      : logoMarkDark
    : theme.palette.mode === 'light'
      ? logoLight
      : logoDark;

  if (siteInfo.isLoading) {
    return null; // 数据加载未完成时不显示 logo
  }

  const logoToDisplay = siteInfo.logo ? siteInfo.logo : defaultLogo;

  return (
    <img
      src={logoToDisplay}
      alt={siteInfo.system_name}
      style={{
        height: isMini ? '34px' : '44px',
        width: isMini ? '30px' : '168px',
        objectFit: 'contain',
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)'
      }}
    />
  );
};

Logo.propTypes = {
  isMini: PropTypes.bool
};

export default Logo;
