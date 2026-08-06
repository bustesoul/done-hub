const TOKEN_DEFINITIONS = [
  { key: 'input_text_tokens', label: 'logPage.inputTextTokens', side: 'input' },
  { key: 'output_text_tokens', label: 'logPage.outputTextTokens', side: 'output' },
  { key: 'input_audio_tokens', label: 'logPage.inputAudioTokens', side: 'input' },
  { key: 'output_audio_tokens', label: 'logPage.outputAudioTokens', side: 'output' },
  { key: 'cached_tokens', label: 'logPage.cachedTokens', shortLabel: 'logPage.cacheReadShort', side: 'input', cache: true },
  { key: 'cached_write_tokens', label: 'logPage.cachedWriteTokens', shortLabel: 'logPage.cacheWriteShort', side: 'input', cache: true },
  {
    key: 'cached_write_1h_tokens',
    label: 'logPage.cachedWrite1hTokens',
    shortLabel: 'logPage.cacheWrite1hShort',
    side: 'input',
    cache: true
  },
  { key: 'cached_read_tokens', label: 'logPage.cachedReadTokens', shortLabel: 'logPage.cacheReadShort', side: 'input', cache: true },
  {
    key: 'openai_cache_write_tokens',
    label: 'logPage.openaiCacheWriteTokens',
    shortLabel: 'logPage.openAICacheWriteShort',
    side: 'input',
    cache: true
  },
  { key: 'reasoning_tokens', label: 'logPage.reasoningTokens', side: 'output' },
  { key: 'input_image_tokens', label: 'logPage.inputImageTokens', side: 'input' },
  { key: 'output_image_tokens', label: 'logPage.outputImageTokens', side: 'output' }
]

const toNumber = (value) => {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

export const formatTokenCount = (value) => new Intl.NumberFormat('en-US').format(toNumber(value))

export function calculateTokenBilling(item) {
  const metadata = item?.metadata || {}
  const rawInputTokens = toNumber(item?.prompt_tokens)
  const rawOutputTokens = toNumber(item?.completion_tokens)
  const hasExplicitCachedRead = toNumber(metadata.cached_read_tokens) > 0

  const tokenDetails = TOKEN_DEFINITIONS
    .filter(({ key }) => toNumber(metadata[key]) > 0)
    // The backend treats explicit cached_read_tokens as authoritative and omits
    // the OpenAI-compatible cached_tokens fallback. Mirror that rule for old logs
    // that may contain both fields so the UI never presents a double charge.
    .filter(({ key }) => key !== 'cached_tokens' || !hasExplicitCachedRead)
    .map((definition) => {
      const rawTokens = toNumber(metadata[definition.key])
      const ratioKey = `${definition.key}_ratio`
      const ratioValue = Number(metadata[ratioKey])
      const hasRatio = Number.isFinite(ratioValue)
      const ratio = hasRatio ? ratioValue : 1
      // Go converts the adjustment to int before adding it to prompt/output
      // tokens. Math.trunc matches that behavior for both discounts and markups.
      const adjustment = Math.trunc(rawTokens * (ratio - 1))

      return {
        ...definition,
        rawTokens,
        ratio,
        hasRatio,
        adjustment,
        billableTokens: rawTokens + adjustment
      }
    })

  const inputAdjustment = tokenDetails
    .filter(({ side }) => side === 'input')
    .reduce((sum, { adjustment }) => sum + adjustment, 0)
  const outputAdjustment = tokenDetails
    .filter(({ side }) => side === 'output')
    .reduce((sum, { adjustment }) => sum + adjustment, 0)
  const cacheDetails = tokenDetails.filter(({ cache }) => cache)
  const cachedInputTokens = cacheDetails.reduce((sum, { rawTokens }) => sum + rawTokens, 0)

  return {
    rawInputTokens,
    rawOutputTokens,
    uncachedInputTokens: Math.max(rawInputTokens - cachedInputTokens, 0),
    cachedInputTokens,
    billableInputTokens: rawInputTokens + inputAdjustment,
    billableOutputTokens: rawOutputTokens + outputAdjustment,
    cacheDetails,
    tokenDetails,
    hasCache: cacheDetails.length > 0,
    show: tokenDetails.length > 0
  }
}
