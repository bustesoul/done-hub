import PropTypes from 'prop-types';
import { useMemo, useState } from 'react';
import { ArrowForward } from '@mui/icons-material';

import Badge from '@mui/material/Badge'
import { Box, Collapse, Stack, TableCell, TableRow, Tooltip, Typography } from '@mui/material'

import { renderQuota, timestamp2string } from 'utils/common'
import Label from 'ui-component/Label'
import { useLogType } from '../type/LogType'
import { useTranslation } from 'react-i18next'
import QuotaWithDetailRow from './QuotaWithDetailRow'
import QuotaWithDetailContent, { calculatePrice } from './QuotaWithDetailContent'
import { calculateTokenBilling, formatTokenCount } from './tokenBilling'
import { styled } from '@mui/material/styles'
import { stickyCellSx } from 'ui-component/stickyCellSx';

function renderType(type, logTypes, t) {
  const typeOption = logTypes[type]
  if (typeOption) {
    return (
      <Label variant="filled" color={typeOption.color}>
        {' '}
        {typeOption.text}{' '}
      </Label>
    )
  } else {
    return (
      <Label variant="filled" color="error">
        {' '}
        {t('logPage.unknown')}{' '}
      </Label>
    )
  }
}

function requestTimeLabelOptions(request_time) {
  let color = 'error'
  if (request_time === 0) {
    color = 'default'
  } else if (request_time <= 10) {
    color = 'success'
  } else if (request_time <= 50) {
    color = 'primary'
  } else if (request_time <= 100) {
    color = 'secondary'
  }

  return color
}

function requestTSLabelOptions(request_ts) {
  let color = 'success'
  if (request_ts === 0) {
    color = 'default'
  } else if (request_ts <= 10) {
    color = 'error'
  } else if (request_ts <= 15) {
    color = 'secondary'
  } else if (request_ts <= 20) {
    color = 'primary'
  }

  return color
}

