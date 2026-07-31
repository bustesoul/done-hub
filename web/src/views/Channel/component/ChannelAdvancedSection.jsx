import PropTypes from 'prop-types';
import { Box, FormControl, FormHelperText, InputLabel, OutlinedInput, TextField } from '@mui/material';
import Editor from '@monaco-editor/react';
import MapInput from './MapInput';
import ListInput from './ListInput';
import CollapsibleSection from './CollapsibleSection';

const ChannelAdvancedSection = ({
  title,
  inputPrompt,
  inputLabel,
  customizeT,
  touched,
  errors,
  theme,
  values,
  setFieldValue,
  handleBlur,
  handleChange,
  isTag,
  syncModelMappingToModels
}) => (
    <CollapsibleSection title={title}>
                  {inputPrompt.model_mapping && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.model_mapping && errors.model_mapping)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <MapInput
                        mapValue={values.model_mapping}
                        onChange={(newValue) => {
                          setFieldValue('model_mapping', newValue);
                          // 实时同步模型重定向到模型配置
                          syncModelMappingToModels(newValue, values.models, setFieldValue);
                        }}
                        error={Boolean(touched.model_mapping && errors.model_mapping)}
                        label={{
                          keyName: customizeT(inputLabel.model_mapping),
                          valueName: customizeT(inputPrompt.model_mapping),
                          name: customizeT(inputLabel.model_mapping)
                        }}
                      />
                      {touched.model_mapping && errors.model_mapping ? (
                        <FormHelperText error id="helper-tex-channel-model_mapping-label">
                          {errors.model_mapping}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-model_mapping-label">{customizeT(inputPrompt.model_mapping)}</FormHelperText>
                      )}
                    </FormControl>
                  )}

                  <FormControl fullWidth error={Boolean(touched.proxy && errors.proxy)} sx={{ ...theme.typography.otherInput }}>
                    <InputLabel htmlFor="channel-proxy-label">{customizeT(inputLabel.proxy)}</InputLabel>
                    <OutlinedInput
                      id="channel-proxy-label"
                      label={customizeT(inputLabel.proxy)}
                      type="text"
                      value={values.proxy}
                      name="proxy"
                      onBlur={handleBlur}
                      onChange={handleChange}
                      inputProps={{}}
                      aria-describedby="helper-text-channel-proxy-label"
                    />
                    {touched.proxy && errors.proxy ? (
                      <FormHelperText error id="helper-tex-channel-proxy-label">
                        {errors.proxy}
                      </FormHelperText>
                    ) : (
                      <FormHelperText id="helper-tex-channel-proxy-label"> {customizeT(inputPrompt.proxy)} </FormHelperText>
                    )}
                  </FormControl>
                  {inputPrompt.test_model && (
                    <FormControl fullWidth error={Boolean(touched.test_model && errors.test_model)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-test_model-label">{customizeT(inputLabel.test_model)}</InputLabel>
                      <OutlinedInput
                        id="channel-test_model-label"
                        label={customizeT(inputLabel.test_model)}
                        type="text"
                        value={values.test_model}
                        name="test_model"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        inputProps={{}}
                        aria-describedby="helper-text-channel-test_model-label"
                      />
                      {touched.test_model && errors.test_model ? (
                        <FormHelperText error id="helper-tex-channel-test_model-label">
                          {errors.test_model}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-test_model-label"> {customizeT(inputPrompt.test_model)} </FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {inputPrompt.model_headers && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.model_headers && errors.model_headers)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <MapInput
                        mapValue={values.model_headers}
                        onChange={(newValue) => {
                          setFieldValue('model_headers', newValue);
                        }}
                        enableSkip
                        error={Boolean(touched.model_headers && errors.model_headers)}
                        label={{
                          keyName: customizeT(inputLabel.model_headers),
                          valueName: customizeT(inputPrompt.model_headers),
                          name: customizeT(inputLabel.model_headers)
                        }}
                      />
                      {touched.model_headers && errors.model_headers ? (
                        <FormHelperText error id="helper-tex-channel-model_headers-label">
                          {errors.model_headers}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-model_headers-label">{customizeT(inputPrompt.model_headers)}</FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {inputPrompt.header_override && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.header_override && errors.header_override)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <MapInput
                        mapValue={values.header_override}
                        onChange={(newValue) => {
                          setFieldValue('header_override', newValue);
                        }}
                        error={Boolean(touched.header_override && errors.header_override)}
                        label={{
                          keyName: customizeT(inputLabel.header_override),
                          valueName: customizeT(inputPrompt.header_override),
                          name: customizeT(inputLabel.header_override)
                        }}
                      />
                      {touched.header_override && errors.header_override ? (
                        <FormHelperText error id="helper-tex-channel-header_override-label">
                          {errors.header_override}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-header_override-label">
                          {customizeT(inputPrompt.header_override)}
                        </FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {inputPrompt.custom_parameter && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.custom_parameter && errors.custom_parameter)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <InputLabel shrink htmlFor="channel-custom_parameter-label">
                        {customizeT(inputLabel.custom_parameter)}
                      </InputLabel>
                      <Box
                        sx={{
                          border: '1px solid',
                          borderColor: touched.custom_parameter && errors.custom_parameter ? 'error.main' : 'divider',
                          borderRadius: 1,
                          overflow: 'hidden',
                          marginTop: 2, // Add some margin for the label
                          resize: 'vertical',
                          height: '200px',
                          minHeight: '100px',
                          '&:hover': {
                            borderColor: 'primary.main'
                          },
                          '&:focus-within': {
                            borderColor: 'primary.main',
                            borderWidth: 2
                          }
                        }}
                      >
                        <Editor
                          height="100%"
                          language="json"
                          theme={theme.palette.mode === 'dark' ? 'vs-dark' : 'light'}
                          value={values.custom_parameter}
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
                          onChange={(value) => {
                            setFieldValue('custom_parameter', value);
                          }}
                        />
                      </Box>
                      {touched.custom_parameter && errors.custom_parameter ? (
                        <FormHelperText error id="helper-tex-channel-custom_parameter-label">
                          {errors.custom_parameter}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-custom_parameter-label">
                          {customizeT(inputPrompt.custom_parameter)}
                        </FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {inputPrompt.need2response_models && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.need2response_models && errors.need2response_models)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <TextField
                        multiline
                        minRows={6}
                        id="channel-need2response_models-label"
                        name="need2response_models"
                        label={customizeT(inputLabel.need2response_models)}
                        value={values.need2response_models || ''}
                        onBlur={handleBlur}
                        onChange={handleChange}
                        disabled={isTag}
                      />
                      {touched.need2response_models && errors.need2response_models ? (
                        <FormHelperText error id="helper-tex-channel-need2response_models-label">
                          {errors.need2response_models}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-need2response_models-label">
                          {customizeT(inputPrompt.need2response_models)}
                        </FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {inputPrompt.disabled_stream && (
                    <FormControl
                      fullWidth
                      error={Boolean(touched.disabled_stream && errors.disabled_stream)}
                      sx={{ ...theme.typography.otherInput }}
                    >
                      <ListInput
                        listValue={values.disabled_stream}
                        onChange={(newValue) => {
                          setFieldValue('disabled_stream', newValue);
                        }}
                        error={Boolean(touched.disabled_stream && errors.disabled_stream)}
                        label={{
                          name: customizeT(inputLabel.disabled_stream),
                          itemName: customizeT(inputPrompt.disabled_stream)
                        }}
                      />
                    </FormControl>
                  )}
                </CollapsibleSection>
);

ChannelAdvancedSection.propTypes = {
  title: PropTypes.string.isRequired,
  inputPrompt: PropTypes.object.isRequired,
  inputLabel: PropTypes.object.isRequired,
  customizeT: PropTypes.func.isRequired,
  touched: PropTypes.object.isRequired,
  errors: PropTypes.object.isRequired,
  theme: PropTypes.object.isRequired,
  values: PropTypes.object.isRequired,
  setFieldValue: PropTypes.func.isRequired,
  handleBlur: PropTypes.func.isRequired,
  handleChange: PropTypes.func.isRequired,
  isTag: PropTypes.bool,
  syncModelMappingToModels: PropTypes.func.isRequired
};

export default ChannelAdvancedSection;
