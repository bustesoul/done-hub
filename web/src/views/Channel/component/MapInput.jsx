import React, { useState, useEffect } from 'react';
import PropTypes from 'prop-types';

import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import { Box, List, Button, ListItem, TextField, IconButton, ListItemSecondaryAction, Checkbox, Tooltip } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import Editor from '@monaco-editor/react';

import { Icon } from '@iconify/react';
import { showError } from 'utils/common';
import { useTranslation } from 'react-i18next';

const MapInput = ({ mapValue, onChange, disabled, error, label, enableSkip = false }) => {
  const { t } = useTranslation();
  const theme = useTheme();
  const [mappings, setMappings] = useState([]);

  useEffect(() => {
    try {
      const list = (mapValue || [{ index: 0, key: '', value: '' }]).map((item) => ({
        ...item,
        skip: item.skip === true
      }));
      setMappings(list);
    } catch (e) {
      setMappings([{ index: 0, key: '', value: '', skip: false }]);
    }
  }, [mapValue]);

  const [openJsonDialog, setOpenJsonDialog] = useState(false);
  const [jsonInput, setJsonInput] = useState('');

  const handleAdd = () => {
    const newIndex = mappings.length > 0 ? Math.max(...mappings.map((m) => m.index)) + 1 : 0;
    setMappings([...mappings, { index: newIndex, key: '', value: '', skip: false }]);
  };

  const handleDelete = (index) => {
    const newMappings = mappings.filter((mapping) => mapping.index !== index);
    setMappings(newMappings);
    updateParent(newMappings);
  };

  const handleChange = (index, field, newValue) => {
    const newMappings = mappings.map((mapping) => (mapping.index === index ? { ...mapping, [field]: newValue } : mapping));

    setMappings(newMappings);
    updateParent(newMappings);
  };

  const updateParent = (newMappings) => {
    onChange(newMappings);
  };

  const handleAddByJson = () => {
    // 将当前映射转换为 JSON 字符串
    // skip=true 的条目输出为对象 {"value": x, "skip": true}，否则输出纯字符串 x
    const currentMappingsObject = mappings.reduce((acc, { key, value, skip }) => {
      if (key) {
        acc[key] = enableSkip && skip ? { value, skip: true } : value;
      }
      return acc;
    }, {});
    const currentMappingsJson = JSON.stringify(currentMappingsObject, null, 2);
    setJsonInput(currentMappingsJson);
    setOpenJsonDialog(true);
  };

  const handleCloseJsonDialog = () => {
    setOpenJsonDialog(false);
    setJsonInput('');
  };

  const handleJsonSubmit = () => {
    try {
      const parsedJson = JSON.parse(jsonInput);
      // value 支持三种形态：
      //  - 字符串 "v"            -> {value:"v", skip:false}
      //  - {value, skip}         -> 直接采用
      //  - {value}（无 skip）    -> skip:false
      const newMappings = Object.entries(parsedJson).map(([key, raw], index) => {
        const isObj = raw !== null && typeof raw === 'object';
        const rawValue = isObj ? raw.value : raw;
        const skip = isObj ? raw.skip === true : false;
        // 强制将 value 转为字符串，避免 number/bool 等被后端静默丢弃
        let value;
        if (typeof rawValue === 'string') {
          value = rawValue;
        } else if (typeof rawValue === 'number' || typeof rawValue === 'boolean') {
          value = String(rawValue);
        } else {
          // null / undefined / 对象 / 数组等不支持的类型
          throw new Error(`Invalid value type for key "${key}"`);
        }
        return { index, key, value, skip };
      });
      setMappings(newMappings);
      updateParent(newMappings);
      handleCloseJsonDialog();
    } catch (e) {
      showError(t('common.jsonFormatError'));
    }
  };

  return (
    <Box>
      <List>
        {mappings.map(({ index, key, value, skip }) => (
          <ListItem key={index}>
            <TextField
              label={label.keyName}
              value={key}
              onChange={(e) => handleChange(index, 'key', e.target.value)}
              disabled={disabled}
              error={error}
              sx={{ mr: 1, flex: 1 }}
            />
            <TextField
              label={label.valueName}
              value={value}
              onChange={(e) => handleChange(index, 'value', e.target.value)}
              disabled={disabled}
              error={error}
              sx={{ mr: 1, flex: 1 }}
            />
            {enableSkip && (
              <Tooltip title={t('channel_edit.mapSkip')}>
                <Checkbox
                  checked={!!skip}
                  onChange={(e) => handleChange(index, 'skip', e.target.checked)}
                  disabled={disabled}
                  size="small"
                  sx={{ mr: 1 }}
                />
              </Tooltip>
            )}
            <ListItemSecondaryAction>
              <IconButton edge="end" aria-label="delete" onClick={() => handleDelete(index)} disabled={disabled}>
                <Icon icon="mdi:delete" />
              </IconButton>
            </ListItemSecondaryAction>
          </ListItem>
        ))}
      </List>
      <Button startIcon={<Icon icon="mdi:plus" />} onClick={handleAdd} disabled={disabled}>
        {t('channel_edit.mapAdd', { name: label.name })}
      </Button>

      <Button startIcon={<Icon icon="mdi:plus" />} onClick={handleAddByJson} disabled={disabled}>
        {t('channel_edit.mapAddByJson', { name: label.name })}
      </Button>

      <Dialog open={openJsonDialog} onClose={handleCloseJsonDialog} fullWidth maxWidth="md">
        <DialogTitle>{t('channel_edit.mapAddByJson', { name: label.name })}</DialogTitle>
        <DialogContent>
          <Box
            sx={{
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 1,
              overflow: 'hidden',
              marginTop: 1,
              resize: 'vertical',
              height: '400px',
              minHeight: '200px',
              '&:hover': {
                borderColor: 'primary.main'
              },
              '&:focus-within': {
                borderColor: 'primary.main',
                borderWidth: 1
              }
            }}
          >
            <Editor
              height="100%"
              language="json"
              theme={theme.palette.mode === 'dark' ? 'vs-dark' : 'light'}
              value={jsonInput}
              options={{
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                automaticLayout: true,
                fontSize: 14,
                lineNumbers: 'on',
                folding: true,
                formatOnPaste: true,
                formatOnType: true
              }}
              onChange={(value) => setJsonInput(value)}
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseJsonDialog}>{t('common.cancel')}</Button>
          <Button onClick={handleJsonSubmit}>{t('common.submit')}</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

MapInput.propTypes = {
  mapValue: PropTypes.array,
  onChange: PropTypes.func.isRequired,
  disabled: PropTypes.bool,
  error: PropTypes.bool,
  enableSkip: PropTypes.bool,
  label: PropTypes.shape({
    name: PropTypes.string,
    keyName: PropTypes.string,
    valueName: PropTypes.string
  }).isRequired
};

export default MapInput;
