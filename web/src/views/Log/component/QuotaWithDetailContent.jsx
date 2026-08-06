import { Box, Divider, Stack, Typography } from '@mui/material';
import Decimal from 'decimal.js';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';

import { renderQuota } from 'utils/common';
import { calculateOriginalQuota } from './QuotaWithDetailRow';
import { formatTokenCount } from './tokenBilling';

export function calculatePrice(ratio, groupDiscount, isTimes) {
  ratio = ratio || 0;
  groupDiscount = groupDiscount || 0;

  let discount = new Decimal(ratio).mul(groupDiscount);
  if (!isTimes) {
    discount = discount.mul(1000);
  }

  let priceString = discount.mul(0.002).toFixed(6);
  priceString = priceString.replace(/(\.\d*?[1-9])0+$|\.0*$/, '$1');
  return priceString;
}

function parsePrice(price) {
  const match = String(price || '').replaceAll(',', '').match(/-?\d+(?:\.\d+)?/);
  return new Decimal(match?.[0] || 0);
}

function formatLineCost(tokens, unitPrice, ratio = 1) {
  return `$${new Decimal(tokens).mul(parsePrice(unitPrice)).mul(ratio).div(1_000_000).toFixed(6)}`;
}

export default function QuotaWithDetailContent({ item, userGroup, userIsAdmin, tokenBilling }) {
  const { t } = useTranslation();
  const metadata = item.metadata || {};
  const priceType = metadata.price_type || 'tokens';
  const isTokenBilling = priceType === 'tokens';
  const quota = item.quota || 0;
  const costQuota = item.cost_quota || 0;
  const showCost = userIsAdmin && costQuota > 0;
  const originalQuota = calculateOriginalQuota(item);

  const groupRatio = metadata.group_ratio || 1;
  const serviceTier = metadata.service_tier || '';
  const serviceTierRatio = metadata.service_tier_ratio || 1;
  const finalRatio = groupRatio * serviceTierRatio;
  const longContextInputRatio = metadata.long_context_input_ratio || 1;
  const longContextOutputRatio = metadata.long_context_output_ratio || 1;
  const groupKey = metadata.is_backup_group ? metadata.backup_group_name : metadata.group_name;
  const groupName = userGroup?.[groupKey]?.name || groupKey || '-';

  const inputPrice =
    metadata.input_price ||
    `$${calculatePrice((metadata.input_ratio || 0) * longContextInputRatio, finalRatio, false)}`;
  const outputPrice =
    metadata.output_price ||
    `$${calculatePrice((metadata.output_ratio || 0) * longContextOutputRatio, finalRatio, false)}`;
  const timesPrice = metadata.input_price || `$${calculatePrice(metadata.input_ratio || 0, finalRatio, true)}`;

  const {
    billableInputTokens,
    cacheDetails,
    cachedInputTokens,
    rawInputTokens,
    rawOutputTokens,
    tokenDetails,
    uncachedInputTokens
  } = tokenBilling;
  const cacheHitRate = rawInputTokens > 0 ? Math.min((cachedInputTokens / rawInputTokens) * 100, 100) : 0;
  const regularRate = 100 - cacheHitRate;

  const inputAdjustments = tokenDetails.filter(({ side, cache, adjustment }) => side === 'input' && !cache && adjustment !== 0);
  const outputAdjustments = tokenDetails.filter(({ side, adjustment }) => side === 'output' && adjustment !== 0);
  const billingRows = [];

  if (isTokenBilling) {
    const regularInputTokens = cacheDetails.length > 0 ? uncachedInputTokens : rawInputTokens;
    if (regularInputTokens > 0) {
      billingRows.push({
        key: 'regular-input',
        label: t('logPage.quotaDetail.regularInput'),
        formula: `${formatTokenCount(regularInputTokens)} × ${inputPrice} /M`,
        amount: formatLineCost(regularInputTokens, inputPrice)
      });
    }

    cacheDetails.forEach(({ key, shortLabel, rawTokens, ratio, hasRatio }) => {
      billingRows.push({
        key,
        label: t(shortLabel),
        formula: `${formatTokenCount(rawTokens)} × ${inputPrice} /M${hasRatio ? ` × ${ratio}` : ''}`,
        amount: formatLineCost(rawTokens, inputPrice, hasRatio ? ratio : 1),
        accent: true
      });
    });

    inputAdjustments.forEach(({ key, label, rawTokens, ratio, adjustment }) => {
      billingRows.push({
        key: `${key}-adjustment`,
        label: t(label, { ratio }),
        formula: `${formatTokenCount(rawTokens)} × (${ratio} − 1) = ${formatTokenCount(adjustment)}`,
        amount: formatLineCost(adjustment, inputPrice)
      });
    });

    if (rawOutputTokens > 0) {
      billingRows.push({
        key: 'output',
        label: t('logPage.quotaDetail.outputTokens'),
        formula: `${formatTokenCount(rawOutputTokens)} × ${outputPrice} /M`,
        amount: formatLineCost(rawOutputTokens, outputPrice)
      });
    }

    outputAdjustments.forEach(({ key, label, rawTokens, ratio, adjustment }) => {
      billingRows.push({
        key: `${key}-adjustment`,
        label: t(label, { ratio }),
        formula: `${formatTokenCount(rawTokens)} × (${ratio} − 1) = ${formatTokenCount(adjustment)}`,
        amount: formatLineCost(adjustment, outputPrice)
      });
    });
  } else {
    billingRows.push({
      key: 'times',
      label: t('logPage.quotaDetail.perRequestBilling'),
      formula: t('logPage.quotaDetail.oneRequest'),
      amount: timesPrice
    });
  }

  Object.entries(metadata.extra_billing || {}).forEach(([key, data]) => {
    const amount = new Decimal(data.price || 0).mul(data.call_count || 0).mul(finalRatio);
    billingRows.push({
      key: `extra-${key}`,
      label: data.type ? `${key} [${data.type}]` : key,
      formula: `$${data.price} × ${data.call_count}`,
      amount: `$${amount.toFixed(6)}`
    });
  });

  return (
    <Box
      sx={{
        mx: 2,
        my: 1.5,
        px: { xs: 2, md: 3 },
        py: 2.5,
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 2,
        bgcolor: 'background.paper'
      }}
    >
      <Box sx={{ display: 'flex', alignItems: { xs: 'flex-start', md: 'center' }, justifyContent: 'space-between', gap: 2, flexWrap: 'wrap' }}>
        <Box>
          <Typography sx={{ color: 'text.secondary', fontSize: 12, fontWeight: 600, letterSpacing: '0.04em' }}>
            {t('logPage.quotaDetail.chargeForRequest')}
          </Typography>
          <Stack direction="row" spacing={1.2} alignItems="baseline" sx={{ mt: 0.35 }}>
            <Typography sx={{ color: 'success.main', fontSize: 26, fontWeight: 750, letterSpacing: '-0.03em', fontVariantNumeric: 'tabular-nums' }}>
              {renderQuota(quota, 6)}
            </Typography>
            {finalRatio !== 1 && (
              <Typography sx={{ color: 'text.disabled', fontSize: 12, textDecoration: 'line-through' }}>
                {renderQuota(originalQuota, 6)}
              </Typography>
            )}
          </Stack>
        </Box>
        <Stack direction="row" spacing={1.1} divider={<Typography sx={{ color: 'divider' }}>·</Typography>} flexWrap="wrap" useFlexGap>
          {isTokenBilling ? (
            <>
              <ContextValue label={t('logPage.quotaDetail.inputUnitPrice')} value={`${inputPrice} /M`} />
              {cacheDetails.length > 0 && (
                <ContextValue
                  label={t('logPage.quotaDetail.cacheRatio')}
                  value={cacheDetails[0].hasRatio ? `${cacheDetails[0].ratio}×` : '-'}
                  accent
                />
              )}
              <ContextValue label={t('logPage.quotaDetail.outputUnitPrice')} value={`${outputPrice} /M`} />
            </>
          ) : (
            <ContextValue label={t('logPage.quotaDetail.perRequestBilling')} value={timesPrice} />
          )}
          <ContextValue label={t('logPage.groupLabel')} value={groupName} />
          {serviceTierRatio !== 1 && <ContextValue label={serviceTier || t('logPage.quotaDetail.serviceTierRatioValue')} value={`${serviceTierRatio}×`} />}
          {(longContextInputRatio !== 1 || longContextOutputRatio !== 1) && (
            <ContextValue label={t('logPage.quotaDetail.longContextRatio')} value={`${longContextInputRatio}× / ${longContextOutputRatio}×`} />
          )}
        </Stack>
      </Box>

      <Divider sx={{ my: 2.25 }} />

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'minmax(220px, 0.75fr) minmax(360px, 1.45fr)' }, gap: { xs: 2.5, md: 4 } }}>
        <Box>
          <Typography sx={{ color: 'text.primary', fontSize: 14, fontWeight: 700 }}>{t('logPage.quotaDetail.inputComposition')}</Typography>
          {isTokenBilling ? (
            <>
              <Stack direction="row" alignItems="baseline" justifyContent="space-between" sx={{ mt: 1.2 }}>
                <Typography sx={{ color: 'text.secondary', fontSize: 12 }}>{t('logPage.rawInputTokens')}</Typography>
                <Typography sx={{ color: 'text.primary', fontSize: 20, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
                  {formatTokenCount(rawInputTokens)}
                </Typography>
              </Stack>
              <Box sx={{ display: 'flex', height: 8, mt: 1.15, overflow: 'hidden', borderRadius: 99, bgcolor: 'action.hover' }}>
                <Box sx={{ width: `${regularRate}%`, bgcolor: 'text.disabled' }} />
                <Box sx={{ width: `${cacheHitRate}%`, bgcolor: 'primary.main' }} />
              </Box>
              <Stack spacing={0.75} sx={{ mt: 1.3 }}>
                <CompositionRow label={t('logPage.quotaDetail.regularInput')} value={formatTokenCount(uncachedInputTokens)} />
                {cacheDetails.length > 0 && (
                  <CompositionRow
                    accent
                    label={`${t('logPage.quotaDetail.cacheInput')} · ${cacheHitRate.toFixed(1)}%`}
                    value={formatTokenCount(cachedInputTokens)}
                  />
                )}
                <CompositionRow strong label={t('logPage.billableInputTokens')} value={formatTokenCount(billableInputTokens)} />
                <CompositionRow label={t('logPage.quotaDetail.outputTokens')} value={formatTokenCount(rawOutputTokens)} />
              </Stack>
            </>
          ) : (
            <Typography sx={{ mt: 1.2, color: 'text.secondary', fontSize: 13 }}>{t('logPage.quotaDetail.perRequestBilling')}</Typography>
          )}
        </Box>

        <Box sx={{ borderLeft: { xs: 0, md: '1px solid' }, borderColor: 'divider', pl: { xs: 0, md: 4 } }}>
          <Typography sx={{ color: 'text.primary', fontSize: 14, fontWeight: 700, mb: 0.8 }}>{t('logPage.quotaDetail.billingItems')}</Typography>
          <Stack>
            {billingRows.map((row) => <BillingRow key={row.key} {...row} />)}
          </Stack>
          <Divider sx={{ my: 1.2 }} />
          <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2 }}>
            <Box>
              <Typography sx={{ color: 'text.primary', fontSize: 13, fontWeight: 700 }}>{t('logPage.quotaDetail.roundedBilling')}</Typography>
              <Typography sx={{ color: 'text.disabled', fontSize: 11 }}>{t('logPage.quotaDetail.roundingHint')}</Typography>
            </Box>
            <Typography sx={{ color: 'success.main', fontSize: 19, fontWeight: 750, fontVariantNumeric: 'tabular-nums' }}>
              {renderQuota(quota, 6)}
            </Typography>
          </Box>
        </Box>
      </Box>

      {showCost && (
        <>
          <Divider sx={{ my: 2 }} />
          <Stack direction="row" spacing={1.5}>
            <Typography sx={{ color: 'warning.main', fontSize: 12 }}>{t('logPage.quotaDetail.cost')}: {renderQuota(costQuota, 6)}</Typography>
            <Typography sx={{ color: 'primary.main', fontSize: 12 }}>{t('logPage.quotaDetail.profit')}: {renderQuota(quota - costQuota, 6)}</Typography>
          </Stack>
        </>
      )}
    </Box>
  );
}

