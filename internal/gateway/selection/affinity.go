package selection

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"

	"done-hub/common/session"
	"done-hub/internal/gateway/domain"

	"github.com/tidwall/gjson"
)

const affinityKeyVersion = "v1"

type AffinityInput struct {
	Protocol domain.Protocol
	Headers  http.Header
	Body     []byte
	UserID   int
	TokenID  int
	Group    string
	Model    string
}

type AffinityKey struct {
	Digest string
	Source string
}

type Candidate struct {
	EndpointID int
	Weight     int
}

// BuildAffinityKey derives a privacy-safe routing key. Explicit protocol
// session/cache signals win; authenticated Done Hub identity is the stable
// fallback so ordinary SDK requests can still benefit from affinity.
func BuildAffinityKey(input AffinityInput) (AffinityKey, bool) {
	// Affinity must never collapse anonymous callers into a shared routing
	// identity when they happen to send the same upstream cache/session hint.
	if input.UserID <= 0 && input.TokenID <= 0 {
		return AffinityKey{}, false
	}
	seed, source := affinitySeed(input)
	if seed == "" {
		return AffinityKey{}, false
	}
	identity := ""
	if input.UserID > 0 {
		identity = "user:" + strconv.Itoa(input.UserID)
	} else if input.TokenID > 0 {
		identity = "token:" + strconv.Itoa(input.TokenID)
	}
	canonical := strings.Join([]string{
		affinityKeyVersion,
		identity,
		"token:" + strconv.Itoa(input.TokenID),
		strings.TrimSpace(input.Group),
		string(input.Protocol),
		strings.TrimSpace(input.Model),
		source,
		seed,
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return AffinityKey{Digest: hex.EncodeToString(digest[:]), Source: source}, true
}

// PickWeightedRendezvous deterministically selects one endpoint while
// preserving weights across affinity identities. Re-running with an endpoint
// removed naturally returns the next stable candidate, which fits retry and
// cooldown filtering without a mutable affinity store.
func PickWeightedRendezvous(key AffinityKey, candidates []Candidate) (int, bool) {
	if key.Digest == "" || len(candidates) == 0 {
		return 0, false
	}
	bestID := 0
	bestScore := math.Inf(1)
	for _, candidate := range candidates {
		if candidate.EndpointID <= 0 {
			continue
		}
		weight := candidate.Weight
		if weight <= 0 {
			weight = 1
		}
		sum := sha256.Sum256([]byte(key.Digest + ":" + strconv.Itoa(candidate.EndpointID)))
		raw := binary.BigEndian.Uint64(sum[:8])
		// Use the 53 exactly representable high bits and map them to (0, 1],
		// avoiding both log(0) and uint64-to-float rounding at the boundary.
		u := (float64(raw>>11) + 1) / 9007199254740993.0
		score := -math.Log(u) / float64(weight)
		if score < bestScore || (score == bestScore && candidate.EndpointID < bestID) {
			bestScore = score
			bestID = candidate.EndpointID
		}
	}
	return bestID, bestID > 0
}

func affinitySeed(input AffinityInput) (string, string) {
	header := func(names ...string) string {
		for _, name := range names {
			if value := strings.TrimSpace(input.Headers.Get(name)); value != "" {
				return value
			}
		}
		return ""
	}
	bodyValue := func(paths ...string) string {
		for _, path := range paths {
			value := gjson.GetBytes(input.Body, path)
			if !value.Exists() {
				continue
			}
			if text := strings.TrimSpace(value.String()); text != "" {
				return text
			}
		}
		return ""
	}

	switch input.Protocol {
	case domain.ProtocolOpenAIResponses:
		if value := bodyValue("prompt_cache_key"); value != "" {
			return value, "prompt_cache_key"
		}
		if value := header("session_id", "session-id", "x-session-id"); value != "" {
			return value, "session_header"
		}
		if value := header("thread_id", "thread-id"); value != "" {
			return value, "thread_header"
		}
		if value := bodyValue("conversation.id", "previous_response_id", "user"); value != "" {
			return value, "request_identity"
		}
	case domain.ProtocolOpenAIChat:
		if value := bodyValue("prompt_cache_key"); value != "" {
			return value, "prompt_cache_key"
		}
		if value := header("session_id", "session-id", "x-session-id", "conversation_id"); value != "" {
			return value, "session_header"
		}
		if value := bodyValue("user"); value != "" {
			return value, "request_user"
		}
	case domain.ProtocolClaudeMessages:
		if value := bodyValue("metadata.user_id.session_id"); value != "" {
			return value, "claude_session"
		}
		if metadata := gjson.GetBytes(input.Body, "metadata.user_id"); metadata.Exists() {
			if value := session.ExtractSessionIDFromMetadata(metadata.String()); value != "" {
				return value, "claude_session"
			}
			if value := strings.TrimSpace(metadata.String()); value != "" {
				return value, "metadata_user_id"
			}
		}
		if value := header("x-session-id", "session-id"); value != "" {
			return value, "session_header"
		}
	case domain.ProtocolGemini:
		if value := bodyValue("cachedContent", "cached_content"); value != "" {
			return value, "cached_content"
		}
		if value := header("x-session-id", "session-id", "conversation-id"); value != "" {
			return value, "session_header"
		}
	default:
		return "", ""
	}

	if input.UserID > 0 {
		return strconv.Itoa(input.UserID), "authenticated_user"
	}
	if input.TokenID > 0 {
		return strconv.Itoa(input.TokenID), "authenticated_token"
	}
	return "", ""
}
