import PropTypes from 'prop-types';
import { useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography
} from '@mui/material';
import { Icon } from '@iconify/react';
import { copy, showError } from 'utils/common';

export const createKeyRow = (secret = '') => ({ secret, enabled: true, client_id: crypto.randomUUID() });

const KeyRow = ({ item, index, label, onChange, onRemove, removable }) => {
  const [visible, setVisible] = useState(false);
  return (
    <Stack direction="row" spacing={0.5} alignItems="center">
      <TextField
        fullWidth
        size="small"
        type={visible ? 'text' : 'password'}
        label={`${label} ${index + 1}`}
        value={item.secret}
        autoComplete="new-password"
        onChange={(event) => onChange({ ...item, secret: event.target.value })}
        InputProps={{
          endAdornment: (
            <InputAdornment position="end">
              <Tooltip title={visible ? '隐藏' : '显示'}>
                <IconButton
                  aria-label={`${visible ? '隐藏' : '显示'} Key ${index + 1}`}
                  size="small"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => setVisible(!visible)}
                >
                  <Icon icon={visible ? 'solar:eye-bold-duotone' : 'solar:eye-closed-bold-duotone'} />
                </IconButton>
              </Tooltip>
              <Tooltip title="复制">
                <span>
                  <IconButton aria-label={`复制 Key ${index + 1}`} size="small" disabled={!item.secret} onClick={() => copy(item.secret)}>
                    <Icon icon="solar:copy-outline" />
                  </IconButton>
                </span>
              </Tooltip>
            </InputAdornment>
          )
        }}
      />
      <Tooltip title={item.enabled ? '停用此 Key' : '启用此 Key'}>
        <Switch
          size="small"
          checked={item.enabled}
          inputProps={{ 'aria-label': `启用 Key ${index + 1}` }}
          onChange={(event) => onChange({ ...item, enabled: event.target.checked })}
        />
      </Tooltip>
      <Tooltip title="删除">
        <span>
          <IconButton aria-label={`删除 Key ${index + 1}`} size="small" disabled={!removable} onClick={onRemove}>
            <Icon icon="solar:trash-bin-trash-outline" />
          </IconButton>
        </span>
      </Tooltip>
    </Stack>
  );
};

KeyRow.propTypes = {
  item: PropTypes.object.isRequired,
  index: PropTypes.number.isRequired,
  label: PropTypes.string.isRequired,
  onChange: PropTypes.func.isRequired,
  onRemove: PropTypes.func.isRequired,
  removable: PropTypes.bool.isRequired
};

const ChannelKeysEditor = ({ keys, strategy, onChange, onStrategyChange, label = 'API Key' }) => {
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');
  const importKeys = () => {
    try {
      const text = importText.trim();
      let secrets;
      if (text.startsWith('{') || text.startsWith('[')) {
        const parsed = JSON.parse(text);
        secrets = (Array.isArray(parsed) ? parsed : [parsed]).map((item) =>
          typeof item === 'string' ? item.trim() : JSON.stringify(item)
        );
      } else {
        secrets = text
          .split(/\r?\n/)
          .map((item) => item.trim())
          .filter(Boolean);
      }
      if (!secrets.length) throw new Error('请先粘贴 Key');
      const existing = keys.filter((item) => item.secret.trim());
      const seen = new Set(existing.map((item) => item.secret.trim()));
      const added = secrets
        .filter((secret) => {
          if (!secret || seen.has(secret)) return false;
          seen.add(secret);
          return true;
        })
        .map(createKeyRow);
      if (existing.length + added.length > 1000) throw new Error('一个渠道最多支持 1000 个 Key');
      onChange([...existing, ...added]);
      setImportOpen(false);
      setImportText('');
    } catch (error) {
      showError(error instanceof SyntaxError ? 'JSON 格式不正确，请粘贴完整的凭据对象或数组' : error.message);
    }
  };
  const enabledCount = keys.filter((item) => item.enabled).length;
  return (
    <Box sx={{ my: 2 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
        <Typography variant="body2" color="text.secondary">
          {enabledCount} / {keys.length} 个已启用
        </Typography>
        <FormControl size="small" sx={{ minWidth: 150 }}>
          <InputLabel id="channel-key-strategy-label">调用策略</InputLabel>
          <Select
            labelId="channel-key-strategy-label"
            label="调用策略"
            value={strategy || 'round_robin'}
            onChange={(event) => onStrategyChange(event.target.value)}
          >
            <MenuItem value="round_robin">轮询</MenuItem>
            <MenuItem value="random">随机</MenuItem>
          </Select>
        </FormControl>
      </Stack>
      <Stack spacing={1.5}>
        {keys.map((item, index) => (
          <KeyRow
            key={item.id || item.client_id || index}
            item={item}
            index={index}
            label={label}
            removable={keys.length > 1}
            onChange={(updated) => onChange(keys.map((key, position) => (position === index ? updated : key)))}
            onRemove={() => onChange(keys.filter((_, position) => position !== index))}
          />
        ))}
      </Stack>
      <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
        <Button
          size="small"
          disabled={keys.length >= 1000}
          startIcon={<Icon icon="mdi:plus" />}
          onClick={() => onChange([...keys, createKeyRow()])}
        >
          添加 Key
        </Button>
        <Button size="small" onClick={() => setImportOpen(true)}>
          批量导入
        </Button>
      </Stack>
      {enabledCount === 0 && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          没有启用的 Key，保存后该渠道无法接收请求。
        </Alert>
      )}
      <Dialog open={importOpen} onClose={() => setImportOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>批量导入 Key</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            普通 Key 每行一个；OAuth 凭据可粘贴完整 JSON，多份凭据使用 JSON 数组。
          </Typography>
          <TextField
            autoFocus
            fullWidth
            multiline
            minRows={6}
            maxRows={12}
            label="Key / JSON 凭据"
            value={importText}
            onChange={(event) => setImportText(event.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setImportOpen(false)}>取消</Button>
          <Button variant="contained" onClick={importKeys}>
            导入
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

ChannelKeysEditor.propTypes = {
  keys: PropTypes.array.isRequired,
  strategy: PropTypes.string,
  label: PropTypes.string,
  onChange: PropTypes.func.isRequired,
  onStrategyChange: PropTypes.func.isRequired
};

export default ChannelKeysEditor;
