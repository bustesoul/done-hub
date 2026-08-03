import PropTypes from 'prop-types';
import { useEffect, useRef, useState } from 'react';
import { useTheme } from '@mui/material/styles';
import { API } from 'utils/api';
import { copy, showError, showSuccess, trims } from 'utils/common';
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  ButtonGroup,
  Checkbox,
  Chip,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormControlLabel,
  FormHelperText,
  IconButton,
  InputLabel,
  ListItemText,
  MenuItem,
  OutlinedInput,
  Select,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
  useMediaQuery
} from '@mui/material';
import { Formik } from 'formik';
import * as Yup from 'yup';
import { defaultConfig, typeConfig } from '../type/Config'; //typeConfig
import { createFilterOptions } from '@mui/material/Autocomplete';
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank';
import CheckBoxIcon from '@mui/icons-material/CheckBox';
import { useTranslation } from 'react-i18next';
import useCustomizeT from 'hooks/useCustomizeT';
import { formatGroupLabel } from './batchHelpers';
import ModelSelectorModal from './ModelSelectorModal';
import ConnectionProfilePicker from './ConnectionProfilePicker';
import CredentialRotation from './CredentialRotation';
import OAuthCredentialFlow from './OAuthCredentialFlow';
import ChannelAdvancedSection from './ChannelAdvancedSection';
import ChannelBillingSection from './ChannelBillingSection';
import CollapsibleSection from './CollapsibleSection';
import ConfirmDialog from 'ui-component/confirm-dialog';
import RatioBadge from 'ui-component/RatioBadge';
import GroupRatioLabel from 'ui-component/GroupRatioLabel';
import pluginList from '../type/Plugin.json';
import { Icon } from '@iconify/react';

const icon = <CheckBoxOutlineBlankIcon fontSize="small" />;
const checkedIcon = <CheckBoxIcon fontSize="small" />;

const filter = createFilterOptions();
const normalizeDraftValue = (value) => (typeof value === 'string' ? value.trim() : value);
const normalizeDraftBaseURL = (value) =>
  String(value || '')
    .split('\n')
    .map((item) => item.trim().replace(/\/$/, ''))
    .join('\n')
    .trim();
const resolveDraftTestModel = (values) => {
  const explicit = normalizeDraftValue(values.test_model);
  if (explicit) return explicit;
  const firstModel = Array.isArray(values.models) ? values.models.find((model) => normalizeDraftValue(model?.id || model)) : null;
  return normalizeDraftValue(firstModel?.id || firstModel || '');
};
const draftProbePayload = (values) => ({
  name: normalizeDraftValue(values.name),
  type: values.type,
  protocol_profile_id: normalizeDraftValue(values.protocol_profile_id),
  key: normalizeDraftValue(values.key),
  base_url: normalizeDraftBaseURL(values.base_url),
  proxy: normalizeDraftValue(values.proxy),
  other: normalizeDraftValue(values.other),
  test_model: resolveDraftTestModel(values)
});
const draftProbeFingerprint = (values) => {
  const { name: _name, ...probeBoundConfig } = draftProbePayload(values);
  return JSON.stringify(probeBoundConfig);
};
const getValidationSchema = (t, providerDefinitions) =>
  Yup.object().shape({
    is_edit: Yup.boolean(),
    // is_tag: Yup.boolean(),
    name: Yup.string().required(t('channel_edit.requiredName')),
    type: Yup.number().required(t('channel_edit.requiredChannel')),
    protocol_profile_id: Yup.string().nullable(),
    // Keyless providers (Ollama / local self-hosted) are validated by the
    // server-side provider definition and probe flow.
    key: Yup.string(),
    other: Yup.string(),
    proxy: Yup.string(),
    test_model: Yup.string(),
    models: Yup.array().min(1, t('channel_edit.requiredModels')),
    groups: Yup.array().min(1, t('channel_edit.requiredGroup')),
    base_url: Yup.string().test('base-url-policy', t('channel_edit.requiredBaseUrl'), function (value) {
      const definition = providerDefinitions.find((provider) => provider.channel_type === this.parent.type);
      return definition?.base_url_policy !== 'required' || Boolean(value?.trim());
    }),
    model_mapping: Yup.array(),
    model_headers: Yup.array(),
    header_override: Yup.array(),
    custom_parameter: Yup.string().nullable(),
    remark: Yup.string(),
    need2response_models: Yup.string().nullable()
  });

