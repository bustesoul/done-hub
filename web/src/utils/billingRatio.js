// Prices are reconstructed from the request's snapshot, never current channel settings.
export function billingRatio(metadata = {}) {
  if (Object.prototype.hasOwnProperty.call(metadata, 'effective_ratio')) return Number(metadata.effective_ratio);
  return Number(metadata.group_ratio ?? 1) * Number(metadata.service_tier_ratio ?? 1);
}