export default function LogTableRow({ item, userIsAdmin, userGroup, columnVisibility }) {
  const { t } = useTranslation()
  const LogType = useLogType()
  let request_time = item.request_time / 1000
  let request_time_str = request_time.toFixed(2) + ' S'

  let first_time = item.metadata?.first_response ? item.metadata.first_response / 1000 : 0
  let first_time_str = first_time ? `${first_time.toFixed(2)} S` : ''

  const stream_time = request_time - first_time

  let request_ts = 0
  let request_ts_str = ''
  if (first_time > 0 && item.completion_tokens > 0) {
    // Using the completion_tokens directly since we already checked it's > 0
    request_ts = item.completion_tokens / stream_time
    request_ts_str = `${request_ts.toFixed(2)} t/s`
  }

  const tokenBilling = useMemo(() => calculateTokenBilling(item), [item])

  // 计算当前显示的列数
  const colCount = Object.values(columnVisibility).filter(Boolean).length

  // 展开状态（仅type=2时才有展开）
  const [open, setOpen] = useState(false)
  const showExpand = item.type === 2 && columnVisibility.quota

  return (
    <>
      <TableRow tabIndex={item.id}>
        {columnVisibility.created_at &&
          <TableCell sx={{
            p: '10px 8px',
            whiteSpace: 'nowrap',
            textAlign: 'center'
          }}>{timestamp2string(item.created_at)}</TableCell>}

        {userIsAdmin && columnVisibility.channel_id && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center', whiteSpace: 'nowrap' }}>
            {(item.channel_id || '') + ' ' + (item.channel?.name ? '(' + item.channel.name + ')' : '')}
          </TableCell>
        )}
        {userIsAdmin && columnVisibility.user_id && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>
            <Label color="default" variant="outlined" copyText={item.username}>
              {item.username}
            </Label>
          </TableCell>
        )}

        {columnVisibility.group && (
          <TableCell sx={{ p: '10px 8px' }}>
            {item?.metadata?.is_backup_group ? (
              // 显示分组重定向：原始分组 → 备份分组
              <Stack direction="row" spacing={1} alignItems="center">
                <Label color="default" variant="soft">
                  {userGroup[item.metadata.group_name]?.name || '跟随用户'}
                </Label>
                <ArrowForward sx={{ fontSize: 16, color: 'text.secondary' }} />
                <Label color="warning" variant="soft">
                  {userGroup[item.metadata.backup_group_name]?.name || '备份分组'}
                </Label>
              </Stack>
            ) : (
              // 正常显示分组
              item?.metadata?.group_name || item?.metadata?.backup_group_name ? (
                <Label color="default" variant="soft">
                  {userGroup[item.metadata.group_name || item.metadata.backup_group_name]?.name || '跟随用户'}
                </Label>
              ) : (
                ''
              )
            )}
          </TableCell>
        )}
        {columnVisibility.token_name && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>
            {item.token_name && (
              <Label color="default" variant="soft" copyText={item.token_name}>
                {item.token_name}
              </Label>
            )}
          </TableCell>
        )}
        {columnVisibility.type &&
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>{renderType(item.type, LogType, t)}</TableCell>}
        {columnVisibility.model_name &&
          <TableCell
            sx={{ p: '10px 8px', textAlign: 'center' }}>{viewModelName(item.model_name, item.is_stream, item.metadata?.service_tier)}</TableCell>}

        {columnVisibility.reasoning_effort && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center', whiteSpace: 'nowrap' }}>
            {item.metadata?.reasoning_effort ? (
              <Label color="default" variant="soft" copyText={item.metadata.reasoning_effort}>
                {item.metadata.reasoning_effort}
              </Label>
            ) : (
              <Typography component="span" color="text.secondary">—</Typography>
            )}
          </TableCell>
        )}

        {columnVisibility.duration && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>
            <Stack direction="column" spacing={0.5}>
              <Label color={requestTimeLabelOptions(request_time)}>
                {item.request_time === 0 ? '无' : request_time_str} {first_time_str ? ' / ' + first_time_str : ''}
              </Label>

              {request_ts_str && <Label color={requestTSLabelOptions(request_ts)}>{request_ts_str}</Label>}
            </Stack>
          </TableCell>
        )}
        {columnVisibility.message && (
          <TableCell
            sx={{
              p: '10px 8px',
              textAlign: 'center'
            }}>{viewInput(item, t, tokenBilling)}</TableCell>
        )}
        {columnVisibility.completion && <TableCell sx={{
          p: '10px 8px',
          textAlign: 'center'
        }}>{item.completion_tokens !== undefined && item.completion_tokens !== null ? item.completion_tokens : ''}</TableCell>}
        {columnVisibility.quota && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>
            {item.type === 2 ? (
              <QuotaWithDetailRow item={item} open={open} setOpen={setOpen}/>
            ) : item.quota ? (
              renderQuota(item.quota, 6)
            ) : (
              '$0'
            )}
          </TableCell>
        )}
        {columnVisibility.source_ip &&
          <TableCell sx={{ p: '10px 8px', textAlign: 'center' }}>{item.source_ip || ''}</TableCell>}
        {columnVisibility.detail && (
          <TableCell sx={{ p: '10px 8px', textAlign: 'center', ...stickyCellSx }}>
            {item.type === 5
              ? viewErrorDetail(item, t)
              : viewLogContent(item, t, tokenBilling.billableInputTokens, tokenBilling.billableOutputTokens)}
          </TableCell>
        )}
      </TableRow>
      {/* 展开行 */}
      {showExpand && (
        <TableRow>
          <TableCell colSpan={colCount} sx={{ p: 0, border: 0, bgcolor: 'transparent' }}>
            <Collapse in={open} timeout="auto" unmountOnExit>
              <QuotaWithDetailContent
                item={item}
                userGroup={userGroup}
                userIsAdmin={userIsAdmin}
                t={t}
                tokenBilling={tokenBilling}
              />
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

LogTableRow.propTypes = {
  item: PropTypes.object,
  userIsAdmin: PropTypes.bool,
  userGroup: PropTypes.object,
  columnVisibility: PropTypes.object
}

function viewModelName(model_name, isStream, serviceTier) {
  if (!model_name) {
    return ''
  }

  const isFast = String(serviceTier || '').toLowerCase() === 'priority'

  return (
    <Box
      component="span"
      sx={{
        position: 'relative',
        display: 'inline-flex',
        pt: isStream ? '6px' : 0,
        pb: isFast ? '6px' : 0
      }}
    >
      <Label color="primary" variant="outlined" copyText={model_name}>
        {model_name}
      </Label>
      {isStream && (
        <Box
          component="span"
          sx={{
            position: 'absolute',
            top: '-3px',
            right: '-16px',
            height: '16px',
            minWidth: '16px',
            px: '4px',
            borderRadius: '8px',
            bgcolor: 'primary.main',
            color: 'primary.contrastText',
            fontSize: '0.55rem',
            lineHeight: '16px',
            whiteSpace: 'nowrap',
            pointerEvents: 'none'
          }}
        >
          Stream
        </Box>
      )}
      {isFast && (
        <Box
          component="span"
          sx={{
            position: 'absolute',
            right: '-12px',
            bottom: '-3px',
            height: '16px',
            minWidth: '16px',
            px: '4px',
            borderRadius: '8px',
            bgcolor: 'warning.main',
            color: 'warning.contrastText',
            fontSize: '0.55rem',
            lineHeight: '16px',
            whiteSpace: 'nowrap',
            pointerEvents: 'none'
          }}
        >
          Fast
        </Box>
      )}
    </Box>
  )
}

const MetadataTypography = styled(Typography)(({ theme }) => ({
  fontSize: 12,
  color: theme.palette.grey[300],
  '&:not(:last-child)': {
    marginBottom: theme.spacing(0.5)
  }
}))

function viewInput(item, t, tokenBilling) {
  const { prompt_tokens } = item
  const {
    billableInputTokens,
    billableOutputTokens,
    cacheDetails,
    hasCache,
    rawInputTokens,
    show,
    tokenDetails,
    uncachedInputTokens
  } = tokenBilling

  if (prompt_tokens === undefined || prompt_tokens === null) return ''
  if (!show) return formatTokenCount(prompt_tokens)

  const tooltipContent = tokenDetails.map(({ key, label, rawTokens, billableTokens, ratio }) => (
    <MetadataTypography
      key={key}
    >
      {t('logPage.tokenBillingAdjustment', {
        label: t(label, { ratio }),
        raw: formatTokenCount(rawTokens),
        billable: formatTokenCount(billableTokens)
      })}
    </MetadataTypography>
  ))

  const cacheSummary = cacheDetails.map(({ key, shortLabel, rawTokens, ratio, hasRatio }) => (
    <Typography
      component="span"
      key={key}
      sx={{ color: 'primary.main', fontSize: 11, fontWeight: 500, lineHeight: 1.35, whiteSpace: 'nowrap' }}
    >
      {t('logPage.cacheSummary', {
        label: t(shortLabel),
        tokens: formatTokenCount(rawTokens),
        ratio: hasRatio ? ratio : '-'
      })}
    </Typography>
  ))

  return (
    <Badge variant="dot" color="primary">
      <Tooltip
        title={
          <>
            {tooltipContent}
            <MetadataTypography>
              {t('logPage.rawInputTokens')}: {formatTokenCount(rawInputTokens)}
            </MetadataTypography>
            <MetadataTypography>
              {t('logPage.billableInputTokens')}: {formatTokenCount(billableInputTokens)}
            </MetadataTypography>
            <MetadataTypography>
              {t('logPage.totalOutputTokens')}: {formatTokenCount(billableOutputTokens)}
            </MetadataTypography>
          </>
        }
        placement="top"
        arrow
      >
        <Stack component="span" spacing={0.15} alignItems="center" sx={{ cursor: 'help' }}>
          <Typography component="span" sx={{ color: 'text.primary', fontSize: 13, fontWeight: 600, lineHeight: 1.3 }}>
            {formatTokenCount(prompt_tokens)}
          </Typography>
          {hasCache && (
            <Typography component="span" sx={{ color: 'text.secondary', fontSize: 11, lineHeight: 1.35, whiteSpace: 'nowrap' }}>
              {t('logPage.uncachedInputSummary', { tokens: formatTokenCount(uncachedInputTokens) })}
              {' · '}
              {cacheSummary.reduce((result, entry, index) => (index === 0 ? [entry] : [...result, ' · ', entry]), [])}
            </Typography>
          )}
        </Stack>
      </Tooltip>
    </Badge>
  )
}

// viewErrorDetail 渲染错误日志（type=5）的详情列：
// 主文案是简短的 item.content（[status] error_type），hover Tooltip 展示完整错误信息与重试上下文。
function viewErrorDetail(item, t) {
  const meta = item?.metadata || {}
  const isLocal = meta.error_source === 'local' || meta.local_error === true
  const title = (
    <Stack direction="column" spacing={0.5} sx={{ maxWidth: 480 }}>
      {meta.error_message && (
        <Typography variant="caption" sx={{ wordBreak: 'break-all' }}>
          {meta.error_message}
        </Typography>
      )}
      <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
        <Typography variant="caption">{t('logPage.errorDetail.source')}: {isLocal ? t('logPage.errorDetail.sourceLocal') : t('logPage.errorDetail.sourceUpstream')}</Typography>
        {meta.break_reason && <Typography variant="caption">{t('logPage.errorDetail.breakReason')}: {meta.break_reason}</Typography>}
        {meta.retry_count !== undefined && <Typography variant="caption">{t('logPage.errorDetail.retryCount')}: {meta.retry_count}</Typography>}
        {meta.attempt_count !== undefined && <Typography variant="caption">{t('logPage.errorDetail.attemptCount')}: {meta.attempt_count}</Typography>}
      </Stack>
    </Stack>
  )
  return (
    <Tooltip title={title} placement="top" arrow>
      <Stack direction="row" spacing={0.5} alignItems="center" sx={{ cursor: 'help' }}>
        <Label
          variant="soft"
          color={isLocal ? 'warning' : 'error'}
          sx={{ flexShrink: 0, height: 20, '& .MuiBox-root': { p: 0 } }}
        >
          {isLocal ? t('logPage.errorDetail.sourceLocal') : t('logPage.errorDetail.sourceUpstream')}
        </Label>
        <Typography variant="body2">{item.content || ''}</Typography>
      </Stack>
    </Tooltip>
  )
}

function viewLogContent(item, t) {
  // totalOutputTokens is passed but not used in this function
  // Check if we have the necessary data to calculate prices
  if (!item?.metadata?.input_ratio) {
    const free = (item.quota === 0 || item.quota === undefined) && item.type === 2
    return free ? (
      <Stack direction="column" spacing={0.3}>
        <Label color={free ? 'success' : 'secondary'} variant="soft">
          {t('logPage.content.free')}
        </Label>
      </Stack>
    ) : (
      <>{item.content || ''}</>
    )
  }

  // Ensure we have valid values with appropriate defaults
  const groupDiscount = item?.metadata?.group_ratio || 1
  const priceType = item?.metadata?.price_type || ''
  const originalCompletionRatio = item?.metadata?.output_ratio || 0
  const originalInputRatio = item?.metadata?.input_ratio || 0

  let inputPriceInfo
  let outputPriceInfo = ''
  if (priceType === 'times') {
    // Calculate prices for 'times' price type
    const inputPrice = calculatePrice(originalInputRatio, groupDiscount, true)

    inputPriceInfo = t('logPage.content.times_price', {
      times: inputPrice
    })
  } else {
    // Calculate prices for a standard price type
    const inputPrice = calculatePrice(originalInputRatio, groupDiscount, false)
    const outputPrice = calculatePrice(originalCompletionRatio, groupDiscount, false)

    inputPriceInfo = t('logPage.content.input_price', {
      price: inputPrice
    })
    outputPriceInfo = t('logPage.content.output_price', {
      price: outputPrice
    })
  }

  return (
    <Stack direction="column" spacing={0.3}>
      {inputPriceInfo && (
        <Label color="info" variant="soft">
          {inputPriceInfo}
        </Label>
      )}
      {outputPriceInfo && (
        <Label color="info" variant="soft">
          {outputPriceInfo}
        </Label>
      )}
    </Stack>
  )
}
