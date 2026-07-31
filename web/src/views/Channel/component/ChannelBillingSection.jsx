import PropTypes from 'prop-types';
import { FormControl, FormControlLabel, FormHelperText, InputLabel, MenuItem, OutlinedInput, Select, Switch } from '@mui/material';
import { PreCostType } from '../type/other';
import CollapsibleSection from './CollapsibleSection';

const ChannelBillingSection = ({
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
  isTag
}) => (
    <CollapsibleSection title={title}>
                  {inputPrompt.only_chat && (
                    <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={Boolean(values.only_chat)}
                            onChange={(event) => {
                              setFieldValue('only_chat', event.target.checked);
                            }}
                          />
                        }
                        label={customizeT(inputLabel.only_chat)}
                      />
                      <FormHelperText id="helper-tex-only_chat_model-label"> {customizeT(inputPrompt.only_chat)} </FormHelperText>
                    </FormControl>
                  )}
                  {inputPrompt.compatible_response && (
                    <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={Boolean(values.compatible_response)}
                            onChange={(event) => {
                              setFieldValue('compatible_response', event.target.checked);
                            }}
                          />
                        }
                        label={customizeT(inputLabel.compatible_response)}
                      />
                      <FormHelperText id="helper-tex-compatible_response-label">
                        {customizeT(inputPrompt.compatible_response)}
                      </FormHelperText>
                    </FormControl>
                  )}
                  {inputPrompt.allow_extra_body && (
                    <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={Boolean(values.allow_extra_body)}
                            onChange={(event) => {
                              setFieldValue('allow_extra_body', event.target.checked);
                            }}
                          />
                        }
                        label={customizeT(inputLabel.allow_extra_body)}
                      />
                      <FormHelperText id="helper-tex-allow_extra_body-label">{customizeT(inputPrompt.allow_extra_body)}</FormHelperText>
                    </FormControl>
                  )}
                  {inputPrompt.pass_through_body && (
                    <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={Boolean(values.pass_through_body)}
                            onChange={(event) => {
                              setFieldValue('pass_through_body', event.target.checked);
                            }}
                          />
                        }
                        label={customizeT(inputLabel.pass_through_body)}
                      />
                      <FormHelperText id="helper-tex-pass_through_body-label">{customizeT(inputPrompt.pass_through_body)}</FormHelperText>
                    </FormControl>
                  )}
                  {inputPrompt.pre_cost && (
                    <FormControl fullWidth error={Boolean(touched.pre_cost && errors.pre_cost)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-pre_cost-label">{customizeT(inputLabel.pre_cost)}</InputLabel>
                      <Select
                        id="channel-pre_cost-label"
                        label={customizeT(inputLabel.pre_cost)}
                        value={values.pre_cost}
                        name="pre_cost"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        MenuProps={{
                          PaperProps: {
                            style: {
                              maxHeight: 200
                            }
                          }
                        }}
                      >
                        {PreCostType.map((option) => {
                          return (
                            <MenuItem key={option.value} value={option.value}>
                              {option.label}
                            </MenuItem>
                          );
                        })}
                      </Select>
                      {touched.pre_cost && errors.pre_cost ? (
                        <FormHelperText error id="helper-tex-channel-pre_cost-label">
                          {errors.pre_cost}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-pre_cost-label"> {customizeT(inputPrompt.pre_cost)} </FormHelperText>
                      )}
                    </FormControl>
                  )}
                  {!isTag && inputPrompt.cost_ratio && (
                    <FormControl fullWidth error={Boolean(touched.cost_ratio && errors.cost_ratio)} sx={{ ...theme.typography.otherInput }}>
                      <InputLabel htmlFor="channel-cost_ratio-label">{customizeT(inputLabel.cost_ratio)}</InputLabel>
                      <OutlinedInput
                        id="channel-cost_ratio-label"
                        label={customizeT(inputLabel.cost_ratio)}
                        type="number"
                        value={values.cost_ratio}
                        name="cost_ratio"
                        onBlur={handleBlur}
                        onChange={handleChange}
                        inputProps={{ step: 0.1, min: 0 }}
                        aria-describedby="helper-text-channel-cost_ratio-label"
                      />
                      {touched.cost_ratio && errors.cost_ratio ? (
                        <FormHelperText error id="helper-tex-channel-cost_ratio-label">
                          {errors.cost_ratio}
                        </FormHelperText>
                      ) : (
                        <FormHelperText id="helper-tex-channel-cost_ratio-label"> {customizeT(inputPrompt.cost_ratio)} </FormHelperText>
                      )}
                    </FormControl>
                  )}
                </CollapsibleSection>
);

ChannelBillingSection.propTypes = {
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
  isTag: PropTypes.bool
};

export default ChannelBillingSection;