const EditModal = ({ open, channelId, onCancel, onOk, groupOptions, groupMap, isTag, modelOptions, prices, tags }) => {
  const { t } = useTranslation();
  const { t: customizeT } = useCustomizeT();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  // 分组编辑：此标签下的渠道总数，用于"覆盖全部"警告/二次确认的文案。
  // 取自 loadChannel 已加载的分组数据（权威且实时），不依赖单独的 _all 统计。
  const [tagGroupCount, setTagGroupCount] = useState(0);
  // 分组编辑保存前的二次确认（敏感操作：会覆盖整组渠道配置）
  const [saveConfirmOpen, setSaveConfirmOpen] = useState(false);
  const pendingSubmitRef = useRef(null);
  // const [loading, setLoading] = useState(false);
  const [initialInput, setInitialInput] = useState(defaultConfig.input);
  const [inputLabel, setInputLabel] = useState(defaultConfig.inputLabel); //
  const [inputPrompt, setInputPrompt] = useState(defaultConfig.prompt);
  const [batchAdd, setBatchAdd] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [parameterFocused, setParameterFocused] = useState(false);
  const parameterInputRef = useRef(null);
  const removeDuplicates = (array) => [...new Set(array)];
  const [modelSelectorOpen, setModelSelectorOpen] = useState(false);
  const [tempFormikValues, setTempFormikValues] = useState(null);
  const [tempSetFieldValue, setTempSetFieldValue] = useState(null);
  const [providerDefinitions, setProviderDefinitions] = useState([]);
  const [connectionProfiles, setConnectionProfiles] = useState([]);
  const [providerCatalogError, setProviderCatalogError] = useState('');
  const [draftProbeSignature, setDraftProbeSignature] = useState('');
  const [draftValidationToken, setDraftValidationToken] = useState('');
  const [draftProbeWorking, setDraftProbeWorking] = useState(false);

  // 用于追踪模型的原始名称映射关系 { displayName: originalName }
  const [modelOriginalMapping, setModelOriginalMapping] = useState({});

  const handleCancel = () => onCancel();

  const initChannel = (typeValue) => {
    if (typeConfig[typeValue]?.inputLabel) {
      setInputLabel({ ...defaultConfig.inputLabel, ...typeConfig[typeValue].inputLabel });
    } else {
      setInputLabel(defaultConfig.inputLabel);
    }

    if (typeConfig[typeValue]?.prompt) {
      setInputPrompt({ ...defaultConfig.prompt, ...typeConfig[typeValue].prompt });
    } else {
      setInputPrompt(defaultConfig.prompt);
    }

    return typeConfig[typeValue]?.input;
  };

  const loadProviderCatalog = async () => {
    try {
      const [definitionsResponse, profilesResponse] = await Promise.all([
        API.get('/api/admin/provider-definitions'),
        API.get('/api/admin/connection-profiles')
      ]);
      if (
        definitionsResponse.data?.success &&
        Array.isArray(definitionsResponse.data.data) &&
        profilesResponse.data?.success &&
        Array.isArray(profilesResponse.data.data)
      ) {
        setProviderDefinitions(definitionsResponse.data.data);
        setConnectionProfiles(profilesResponse.data.data);
        setProviderCatalogError('');
        return;
      }
      throw new Error('Provider catalog response is invalid');
    } catch (error) {
      setProviderDefinitions([]);
      setConnectionProfiles([]);
      setProviderCatalogError('接入方法定义加载失败，请刷新后重试。为避免保存错误配置，当前禁止提交。');
    }
  };

  const probeDraftConnection = async (values) => {
    setDraftProbeWorking(true);
    try {
      const payload = draftProbePayload(values);
      if (!payload.key && !providerDefinitions.find((item) => item.channel_type === payload.type)?.auth_modes?.includes('none')) {
        throw new Error('请先填写凭据');
      }
      const response = await API.post('/api/admin/provider-connections/probe', payload);
      if (!response.data?.success) {
        throw new Error(response.data?.message || '连接探测失败');
      }
      setDraftProbeSignature(draftProbeFingerprint(values));
      setDraftValidationToken(response.data.data?.validation_token || '');
      showSuccess(`${response.data.data?.tested_connections || 1} 条连接探测全部通过（最慢 ${response.data.data?.latency_ms ?? '-'} ms）`);
    } catch (error) {
      setDraftProbeSignature('');
      setDraftValidationToken('');
      if (!error.shownByApiInterceptor) {
        showError(error.response?.data?.error?.message || error.response?.data?.message || error.message);
      }
    } finally {
      setDraftProbeWorking(false);
    }
  };

  // 解析模型映射配置的工具函数
  const parseModelMapping = (mappingArray) => {
    if (!mappingArray || !Array.isArray(mappingArray) || mappingArray.length === 0) {
      return null;
    }

    try {
      const mapping = mappingArray.reduce((acc, item) => {
        if (item.key && item.value) {
          acc[item.key] = item.value;
        }
        return acc;
      }, {});

      if (Object.keys(mapping).length === 0) {
        return null;
      }
      return mapping;
    } catch (error) {
      console.warn('模型重定向解析失败:', error);
      return null;
    }
  };

  // 更新模型列表的统一方法
  const updateModelsList = (newModels, newMapping, setFieldValue) => {
    const uniqueModels = Array.from(new Set(newModels.filter((model) => model && model.id && model.id.trim())));

    setFieldValue('models', uniqueModels);
    setModelOriginalMapping(newMapping);
  };

  // 恢复模型到原始名称
  const restoreModelsToOriginalNames = (currentModels, setFieldValue) => {
    const restoredModels = currentModels.map((model) => {
      const originalName = modelOriginalMapping[model.id] || model.id;
      return {
        ...model,
        id: originalName
      };
    });

    // 检查是否有变化
    const hasChanges = currentModels.some((model, index) => {
      return model.id !== restoredModels[index].id;
    });

    if (hasChanges) {
      updateModelsList(restoredModels, {}, setFieldValue);
    }
  };

  // 应用模型映射的核心逻辑
  const applyModelMapping = (mapping, currentModels, currentMapping, setFieldValue) => {
    let updatedModels = [...currentModels];
    let newMapping = { ...currentMapping };
    let hasChanges = false;

    // 遍历重定向映射
    Object.entries(mapping).forEach(([key, mappedValue]) => {
      if (typeof key === 'string' && typeof mappedValue === 'string') {
        const keyTrimmed = key.trim();
        const valueTrimmed = mappedValue.trim();

        if (keyTrimmed && valueTrimmed) {
          // 查找模型配置中是否存在重定向的"值"（原始模型名）
          const valueIndex = updatedModels.findIndex((model) => {
            return model.id === valueTrimmed || newMapping[model.id] === valueTrimmed;
          });

          if (valueIndex !== -1) {
            const currentDisplayName = updatedModels[valueIndex].id;
            if (currentDisplayName !== keyTrimmed) {
              // 记录原始映射关系
              if (!newMapping[keyTrimmed]) {
                newMapping[keyTrimmed] = newMapping[currentDisplayName] || currentDisplayName;
              }
              // 清理旧的映射关系
              if (newMapping[currentDisplayName]) {
                delete newMapping[currentDisplayName];
              }
              // 更新显示名称为重定向的键
              updatedModels[valueIndex] = {
                ...updatedModels[valueIndex],
                id: keyTrimmed
              };
              hasChanges = true;
            }
          }
        }
      }
    });

    // 处理不在映射中的模型，恢复为原始名称
    const mappingKeys = new Set(Object.keys(mapping).map((key) => key.trim()));
    updatedModels = updatedModels.map((model) => {
      if (!mappingKeys.has(model.id) && newMapping[model.id]) {
        const originalName = newMapping[model.id];
        delete newMapping[model.id];
        hasChanges = true;
        return {
          ...model,
          id: originalName
        };
      }
      return model;
    });

    return { updatedModels, newMapping, hasChanges };
  };

  // 实时同步模型重定向到模型配置的函数
  const syncModelMappingToModels = (mappingArray, currentModels, setFieldValue) => {
    const mapping = parseModelMapping(mappingArray);

    if (!mapping) {
      restoreModelsToOriginalNames(currentModels, setFieldValue);
      return;
    }

    const { updatedModels, newMapping, hasChanges } = applyModelMapping(mapping, currentModels, modelOriginalMapping, setFieldValue);

    if (hasChanges) {
      updateModelsList(updatedModels, newMapping, setFieldValue);
    }
  };

  const handleTypeChange = (setFieldValue, typeValue, values) => {
    // 处理插件事务
    if (pluginList[typeValue]) {
      const newPluginValues = {};
      const pluginConfig = pluginList[typeValue];
      for (const pluginName in pluginConfig) {
        const plugin = pluginConfig[pluginName];
        const oldValve = values['plugin'] ? values['plugin'][pluginName] || {} : {};
        newPluginValues[pluginName] = {};
        for (const paramName in plugin.params) {
          const param = plugin.params[paramName];
          newPluginValues[pluginName][paramName] = oldValve[paramName] || (param.type === 'bool' ? false : '');
        }
      }
      setFieldValue('plugin', newPluginValues);
    }

    const newInput = initChannel(typeValue);
    const providerDefaults = providerDefinitions.find((provider) => provider.channel_type === typeValue)?.defaults || {};

    if (newInput) {
      Object.keys(newInput).forEach((key) => {
        if (
          (!Array.isArray(values[key]) && values[key] !== null && values[key] !== undefined && values[key] !== '') ||
          (Array.isArray(values[key]) && values[key].length > 0)
        ) {
          return;
        }

        if (key === 'models') {
          setFieldValue(key, initialModel(newInput[key]));
          return;
        }
        setFieldValue(key, newInput[key]);
      });
    }
    Object.entries(providerDefaults).forEach(([field, value]) => {
      if (values[field] === '' || values[field] === null || values[field] === undefined) {
        setFieldValue(field, value);
      }
    });
  };

  const basicModels = (channelType) => {
    let modelGroup = typeConfig[channelType]?.modelGroup || defaultConfig.modelGroup;
    // 循环 modelOptions，找到 modelGroup 对应的模型
    let modelList = [];
    modelOptions.forEach((model) => {
      if (model.group === modelGroup) {
        modelList.push(model);
      }
    });
    return modelList;
  };

  const handleModelSelectorConfirm = (selectedModels, overwriteModels) => {
    if (tempSetFieldValue && tempFormikValues) {
      if (overwriteModels) {
        // 覆盖模式：清空现有的模型列表，使用选择器中的模型
        tempSetFieldValue('models', selectedModels);
      } else {
        // 追加模式：合并现有模型和新选择的模型，避免重复
        const existingModels = tempFormikValues.models || [];
        const existingModelIds = new Set(existingModels.map((model) => model.id));

        // 过滤掉已存在的模型，避免重复
        const newModels = selectedModels.filter((model) => !existingModelIds.has(model.id));

        // 合并模型列表
        tempSetFieldValue('models', [...existingModels, ...newModels]);
      }
    }
  };

  // 分组编辑（isTag 且为编辑）保存会以同一份配置覆盖整组渠道，先二次确认再提交；其余情况直接提交
  const submit = async (values, helpers) => {
    if (isTag && channelId) {
      pendingSubmitRef.current = { values, helpers };
      setSaveConfirmOpen(true);
      return;
    }
    return doSubmit(values, helpers);
  };

  const doSubmit = async (values, { setErrors, setStatus, setSubmitting }) => {
    setSubmitting(true);
    values = trims(values);
    const draftPayload = draftProbePayload(values);
    const saveUnverified = !channelId && !isTag && draftProbeSignature !== draftProbeFingerprint(values);
    if (channelId && !isTag && values.key) {
      const message = '新凭据尚未进入轮换流程，请先点击“建立候选”，完成测试和激活后再保存其他配置。';
      setSubmitting(false);
      setStatus({ success: false });
      setErrors({ submit: message });
      showError(message);
      return;
    }
    if (values.base_url && values.base_url.endsWith('/')) {
      values.base_url = values.base_url.slice(0, values.base_url.length - 1);
    }
    const providerDefaults = providerDefinitions.find((provider) => provider.channel_type === values.type)?.defaults || {};
    Object.entries(providerDefaults).forEach(([field, value]) => {
      if (values[field] === '' || values[field] === null || values[field] === undefined) values[field] = value;
    });
    let res;

    let modelMappingModel = [];

    if (values.model_mapping) {
      try {
        const modelMapping = values.model_mapping.reduce((acc, item) => {
          if (item.key && item.value) {
            acc[item.key] = item.value;
          }
          return acc;
        }, {});
        const cleanedMapping = {};

        for (const [key, value] of Object.entries(modelMapping)) {
          if (key && value && !(key in cleanedMapping)) {
            cleanedMapping[key] = value;
            modelMappingModel.push(key);
          }
        }

        values.model_mapping = JSON.stringify(cleanedMapping, null, 2);
      } catch (error) {
        showError('Error parsing model_mapping:' + error.message);
      }
    }
    let modelHeadersKey = [];

    if (values.model_headers) {
      try {
        // skip=true 编码为 {"value":v,"skip":true}，否则保留为字符串 v
        const modelHeader = values.model_headers.reduce((acc, item) => {
          if (item.key && item.value) {
            acc[item.key] = item.skip ? { value: item.value, skip: true } : item.value;
          }
          return acc;
        }, {});
        const cleanedHeader = {};

        for (const [key, value] of Object.entries(modelHeader)) {
          if (key && value && !(key in cleanedHeader)) {
            cleanedHeader[key] = value;
            modelHeadersKey.push(key);
          }
        }

        values.model_headers = JSON.stringify(cleanedHeader, null, 2);
      } catch (error) {
        showError('Error parsing model_headers:' + error.message);
      }
    }

    if (values.header_override) {
      try {
        const headerOverride = values.header_override.reduce((acc, item) => {
          if (item.key && item.value && !(item.key in acc)) {
            acc[item.key] = item.value;
          }
          return acc;
        }, {});
        values.header_override = JSON.stringify(headerOverride, null, 2);
      } catch (error) {
        showError('Error parsing header_override:' + error.message);
      }
    }

    if (values.custom_parameter) {
      try {
        // Validate that the custom_parameter is valid JSON
        JSON.parse(values.custom_parameter);
      } catch (error) {
        showError('Error parsing custom_parameter: ' + error.message);
        return;
      }
    }

    if (values.disabled_stream) {
      values.disabled_stream = removeDuplicates(values.disabled_stream);
    }

    // 获取现有的模型 ID
    const existingModelIds = values.models.map((model) => model.id);

    // 找出在 modelMappingModel 中存在但不在 existingModelIds 中的模型
    const newModelIds = modelMappingModel.filter((id) => !existingModelIds.includes(id));

    // 合并现有的模型 ID 和新的模型 ID，并去重
    const allUniqueModelIds = Array.from(new Set([...existingModelIds, ...newModelIds]));

    // 创建新的 modelsStr
    const modelsStr = allUniqueModelIds.join(',');
    values.group = values.groups.join(',');

    // cost_ratio 经 type=number 输入后为字符串，后端字段为 *float64，需转回数字；未配置或非法值按 0 处理（不计成本）
    values.cost_ratio = parseFloat(values.cost_ratio);
    if (isNaN(values.cost_ratio) || values.cost_ratio <= 0) {
      values.cost_ratio = 0;
    }

    let baseApiUrl = '/api/admin/provider-connections';

    if (isTag) {
      baseApiUrl = '/api/channel_tag/' + encodeURIComponent(channelId);
    }

    try {
      if (channelId) {
        res = isTag
          ? await API.put(baseApiUrl, { ...values, id: parseInt(channelId), models: modelsStr })
          : await API.patch(`${baseApiUrl}/${channelId}`, { ...values, id: parseInt(channelId), models: modelsStr });
      } else {
        res = await API.post(baseApiUrl, {
          ...values,
          base_url: draftPayload.base_url,
          test_model: draftPayload.test_model,
          models: modelsStr,
          validation_token: draftValidationToken,
          save_unverified: saveUnverified
        });
      }
      const { success, message } = res.data;
      if (success) {
        if (channelId) {
          showSuccess(t('channel_edit.editSuccess'));
        } else if (saveUnverified) {
          showSuccess('连接已保存为未验证状态并保持停用；探测通过后才能启用。');
        } else {
          showSuccess(t('channel_edit.addSuccess'));
        }
        setSubmitting(false);
        setStatus({ success: true });
        onOk(true);
      } else {
        setStatus({ success: false });
        showError(message);
        setErrors({ submit: message });
      }
    } catch (error) {
      setStatus({ success: false });
      if (!error.shownByApiInterceptor) showError(error.message);
      setErrors({ submit: error.message });
    }
  };

  function initialModel(channelModel) {
    if (!channelModel) {
      return [];
    }

    // 如果 channelModel 是一个字符串
    if (typeof channelModel === 'string') {
      channelModel = channelModel.split(',');
    }
    let modelList = channelModel.map((model) => {
      const modelOption = modelOptions.find((option) => option.id === model);
      if (modelOption) {
        return modelOption;
      }
      return { id: model, group: t('channel_edit.customModelTip') };
    });
    return modelList;
  }

  const loadChannel = async () => {
    try {
      let baseApiUrl = `/api/admin/provider-connections/${channelId}`;

      if (isTag) {
        baseApiUrl = '/api/channel_tag/' + encodeURIComponent(channelId);
      }

      let res = await API.get(baseApiUrl);
      const { success, message, data } = res.data;
      if (success) {
        if (data.models === '') {
          data.models = [];
        } else {
          data.models = initialModel(data.models);
        }
        if (data.group === '') {
          data.groups = [];
        } else {
          data.groups = data.group.split(',');
        }

        data.model_mapping =
          data.model_mapping !== ''
            ? Object.entries(JSON.parse(data.model_mapping)).map(([key, value], index) => ({
                index,
                key,
                value
              }))
            : [];

        // 初始化模型原始映射关系
        const mapping = parseModelMapping(data.model_mapping);
        if (mapping) {
          const initialMapping = {};
          // 根据当前的模型映射和模型列表，建立原始映射关系
          Object.entries(mapping).forEach(([key, value]) => {
            const modelExists = data.models.some((model) => model.id === key);
            if (modelExists) {
              initialMapping[key] = value;
            }
          });
          setModelOriginalMapping(initialMapping);
        } else {
          setModelOriginalMapping({});
        }
        // if (data.model_headers) {
        data.model_headers =
          data.model_headers !== ''
            ? Object.entries(JSON.parse(data.model_headers)).map(([key, raw], index) => {
                const isObj = raw !== null && typeof raw === 'object';
                return {
                  index,
                  key,
                  value: isObj ? raw.value : raw,
                  skip: isObj ? raw.skip === true : false
                };
              })
            : [];
        // }

        data.header_override = data.header_override
          ? Object.entries(JSON.parse(data.header_override)).map(([key, value], index) => ({
              index,
              key,
              value
            }))
          : [];

        // Format the custom_parameter JSON for better readability if it's not empty
        if (data.custom_parameter !== '') {
          try {
            // Parse and then stringify with indentation for formatting
            const parsedJson = JSON.parse(data.custom_parameter);
            data.custom_parameter = JSON.stringify(parsedJson, null, 2);
          } catch (error) {
            // If parsing fails, keep the original string
          }
        } else {
          data.custom_parameter = '';
        }

        data.base_url = data.base_url ?? '';
        data.cost_ratio = data.cost_ratio ?? 0;
        data.remark = data.remark ?? '';
        if (data.need2response_models === null || data.need2response_models === undefined) {
          data.need2response_models = '';
        }
        data.is_edit = true;
        if (data.plugin === null) {
          data.plugin = {};
        }
        initChannel(data.type);
        setInitialInput(data);

        if (isTag) {
          // 优先用后端返回的 count；旧后端无该字段时回退到 KeyMap 条数
          setTagGroupCount(data.count || Object.keys(data.KeyMap || {}).length || 0);
        }
      } else {
        showError(message);
      }
    } catch (error) {}
  };

  useEffect(() => {
    if (open) {
      loadProviderCatalog();
      setBatchAdd(isTag);
      if (channelId) {
        loadChannel().then();
      } else {
        initChannel(1);
        setInitialInput({ ...defaultConfig.input, is_edit: false });
        setDraftProbeSignature('');
        setDraftValidationToken('');
        // 重置模型原始映射关系
        setModelOriginalMapping({});
      }
    }

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, open]);

  return (
    <Dialog open={open} onClose={handleCancel} fullWidth maxWidth={'md'}>
      <DialogTitle sx={{ margin: '0px', fontWeight: 700, lineHeight: '1.55556', padding: '24px', fontSize: '1.125rem' }}>
        {channelId ? t('common.edit') : t('common.create')}
      </DialogTitle>
      <Divider />
      <DialogContent>
        <Formik
          initialValues={initialInput}
          enableReinitialize
          validationSchema={getValidationSchema(t, providerDefinitions)}
          onSubmit={submit}
        >
          {({ errors, handleBlur, handleChange, handleSubmit, isSubmitting, touched, values, setFieldValue }) => {
            const affinitySupported = Boolean(
              connectionProfiles.find((profile) => profile.id === values.protocol_profile_id)?.supports_affinity
            );
            // 保存当前Formik状态，以便在模型选择器中使用
            const openModelSelector = () => {
              setTempFormikValues({ ...values });
              setTempSetFieldValue(() => setFieldValue); // 保存函数引用
              setModelSelectorOpen(true);
            };

            return (
              <form noValidate onSubmit={handleSubmit}>
                {/* 成员数依赖异步 loadChannel，count 就绪前不渲染，避免首帧闪现「覆盖全部 0 个渠道」 */}
                {isTag && tagGroupCount > 0 && (
                  <Alert severity="warning" sx={{ mb: 2 }}>
                    {t('channel_edit.tagGroupOverwriteWarning', { count: tagGroupCount })}
                  </Alert>
                )}
                <CollapsibleSection title={t('channel_edit.sectionBasic')} defaultExpanded>
                  {!isTag && (
                    <>
                      <ConnectionProfilePicker
                        profiles={connectionProfiles}
                        providers={providerDefinitions}
                        profileId={values.protocol_profile_id || ''}
                        channelType={values.type}
                        error={providerCatalogError}
                        disabled={Boolean(channelId)}
                        onChange={(profileId, channelType, variant, profile) => {
                          setFieldValue('protocol_profile_id', profileId);
                          setFieldValue('type', channelType);
                          handleTypeChange(setFieldValue, channelType, values);
                          if (!values.base_url && variant?.default_base_url) {
                            setFieldValue('base_url', variant.default_base_url);
                          }
                          if (!values.name && profile?.display_name) {
                            setFieldValue('name', `${profile.display_name} · ${variant?.display_name || ''}`.replace(/ · $/, ''));
                          }
                          if (!channelId) {
                            const authModes =
                              variant?.auth_modes ||
                              providerDefinitions.find((provider) => provider.channel_type === channelType)?.auth_modes ||
                              [];
                            setFieldValue('affinity_enabled', authModes.includes('oauth'));
                          }
                        }}
                      />
                      {touched.protocol_profile_id && errors.protocol_profile_id && (
                        <FormHelperText error>{errors.protocol_profile_id}</FormHelperText>
                      )}
                    </>
                  )}

                  {!isTag && (
                    <FormControl fullWidth error={Boolean(touched.name && errors.name)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-name-label">{customizeT(inputLabel.name)}</InputLabel>
                      <OutlinedInput
                        id="channel-name-label"
                        label={customizeT(inputLabel.name)}
                        type="text"
                        value={values.name}
                        name="name"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        inputProps={{ autoComplete: 'name' }}
                        aria-describedby="helper-text-channel-name-label"
                      />
                      {touched.name && errors.name ? (
                        <FormHelperText error id="helper-tex-channel-name-label">
                          {errors.name}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-name-label"> {customizeT(inputPrompt.name)} </FormHelperText>
                      )}
                    </FormControl>
                  )}

                  <FormControl fullWidth error={Boolean(touched.tag && errors.tag)} sx={{ ...theme.typography.otherInput }}>
                    <InputLabel htmlFor="channel-tag-label">{customizeT(inputLabel.tag)}</InputLabel>
                    <OutlinedInput
                      id="channel-tag-label"
                      label={customizeT(inputLabel.tag)}
                      type="text"
                      value={values.tag}
                      name="tag"
                      onBlur={handleBlur}
                      onChange={handleChange}
                      inputProps={{}}
                      aria-describedby="helper-text-channel-tag-label"
                    />
                    {touched.tag && errors.tag ? (
                      <FormHelperText error id="helper-tex-channel-tag-label">
                        {errors.tag}
                      </FormHelperText>
                    ) : (
                      <FormHelperText id="helper-tex-channel-tag-label"> {customizeT(inputPrompt.tag)} </FormHelperText>
                    )}
                    {(() => {
                      // 普通渠道编辑/新建时，输入了一个已存在的标签 → 将加入该分组，后续分组统一编辑会覆盖其配置
                      const joinTag =
                        !isTag && values.tag && values.tag !== initialInput.tag ? tags?.find((tg) => tg.tag === values.tag) : null;
                      if (!joinTag) return null;
                      return (
                        <FormHelperText sx={{ color: 'warning.main' }}>
                          {t('channel_edit.joinExistingTagWarning', { tag: values.tag, count: joinTag.count })}
                        </FormHelperText>
                      );
                    })()}
                  </FormControl>

                  {inputPrompt.base_url && (
                    <FormControl fullWidth error={Boolean(touched.base_url && errors.base_url)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-base_url-label">{customizeT(inputLabel.base_url)}</InputLabel>
                      <OutlinedInput
                        id="channel-base_url-label"
                        label={customizeT(inputLabel.base_url)}
                        type="text"
                        value={values.base_url}
                        name="base_url"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        inputProps={{}}
                        aria-describedby="helper-text-channel-base_url-label"
                      />

                      {touched.base_url && errors.base_url ? (
                        <FormHelperText error id="helper-tex-channel-base_url-label">
                          {errors.base_url}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-base_url-label"> {customizeT(inputPrompt.base_url)} </FormHelperText>
                      )}
                    </FormControl>
                  )}

                  {inputPrompt.other && (
                    <FormControl fullWidth error={Boolean(touched.other && errors.other)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-other-label">{customizeT(inputLabel.other)}</InputLabel>
                      <OutlinedInput
                        id="channel-other-label"
                        label={customizeT(inputLabel.other)}
                        type="text"
                        value={values.other}
                        name="other"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        inputProps={{}}
                        aria-describedby="helper-text-channel-other-label"
                      />
                      {touched.other && errors.other ? (
                        <FormHelperText error id="helper-tex-channel-other-label">
                          {errors.other}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-other-label"> {customizeT(inputPrompt.other)} </FormHelperText>
                      )}
                    </FormControl>
                  )}

                  {inputPrompt.remark && (
                    <FormControl fullWidth error={Boolean(touched.remark && errors.remark)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-remark-label">{customizeT(inputLabel.remark)}</InputLabel>
                      <OutlinedInput
                        id="channel-remark-label"
                        label={customizeT(inputLabel.remark)}
                        type="text"
                        value={values.remark}
                        name="remark"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        multiline
                        minRows={2}
                        inputProps={{}}
                        aria-describedby="helper-text-channel-remark-label"
                      />
                      {touched.remark && errors.remark ? (
                        <FormHelperText error id="helper-tex-channel-remark-label">
                          {errors.remark}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-remark-label"> {customizeT(inputPrompt.remark)} </FormHelperText>
                      )}
                    </FormControl>
                  )}

                  <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                    <Autocomplete
                      multiple
                      id="channel-groups-label"
                      options={groupOptions}
                      value={values.groups}
                      getOptionLabel={(option) => formatGroupLabel(option, groupMap)}
                      onChange={(e, value) => {
                        const event = {
                          target: {
                            name: 'groups',
                            value: value
                          }
                        };
                        handleChange(event);
                      }}
                      onBlur={handleBlur}
                      filterSelectedOptions
                      renderOption={(props, option) => {
                        const group = groupMap[option];
                        return (
                          <Box component="li" {...props} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <ListItemText
                              sx={{ my: 0, flex: 1, minWidth: 0 }}
                              primary={formatGroupLabel(option, groupMap)}
                              secondary={group?.description || null}
                              primaryTypographyProps={{ sx: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' } }}
                              secondaryTypographyProps={{ sx: { fontSize: '0.7rem', whiteSpace: 'normal', lineHeight: 1.2 } }}
                            />
                            <RatioBadge ratio={group?.ratio} />
                          </Box>
                        );
                      }}
                      renderTags={(value, getTagProps) =>
                        value.map((option, index) => {
                          // 透传 getTagProps 余下的 data-tag-index / tabIndex，保留 MUI 标签键盘焦点导航
                          const { key, onDelete, ...rest } = getTagProps({ index });
                          return (
                            <GroupRatioLabel
                              key={key}
                              label={option}
                              ratio={groupMap[option]?.ratio}
                              onDelete={onDelete}
                              sx={{ m: '3px' }}
                              {...rest}
                            />
                          );
                        })
                      }
                      renderInput={(params) => (
                        <TextField {...params} name="groups" error={Boolean(errors.groups)} label={customizeT(inputLabel.groups)} />
                      )}
                      aria-describedby="helper-text-channel-groups-label"
                    />
                    {errors.groups ? (
                      <FormHelperText error id="helper-tex-channel-groups-label">
                        {errors.groups}
                      </FormHelperText>
                    ) : (
                      <FormHelperText id="helper-tex-channel-groups-label"> {customizeT(inputPrompt.groups)} </FormHelperText>
                    )}
                  </FormControl>

                  <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                    <Box sx={{ position: 'relative' }}>
                      <Autocomplete
                        multiple
                        freeSolo
                        disableCloseOnSelect
                        id="channel-models-label"
                        options={modelOptions}
                        value={values.models}
                        inputValue={inputValue}
                        onInputChange={(event, newInputValue) => {
                          if (newInputValue.includes(',')) {
                            const modelsList = newInputValue
                              .split(',')
                              .map((item) => ({
                                id: item.trim(),
                                group: t('channel_edit.customModelTip')
                              }))
                              .filter((item) => item.id);

                            const updatedModels = [...new Set([...values.models, ...modelsList])];
                            const event = {
                              target: {
                                name: 'models',
                                value: updatedModels
                              }
                            };
                            handleChange(event);
                            setInputValue('');
                          } else {
                            setInputValue(newInputValue);
                          }
                        }}
                        onChange={(e, value) => {
                          const event = {
                            target: {
                              name: 'models',
                              value: value.map((item) =>
                                typeof item === 'string' ? { id: item, group: t('channel_edit.customModelTip') } : item
                              )
                            }
                          };
                          handleChange(event);
                        }}
                        renderInput={(params) => (
                          <TextField
                            {...params}
                            name="models"
                            error={Boolean(errors.models)}
                            label={customizeT(inputLabel.models)}
                            InputProps={{
                              ...params.InputProps
                            }}
                          />
                        )}
                        groupBy={(option) => option.group}
                        getOptionLabel={(option) => {
                          if (typeof option === 'string') {
                            return option;
                          }
                          if (option.inputValue) {
                            return option.inputValue;
                          }
                          return option.id;
                        }}
                        filterOptions={(options, params) => {
                          const filtered = filter(options, params);
                          const { inputValue } = params;
                          const isExisting = options.some((option) => inputValue === option.id);
                          if (inputValue !== '' && !isExisting) {
                            filtered.push({
                              id: inputValue,
                              group: t('channel_edit.customModelTip')
                            });
                          }
                          return filtered;
                        }}
                        renderOption={(props, option, { selected }) => (
                          <li {...props}>
                            <Checkbox icon={icon} checkedIcon={checkedIcon} style={{ marginRight: 8 }} checked={selected} />
                            {option.id}
                          </li>
                        )}
                        renderTags={(value, getTagProps) =>
                          value.map((option, index) => {
                            const tagProps = getTagProps({ index });
                            return (
                              <Chip
                                key={index}
                                label={option.id}
                                {...tagProps}
                                onClick={() => copy(option.id)}
                                sx={{
                                  maxWidth: '100%',
                                  height: 'auto',
                                  margin: '3px',
                                  '& .MuiChip-label': {
                                    whiteSpace: 'normal',
                                    wordBreak: 'break-word',
                                    padding: '6px 8px',
                                    lineHeight: 1.4,
                                    fontWeight: 400
                                  },
                                  '& .MuiChip-deleteIcon': {
                                    margin: '0 5px 0 -6px'
                                  }
                                }}
                              />
                            );
                          })
                        }
                        sx={{
                          '& .MuiAutocomplete-tag': {
                            margin: '2px'
                          },
                          '& .MuiAutocomplete-inputRoot': {
                            flexWrap: 'wrap'
                          }
                        }}
                      />
                    </Box>
                    {errors.models ? (
                      <FormHelperText error id="helper-tex-channel-models-label">
                        {errors.models}
                      </FormHelperText>
                    ) : (
                      <FormHelperText id="helper-tex-channel-models-label"> {customizeT(inputPrompt.models)} </FormHelperText>
                    )}
                  </FormControl>
                  <Container
                    sx={{
                      textAlign: isMobile ? 'center' : 'right',
                      p: 0
                    }}
                  >
                    {!isMobile ? (
                      <ButtonGroup variant="outlined" aria-label="small outlined primary button group">
                        <Button
                          size="small"
                          onClick={() => {
                            const modelString = values.models.map((model) => model.id).join(',');
                            copy(modelString);
                          }}
                        >
                          {t('channel_edit.copyModels')}
                        </Button>
                        <Button
                          size="small"
                          onClick={() => {
                            setFieldValue('models', basicModels(values.type));
                          }}
                        >
                          {t('channel_edit.inputChannelModel')}
                        </Button>
                        {inputLabel.provider_models_list && (
                          <Tooltip title={customizeT(inputPrompt.provider_models_list)} placement="top">
                            <Button size="small" onClick={openModelSelector} startIcon={<Icon icon="mdi:cloud-download" />}>
                              {customizeT(inputLabel.provider_models_list)}
                            </Button>
                          </Tooltip>
                        )}
                      </ButtonGroup>
                    ) : (
                      <Stack
                        direction="row"
                        spacing={1}
                        divider={<Divider orientation="vertical" flexItem />}
                        justifyContent="space-around"
                        alignItems="center"
                      >
                        <IconButton
                          size="small"
                          onClick={() => {
                            const modelString = values.models.map((model) => model.id).join(',');
                            copy(modelString);
                          }}
                        >
                          <Icon icon="mdi:content-copy" />
                        </IconButton>
                        <IconButton
                          size="small"
                          onClick={() => {
                            setFieldValue('models', basicModels(values.type));
                          }}
                        >
                          <Icon icon="mdi:playlist-plus" />
                        </IconButton>
                        {inputLabel.provider_models_list && (
                          <Tooltip title={customizeT(inputPrompt.provider_models_list)} placement="top">
                            <IconButton size="small" onClick={openModelSelector}>
                              <Icon icon="mdi:cloud-download" />
                            </IconButton>
                          </Tooltip>
                        )}
                      </Stack>
                    )}
                  </Container>
                  {/* 分组编辑：Key 是逐行字段、不会被共享配置覆盖，因此不在此处编辑；
                      成员的增删在列表展开行内逐个管理。此处仅作只读说明，values.key 保持原值以免保存时误改成员。 */}
                  {isTag ? (
                    <Box
                      sx={{
                        my: 1,
                        p: 1.5,
                        borderRadius: 1,
                        bgcolor: 'background.neutral',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1.5
                      }}
                    >
                      <Icon icon="solar:users-group-rounded-bold-duotone" width={28} height={28} style={{ flexShrink: 0, opacity: 0.7 }} />
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="subtitle2">{t('channel_edit.tagMembersTitle', { count: tagGroupCount })}</Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                          {t('channel_edit.tagMembersHint')}
                        </Typography>
                      </Box>
                    </Box>
                  ) : channelId ? (
                    <CredentialRotation
                      channelId={channelId}
                      testModel={values.test_model}
                      secret={values.key}
                      onSecretChange={(secret) => setFieldValue('key', secret)}
                    />
                  ) : (
                    <FormControl fullWidth error={Boolean(touched.key && errors.key)} sx={{ ...theme.typography.otherInput }}>
                      {!batchAdd ? (
                        <>
                          <InputLabel htmlFor="channel-key-label">{customizeT(inputLabel.key)}</InputLabel>
                          <OutlinedInput
                            id="channel-key-label"
                            label={customizeT(inputLabel.key)}
                            type="text"
                            value={values.key}
                            name="key"
                            onBlur={handleBlur}
                            onChange={handleChange}
                            inputProps={{}}
                            aria-describedby="helper-text-channel-key-label"
                          />
                        </>
                      ) : (
                        <TextField
                          multiline
                          id="channel-key-label"
                          label={customizeT(inputLabel.key)}
                          value={values.key}
                          name="key"
                          onBlur={handleBlur}
                          onChange={handleChange}
                          aria-describedby="helper-text-channel-key-label"
                          minRows={5}
                          maxRows={15}
                          placeholder={customizeT(inputPrompt.key) + t('channel_edit.batchKeytip')}
                        />
                      )}

                      {touched.key && errors.key ? (
                        <FormHelperText error id="helper-tex-channel-key-label">
                          {errors.key}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-key-label">
                          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <span>{customizeT(inputPrompt.key)}</span>
                            {channelId === 0 && (
                              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Switch size="small" checked={Boolean(batchAdd)} onChange={(e) => setBatchAdd(e.target.checked)} />
                                <Typography variant="body2">{t('channel_edit.batchAdd')}</Typography>
                              </Box>
                            )}
                          </Box>
                        </FormHelperText>
                      )}
                    </FormControl>
                  )}

                  <OAuthCredentialFlow
                    definition={providerDefinitions.find((provider) => provider.channel_type === values.type)}
                    channelId={channelId}
                    projectId={values.other}
                    proxy={values.proxy}
                    disabled={batchAdd}
                    active={open}
                    onCredential={(credential, result) => {
                      setFieldValue('key', credential);
                      if (result?.project_id) setFieldValue('other', result.project_id);
                    }}
                  />
                </CollapsibleSection>

                {affinitySupported && (
                  <CollapsibleSection title={t('channel_edit.sectionScheduling')}>
                    <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={Boolean(values.affinity_enabled)}
                            onChange={(event) => setFieldValue('affinity_enabled', event.target.checked)}
                          />
                        }
                        label={t('channel_edit.requestAffinity')}
                      />
                      <FormHelperText>{t('channel_edit.requestAffinityHelp')}</FormHelperText>
                    </FormControl>
                  </CollapsibleSection>
                )}

                <ChannelAdvancedSection
                  title={t('channel_edit.sectionAdvanced')}
                  inputPrompt={inputPrompt}
                  inputLabel={inputLabel}
                  customizeT={customizeT}
                  touched={touched}
                  errors={errors}
                  theme={theme}
                  values={values}
                  setFieldValue={setFieldValue}
                  handleBlur={handleBlur}
                  handleChange={handleChange}
                  isTag={isTag}
                  syncModelMappingToModels={syncModelMappingToModels}
                />

                <ChannelBillingSection
                  title={t('channel_edit.sectionBilling')}
                  inputPrompt={inputPrompt}
                  inputLabel={inputLabel}
                  customizeT={customizeT}
                  touched={touched}
                  errors={errors}
                  theme={theme}
                  values={values}
                  setFieldValue={setFieldValue}
                  handleBlur={handleBlur}
                  handleChange={handleChange}
                  isTag={isTag}
                />

                {pluginList[values.type] &&
                  Object.keys(pluginList[values.type]).map((pluginId) => {
                    const plugin = pluginList[values.type][pluginId];
                    return (
                      <CollapsibleSection key={pluginId} title={customizeT(plugin.name)} description={customizeT(plugin.description)}>
                        {Object.keys(plugin.params).map((paramId) => {
                          const param = plugin.params[paramId];
                          const name = `plugin.${pluginId}.${paramId}`;
                          return param.type === 'bool' ? (
                            <FormControl key={name} fullWidth sx={{ ...theme.typography.otherInput }}>
                              <FormControlLabel
                                key={name}
                                required
                                control={
                                  <Switch
                                    key={name}
                                    name={name}
                                    checked={values.plugin?.[pluginId]?.[paramId] || false}
                                    onChange={(event) => {
                                      setFieldValue(name, event.target.checked);
                                    }}
                                  />
                                }
                                label={t('channel_edit.isEnable')}
                              />
                              <FormHelperText id="helper-tex-channel-key-label"> {customizeT(param.description)} </FormHelperText>
                            </FormControl>
                          ) : (
                            <FormControl key={name} fullWidth sx={{ ...theme.typography.otherInput }}>
                              <TextField
                                multiline
                                key={name}
                                name={name}
                                value={values.plugin?.[pluginId]?.[paramId] || ''}
                                label={customizeT(param.name)}
                                placeholder={customizeT(param.description)}
                                onChange={handleChange}
                              />
                              <FormHelperText id="helper-tex-channel-key-label"> {customizeT(param.description)} </FormHelperText>
                            </FormControl>
                          );
                        })}
                      </CollapsibleSection>
                    );
                  })}
                <DialogActions>
                  <Button onClick={onCancel}>{t('common.cancel')}</Button>
                  {!channelId && !isTag && (
                    <Button
                      disabled={draftProbeWorking || Boolean(providerCatalogError)}
                      onClick={() => probeDraftConnection(values)}
                      variant="outlined"
                      startIcon={draftProbeWorking ? <Icon icon="svg-spinners:ring-resize" /> : <Icon icon="mdi:connection" />}
                    >
                      连接探测
                    </Button>
                  )}
                  <Button
                    disableElevation
                    disabled={isSubmitting || (!isTag && Boolean(providerCatalogError))}
                    type="submit"
                    variant="contained"
                    color="primary"
                  >
                    {!channelId && !isTag && draftProbeSignature !== draftProbeFingerprint(values) ? '保存为未验证' : t('common.submit')}
                  </Button>
                </DialogActions>
              </form>
            );
          }}
        </Formik>

        {/* 模型选择器弹窗 */}
        <ModelSelectorModal
          open={modelSelectorOpen}
          onClose={() => setModelSelectorOpen(false)}
          onConfirm={(selectedModels, mappings, overwriteModels, overwriteMappings) => {
            // 处理普通模型选择
            handleModelSelectorConfirm(selectedModels, overwriteModels);

            // 处理映射关系
            if (mappings && mappings.length > 0) {
              if (overwriteMappings) {
                // 覆盖映射模式：清空现有映射，使用新的
                tempSetFieldValue('model_mapping', mappings);
              } else {
                // 追加映射模式：
                const existingMappings = tempFormikValues?.model_mapping || [];
                const existingKeys = new Set(existingMappings.map((item) => item.key));
                const newMappings = mappings.filter((item) => !existingKeys.has(item.key));
                const mergedMappings = [...existingMappings, ...newMappings].map((item, index) => ({
                  ...item,
                  index
                }));
                tempSetFieldValue('model_mapping', mergedMappings);
              }
            }
          }}
          channelValues={tempFormikValues}
          prices={prices}
        />

        {/* 分组编辑保存二次确认：复述将覆盖整组 N 个渠道 */}
        <ConfirmDialog
          open={saveConfirmOpen}
          onClose={() => {
            setSaveConfirmOpen(false);
            const pending = pendingSubmitRef.current;
            pendingSubmitRef.current = null;
            // 取消时复位 Formik 的提交态，避免按钮一直禁用
            pending?.helpers?.setSubmitting(false);
          }}
          title={t('channel_edit.tagGroupSaveConfirmTitle')}
          content={t('channel_edit.tagGroupSaveConfirmContent', { count: tagGroupCount, tag: channelId })}
          action={
            <Button
              variant="contained"
              color="warning"
              onClick={() => {
                setSaveConfirmOpen(false);
                const pending = pendingSubmitRef.current;
                pendingSubmitRef.current = null;
                if (pending) {
                  doSubmit(pending.values, pending.helpers);
                }
              }}
            >
              {t('common.submit')}
            </Button>
          }
        />
      </DialogContent>
    </Dialog>
  );
};

export default EditModal;

EditModal.propTypes = {
  open: PropTypes.bool,
  channelId: PropTypes.oneOfType([PropTypes.number, PropTypes.string]),
  onCancel: PropTypes.func,
  onOk: PropTypes.func,
  groupOptions: PropTypes.array,
  groupMap: PropTypes.object,
  isTag: PropTypes.bool,
  modelOptions: PropTypes.array,
  prices: PropTypes.array,
  tags: PropTypes.array
};
