package requester

import (
	"context"
	"net/http"
	"strings"

	"done-hub/internal/gateway/requeststate"
)

// ProtocolForPath identifies wire APIs, not channel profiles. Unknown APIs stay unknown.
// Paths/headers/bodies themselves are never stored in the request log.
func ProtocolForPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return "openai_chat"
	case strings.HasSuffix(path, "/completions"):
		return "openai_completions"
	case strings.HasSuffix(path, "/responses/compact"):
		return "openai_responses_compact"
	case strings.HasSuffix(path, "/responses"):
		return "openai_responses"
	case strings.HasSuffix(path, "/messages"):
		return "anthropic_messages"
	case strings.HasSuffix(path, ":generateContent"), strings.HasSuffix(path, ":streamGenerateContent"):
		return "google_gemini"
	case strings.HasSuffix(path, "/embeddings"):
		return "openai_embeddings"
	case strings.HasSuffix(path, "/images/generations"):
		return "openai_images"
	case strings.HasSuffix(path, "/images/edits"):
		return "openai_image_edits"
	case strings.HasSuffix(path, "/images/variations"):
		return "openai_image_variations"
	case strings.HasSuffix(path, "/audio/transcriptions"):
		return "openai_transcription"
	case strings.HasSuffix(path, "/audio/translations"):
		return "openai_translation"
	case strings.HasSuffix(path, "/audio/speech"):
		return "openai_speech"
	case strings.HasSuffix(path, "/moderations"):
		return "openai_moderations"
	case strings.HasSuffix(path, "/realtime"):
		return "openai_realtime"
	}
	return ""
}

func observeRequestProtocol(req *http.Request) {
	if req.Method != http.MethodPost {
		return
	}
	protocol, _ := req.Context().Value(wireProtocolKey{}).(string)
	if protocol == "" {
		protocol = ProtocolForPath(req.URL.Path)
	}
	if protocol == "" {
		return
	} // Do not overwrite with OAuth, quota or discovery requests.
	requeststate.From(req.Context()).ObserveUpstreamProtocol(protocol)
}

// WithRequestProtocol tags a constructed request without emitting an observation until send.
// This also supports custom endpoint paths without inspecting or copying request bodies.
type wireProtocolKey struct{}

func WithRequestProtocol(req *http.Request, protocol string) *http.Request {
	if req == nil || protocol == "" {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), wireProtocolKey{}, protocol))
}
