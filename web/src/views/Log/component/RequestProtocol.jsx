import PropTypes from 'prop-types'
import { Stack, Tooltip, Typography } from '@mui/material'
import { ArrowForward } from '@mui/icons-material'
import { useTranslation } from 'react-i18next'
import Label from 'ui-component/Label'

export const protocols = {
  openai_chat: ['Chat', 'OpenAI Chat Completions', '/v1/chat/completions'],
  openai_completions: ['Completions', 'OpenAI Completions', '/v1/completions'],
  openai_responses: ['Responses', 'OpenAI Responses', '/v1/responses'],
  openai_responses_compact: ['Compact', 'OpenAI Responses Compact', '/v1/responses/compact'],
  anthropic_messages: ['Claude', 'Anthropic Messages', '/v1/messages'],
  google_gemini: ['Gemini', 'Google Gemini', 'generateContent / streamGenerateContent'],
  openai_embeddings: ['Embeddings', 'OpenAI Embeddings', '/v1/embeddings'],
  openai_images: ['Images', 'OpenAI Image Generations', '/v1/images/generations'],
  openai_image_edits: ['Image edits', 'OpenAI Image Edits', '/v1/images/edits'],
  openai_image_variations: ['Variations', 'OpenAI Image Variations', '/v1/images/variations'],
  openai_transcription: ['Transcription', 'OpenAI Audio Transcriptions', '/v1/audio/transcriptions'],
  openai_translation: ['Translation', 'OpenAI Audio Translations', '/v1/audio/translations'],
  openai_speech: ['Speech', 'OpenAI Audio Speech', '/v1/audio/speech'],
  openai_moderations: ['Moderations', 'OpenAI Moderations', '/v1/moderations'],
  openai_realtime: ['Realtime', 'OpenAI Realtime', '/v1/realtime']
}

export default function RequestProtocol({ item }) {
  const { t } = useTranslation()
  const source = item.inbound_protocol
  const target = item.upstream_protocol
  if (!source && !target) return <Typography component="span" color="text.secondary">-</Typography>
  const tag = (protocol, direction) => {
    const info = protocols[protocol]
    return (
      <Tooltip arrow title={info ? `${direction}: ${info[1]} · ${info[2]}` : t('logPage.protocolUnknown')}>
        <span tabIndex={0} style={{ display: 'inline-flex', cursor: 'help' }}>
          {info ? <Label color="default" variant="soft">{info[0]}</Label> : <Typography color="text.secondary">-</Typography>}
        </span>
      </Tooltip>
    )
  }
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" justifyContent="center" sx={{ whiteSpace: 'nowrap' }}>
      {tag(source, source === target ? t('logPage.protocolDirect') : t('logPage.protocolInbound'))}
      {source !== target && <><ArrowForward sx={{ fontSize: 14, color: 'text.disabled' }} />{tag(target, t('logPage.protocolUpstream'))}</>}
    </Stack>
  )
}
RequestProtocol.propTypes = { item: PropTypes.object.isRequired }