function ContextValue({ label, value, accent = false }) {
  return (
    <Typography component="span" sx={{ color: 'text.secondary', fontSize: 12, whiteSpace: 'nowrap' }}>
      {label}{' '}
      <Box component="strong" sx={{ color: accent ? 'primary.main' : 'text.primary', fontWeight: 650 }}>{value}</Box>
    </Typography>
  );
}

ContextValue.propTypes = {
  label: PropTypes.string.isRequired,
  value: PropTypes.string.isRequired,
  accent: PropTypes.bool
};

function CompositionRow({ label, value, accent = false, strong = false }) {
  return (
    <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2 }}>
      <Typography sx={{ color: accent ? 'primary.main' : 'text.secondary', fontSize: 12, fontWeight: strong ? 700 : 400 }}>{label}</Typography>
      <Typography sx={{ color: accent ? 'primary.main' : 'text.primary', fontSize: 12, fontWeight: strong ? 750 : 600, fontVariantNumeric: 'tabular-nums' }}>
        {value}
      </Typography>
    </Box>
  );
}

CompositionRow.propTypes = {
  label: PropTypes.string.isRequired,
  value: PropTypes.string.isRequired,
  accent: PropTypes.bool,
  strong: PropTypes.bool
};

function BillingRow({ label, formula, amount, accent = false }) {
  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: 'minmax(105px, 0.65fr) minmax(170px, 1.4fr) auto', alignItems: 'baseline', gap: 1.5, py: 0.9 }}>
      <Typography sx={{ color: accent ? 'primary.main' : 'text.primary', fontSize: 12, fontWeight: 650 }}>{label}</Typography>
      <Typography sx={{ color: 'text.secondary', fontSize: 11, fontVariantNumeric: 'tabular-nums' }}>{formula}</Typography>
      <Typography sx={{ color: accent ? 'primary.main' : 'text.primary', fontSize: 12, fontWeight: 650, fontVariantNumeric: 'tabular-nums', textAlign: 'right' }}>
        {amount}
      </Typography>
    </Box>
  );
}

BillingRow.propTypes = {
  label: PropTypes.string.isRequired,
  formula: PropTypes.string.isRequired,
  amount: PropTypes.string.isRequired,
  accent: PropTypes.bool
};

QuotaWithDetailContent.propTypes = {
  item: PropTypes.shape({
    quota: PropTypes.number,
    cost_quota: PropTypes.number,
    metadata: PropTypes.object
  }).isRequired,
  tokenBilling: PropTypes.shape({
    rawInputTokens: PropTypes.number.isRequired,
    rawOutputTokens: PropTypes.number.isRequired,
    uncachedInputTokens: PropTypes.number.isRequired,
    cachedInputTokens: PropTypes.number.isRequired,
    billableInputTokens: PropTypes.number.isRequired,
    cacheDetails: PropTypes.arrayOf(PropTypes.object).isRequired,
    tokenDetails: PropTypes.arrayOf(PropTypes.object).isRequired
  }).isRequired,
  userGroup: PropTypes.object,
  userIsAdmin: PropTypes.bool
};
