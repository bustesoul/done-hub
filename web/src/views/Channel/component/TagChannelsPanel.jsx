import PropTypes from 'prop-types';
import {
  Box,
  Button,
  Checkbox,
  CircularProgress,
  Grid,
  IconButton,
  LinearProgress,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TablePagination,
  TableRow,
  Tooltip,
  Typography
} from '@mui/material';
import { Icon } from '@iconify/react';
import { useTranslation } from 'react-i18next';
import { CHANNEL_OPTIONS } from 'constants/ChannelConstants';
import { PAGE_SIZE_OPTIONS } from 'constants';
import { renderQuota } from 'utils/common';
import Label from 'ui-component/Label';
import KeywordTableHead from 'ui-component/TableHead';
import { stickyCellSx } from 'ui-component/stickyCellSx';
import GroupLabel from './GroupLabel';
import ResponseTimeLabel from './ResponseTimeLabel';
import {
  CredentialStatusCell,
  GroupInlineEditor,
  SubscriptionQuotaCell,
  renderBalance,
  statusInfo
} from './ChannelTableCells';

const TagChannelsPanel = ({
  tagChannels,
  totalTagChannels,
  loading,
  selectedChannels,
  page,
  rowsPerPage,
  order,
  orderBy,
  groupMap,
  tagChannelOriginals,
  manageChannel,
  onRefresh,
  onBatchDelete,
  onToggleAll,
  onToggleChannel,
  onSort,
  onPageChange,
  onRowsPerPageChange,
  onStatusChange,
  onPriorityChange,
  onPriorityCommit,
  onWeightChange,
  onWeightCommit,
  onCostRatioChange,
  onCostRatioCommit,
  onTestChannel,
  onEditChannel,
  onOpenActions
}) => {
  const { t } = useTranslation();
  const isTagChannelsLoading = loading;
  const tagPage = page;
  const tagRowsPerPage = rowsPerPage;
  const tagOrder = order;
  const tagOrderBy = orderBy;
  const fetchTagChannels = onRefresh;
  const handleBatchDelete = onBatchDelete;
  const handleToggleAll = onToggleAll;
  const handleToggleChannel = onToggleChannel;
  const handleTagSort = onSort;
  const handleChangeTagPage = onPageChange;
  const handleChangeTagRowsPerPage = onRowsPerPageChange;
  const handleTagChannelStatus = onStatusChange;
  const handleTagChannelPriorityChange = onPriorityChange;
  const commitTagChannelPriority = onPriorityCommit;
  const handleTagChannelWeightChange = onWeightChange;
  const commitTagChannelWeight = onWeightCommit;
  const handleTagChannelCostRatioChange = onCostRatioChange;
  const commitTagChannelCostRatio = onCostRatioCommit;

  return (
                  <Grid item xs={12}>
                    {/* 面板已被 sticky 钉在视口内，子表格在此独立横向滚动；不再加横向 margin，
                      让子表格右缘与面板右缘（即主表格「操作」列冻结右缘）对齐 */}
                    <Box sx={{ mt: 2, mb: 1 }}>
                      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
                        <Stack direction="row" alignItems="center" spacing={1}>
                          <Typography
                            variant="subtitle1"
                            fontWeight="bold"
                            sx={{
                              borderLeft: '3px solid',
                              borderColor: 'primary.main',
                              pl: 1.5,
                              py: 0.5
                            }}
                          >
                            {t('channel_row.tagChannelList')} ({totalTagChannels})
                          </Typography>
                          <Tooltip title={t('channel_row.refreshList')} placement="top">
                            <IconButton size="small" color="primary" disabled={isTagChannelsLoading} onClick={() => fetchTagChannels()}>
                              <Icon icon="mdi:refresh" width={18} height={18} />
                            </IconButton>
                          </Tooltip>
                        </Stack>

                        {selectedChannels.length > 0 && (
                          <Button
                            variant="contained"
                            color="error"
                            startIcon={<Icon icon="solar:trash-bin-trash-bold" />}
                            onClick={handleBatchDelete}
                            size="small"
                          >
                            {t('channel_row.batchDelete')} ({selectedChannels.length})
                          </Button>
                        )}
                      </Stack>

                      {tagChannels.length === 0 && isTagChannelsLoading ? (
                        <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                          <CircularProgress size={24} />
                        </Box>
                      ) : tagChannels.length === 0 ? (
                        <Typography variant="body2" sx={{ py: 2, textAlign: 'center', color: 'text.secondary' }}>
                          {t('channel_row.noTagChannels')}
                        </Typography>
                      ) : (
                        <Box>
                          {/* 刷新/排序时保持表格挂载，仅在顶部叠加进度条，避免内容被替换为居中 spinner 导致的高度塌陷与「下滑闪烁」 */}
                          <Box
                            sx={{
                              position: 'relative',
                              border: '1px solid',
                              borderColor: 'divider',
                              borderRadius: 1,
                              overflow: 'hidden',
                              boxShadow: '0 0 8px rgba(0,0,0,0.05)'
                            }}
                          >
                            {isTagChannelsLoading && (
                              <LinearProgress sx={{ position: 'absolute', top: 0, left: 0, right: 0, zIndex: 3, height: 2 }} />
                            )}
                            {/* 用原生滚动的 TableContainer（与主表格一致），保证「操作」列 position:sticky 冻结生效；
                              PerfectScrollbar 的 overflow:hidden + JS 滚动会让 sticky 失效，导致操作列无法冻结 */}
                            <TableContainer sx={{ maxHeight: 400 }}>
                              <Table size="small" sx={{ minWidth: 1180, '& .MuiTableCell-root': { py: 1, px: 1.5 } }}>
                                <KeywordTableHead
                                  order={tagOrder}
                                  orderBy={tagOrderBy}
                                  onRequestSort={handleTagSort}
                                  numSelected={selectedChannels.length}
                                  rowCount={tagChannels.length}
                                  onSelectAllClick={handleToggleAll}
                                  headLabel={[
                                    { id: 'select', label: '', align: 'center', disableSort: true, width: '40px' },
                                    { id: 'id', label: 'ID', align: 'center', width: '70px' },
                                    { id: 'name', label: t('channel_index.name'), align: 'center', minWidth: 150 },
                                    { id: 'group', label: t('channel_index.group'), align: 'center', disableSort: true, minWidth: 110 },
                                    { id: 'type', label: t('channel_index.type'), align: 'center', minWidth: 100 },
                                    { id: 'status', label: t('channel_index.status'), align: 'center', minWidth: 110 },
                                    { id: 'credential', label: '凭据', align: 'center', disableSort: true, minWidth: 110 },
                                    {
                                      id: 'subscription_quota',
                                      label: t('channel_index.subscriptionQuota'),
                                      align: 'center',
                                      disableSort: true,
                                      minWidth: 140
                                    },
                                    { id: 'used_quota', label: t('channel_index.usedBalance'), align: 'center', minWidth: 120 },
                                    { id: 'response_time', label: t('channel_index.responseTime'), align: 'center', minWidth: 110 },
                                    { id: 'priority', label: t('channel_index.priority'), align: 'center', minWidth: 100 },
                                    { id: 'weight', label: t('channel_index.weight'), align: 'center', minWidth: 100 },
                                    { id: 'cost_ratio', label: t('channel_index.costRatio'), align: 'center', minWidth: 100 },
                                    {
                                      id: 'action',
                                      label: t('channel_index.actions'),
                                      align: 'center',
                                      disableSort: true,
                                      sticky: true,
                                      minWidth: 130
                                    }
                                  ]}
                                />
                                <TableBody>
                                  {tagChannels.map((channel) => (
                                    <TableRow key={channel.id} hover>
                                      <TableCell padding="checkbox" sx={{ pl: 1, textAlign: 'center' }}>
                                        <Checkbox
                                          checked={selectedChannels.includes(channel.id)}
                                          onChange={() => handleToggleChannel(channel.id)}
                                          size="small"
                                        />
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Typography variant="body2">{channel.id}</Typography>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Typography variant="body2" noWrap title={channel.name} sx={{ fontWeight: 500 }}>
                                          {channel.name}
                                        </Typography>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                                          <GroupLabel group={channel.group ?? ''} groupMap={groupMap} />
                                        </Box>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        {CHANNEL_OPTIONS[channel.type] ? (
                                          <Label color={CHANNEL_OPTIONS[channel.type].color} variant="outlined">
                                            {CHANNEL_OPTIONS[channel.type].text}
                                          </Label>
                                        ) : (
                                          <Label color="error" variant="outlined">
                                            {t('common.unknown')}
                                          </Label>
                                        )}
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Stack direction="row" alignItems="center" spacing={0.5} justifyContent="center">
                                          <Switch
                                            checked={channel.status === 1}
                                            onChange={() => handleTagChannelStatus(channel.id, channel.status)}
                                            size="small"
                                          />
                                          <Typography
                                            variant="caption"
                                            sx={{
                                              fontWeight: channel.status === 1 ? 600 : 400,
                                              color: channel.status === 1 ? 'success.main' : 'text.secondary'
                                            }}
                                          >
                                            {statusInfo(t, channel.status)}
                                            {/* {CHANNEL_STATUS_MAP[channel.status]?.label || '未知'} */}
                                          </Typography>
                                        </Stack>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <CredentialStatusCell item={channel} />
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <SubscriptionQuotaCell channelId={channel.id} channelType={channel.type} />
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Tooltip title={t('channel_row.clickUpdateQuota')} placement="top">
                                          <Box sx={{ cursor: 'pointer' }} onClick={() => manageChannel(channel.id, 'update_balance')}>
                                            <Stack direction="column" spacing={0.5} alignItems="center" justifyContent="center">
                                              <Typography
                                                variant="body2"
                                                sx={{
                                                  fontSize: '0.8rem',
                                                  fontWeight: 500,
                                                  '&:hover': { textDecoration: 'underline' }
                                                }}
                                              >
                                                {renderQuota(channel.used_quota)}
                                              </Typography>
                                              <Typography
                                                variant="caption"
                                                sx={{
                                                  color: 'success.main',
                                                  fontWeight: 600
                                                }}
                                              >
                                                {renderBalance(channel.type, channel.balance)}
                                              </Typography>
                                            </Stack>
                                          </Box>
                                        </Tooltip>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <ResponseTimeLabel test_time={channel.test_time} response_time={channel.response_time} />
                                      </TableCell>

                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                                          <GroupInlineEditor
                                            label={t('channel_index.priority')}
                                            value={channel.priority ?? 0}
                                            min="0"
                                            onChange={(v) => handleTagChannelPriorityChange(channel.id, v)}
                                            onCommit={() => commitTagChannelPriority(channel)}
                                            disabled={
                                              channel.priority ===
                                              (tagChannelOriginals[channel.id]?.priority ?? channel.priority)
                                            }
                                          />
                                        </Box>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                                          <GroupInlineEditor
                                            label={t('channel_index.weight')}
                                            value={channel.weight ?? 1}
                                            min="1"
                                            onChange={(v) => handleTagChannelWeightChange(channel.id, v)}
                                            onCommit={() => commitTagChannelWeight(channel)}
                                            disabled={
                                              channel.weight === (tagChannelOriginals[channel.id]?.weight ?? channel.weight)
                                            }
                                          />
                                        </Box>
                                      </TableCell>
                                      <TableCell sx={{ textAlign: 'center' }}>
                                        <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                                          <GroupInlineEditor
                                            label={t('channel_index.costRatio')}
                                            value={channel.cost_ratio ?? 0}
                                            min="0"
                                            step="0.1"
                                            onChange={(v) => handleTagChannelCostRatioChange(channel.id, v)}
                                            onCommit={() => commitTagChannelCostRatio(channel)}
                                            disabled={
                                              channel.cost_ratio ===
                                              (tagChannelOriginals[channel.id]?.cost_ratio ?? channel.cost_ratio)
                                            }
                                          />
                                        </Box>
                                      </TableCell>
                                      <TableCell align="center" sx={stickyCellSx}>
                                        <Stack direction="row" spacing={1} justifyContent="center">
                                          <Tooltip title={t('channel_row.testModels')} placement="top">
                                            <IconButton
                                              size="small"
                                              sx={{ p: 0.5, color: 'info.main' }}
                                              onClick={(event) => onTestChannel(channel, event)}
                                            >
                                              <Icon icon="mdi:speedometer" width={18} height={18} />
                                            </IconButton>
                                          </Tooltip>

                                          <Tooltip title={t('common.edit')} placement="top">
                                            <IconButton
                                              size="small"
                                              sx={{ p: 0.5, color: 'primary.main' }}
                                              onClick={() => onEditChannel(channel.id)}
                                            >
                                              <Icon icon="solar:pen-bold" width={18} height={18} />
                                            </IconButton>
                                          </Tooltip>

                                          <Tooltip title={t('channel_index.actions')} placement="top">
                                            <IconButton
                                              size="small"
                                              sx={{ p: 0.5 }}
                                              onClick={(event) => onOpenActions(channel, event)}
                                            >
                                              <Icon icon="eva:more-vertical-fill" width={18} height={18} />
                                            </IconButton>
                                          </Tooltip>
                                        </Stack>
                                      </TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </TableContainer>
                          </Box>
                          <Box sx={{ display: 'flex', justifyContent: 'flex-end', pt: 1 }}>
                            <TablePagination
                              component="div"
                              count={totalTagChannels}
                              page={tagPage}
                              onPageChange={handleChangeTagPage}
                              rowsPerPage={tagRowsPerPage}
                              onRowsPerPageChange={handleChangeTagRowsPerPage}
                              rowsPerPageOptions={PAGE_SIZE_OPTIONS}
                              labelRowsPerPage={t('channel_row.rowsPerPage')}
                              labelDisplayedRows={({ from, to, count }) => t('channel_row.paginationDisplayedRows', { from, to, count })}
                              sx={{
                                '.MuiTablePagination-toolbar': {
                                  minHeight: '40px',
                                  pl: 1
                                },
                                '.MuiTablePagination-selectLabel, .MuiTablePagination-displayedRows': {
                                  fontSize: '0.75rem'
                                },
                                '.MuiTablePagination-select': {
                                  padding: '0 8px'
                                }
                              }}
                            />
                          </Box>
                        </Box>
                      )}
                    </Box>
                  </Grid>

  );
};

TagChannelsPanel.propTypes = {
  tagChannels: PropTypes.array.isRequired,
  totalTagChannels: PropTypes.number.isRequired,
  loading: PropTypes.bool,
  selectedChannels: PropTypes.array.isRequired,
  page: PropTypes.number.isRequired,
  rowsPerPage: PropTypes.number.isRequired,
  order: PropTypes.string.isRequired,
  orderBy: PropTypes.string,
  groupMap: PropTypes.object.isRequired,
  tagChannelOriginals: PropTypes.object.isRequired,
  manageChannel: PropTypes.func.isRequired,
  onRefresh: PropTypes.func.isRequired,
  onBatchDelete: PropTypes.func.isRequired,
  onToggleAll: PropTypes.func.isRequired,
  onToggleChannel: PropTypes.func.isRequired,
  onSort: PropTypes.func.isRequired,
  onPageChange: PropTypes.func.isRequired,
  onRowsPerPageChange: PropTypes.func.isRequired,
  onStatusChange: PropTypes.func.isRequired,
  onPriorityChange: PropTypes.func.isRequired,
  onPriorityCommit: PropTypes.func.isRequired,
  onWeightChange: PropTypes.func.isRequired,
  onWeightCommit: PropTypes.func.isRequired,
  onCostRatioChange: PropTypes.func.isRequired,
  onCostRatioCommit: PropTypes.func.isRequired,
  onTestChannel: PropTypes.func.isRequired,
  onEditChannel: PropTypes.func.isRequired,
  onOpenActions: PropTypes.func.isRequired
};

export default TagChannelsPanel;
