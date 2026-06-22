package claude

import (
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ClaudeProviderFactory struct{}

// 创建 ClaudeProvider
func (f ClaudeProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	return &ClaudeProvider{
		BaseProvider: base.BaseProvider{
			Config:    getConfig(),
			Channel:   channel,
			Requester: requester.NewHTTPRequester(channel.GetProxy(), RequestErrorHandle),
		},
	}
}

type ClaudeProvider struct {
	base.BaseProvider
}

func getConfig() base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:         "https://api.anthropic.com",
		ChatCompletions: "/v1/messages",
		ModelList:       "/v1/models",
	}
}

// 请求错误处理
func RequestErrorHandle(resp *http.Response) *types.OpenAIError {
	claudeError := &ClaudeError{}
	err := json.NewDecoder(resp.Body).Decode(claudeError)
	if err != nil {
		return nil
	}

	return errorHandle(claudeError)
}

// 错误处理
func errorHandle(claudeError *ClaudeError) *types.OpenAIError {
	if claudeError == nil {
		return nil
	}

	if claudeError.Type == "" {
		return nil
	}
	return &types.OpenAIError{
		Message: claudeError.ErrorInfo.Message,
		Type:    claudeError.ErrorInfo.Type,
		Code:    claudeError.Type,
	}
}

// 获取请求头
func (p *ClaudeProvider) GetRequestHeaders() (headers map[string]string) {
	headers = make(map[string]string)
	p.CommonRequestHeaders(headers)

	// 透传客户端的 anthropic-beta。ModelHeaders 会在调用方补齐默认头后应用。
	if p.Context != nil {
		if anthropicBeta := p.Context.Request.Header.Get("anthropic-beta"); anthropicBeta != "" {
			headers["anthropic-beta"] = anthropicBeta
		}

		// 透传 Claude Code 客户端指纹头，避免上游把请求识别为非客户端。
		// 透传规则：除了少数由本函数显式管理的头（host / x-api-key / anthropic-version /
		// anthropic-beta），其它客户端头都透传。这样不需要为每一种新指纹头单独维护白名单：
		//   - User-Agent：Claude Code CLI 携带 "claude-cli/x.x.x"，上游常用此识别客户端
		//   - x-app：relay-code-github 等中转会校验非空
		//   - x-stainless-*：Anthropic SDK 指纹头
		//   - 任何未来新增的客户端识别头
		for headerName, values := range p.Context.Request.Header {
			if len(values) == 0 || values[0] == "" {
				continue
			}
			if isProviderManagedHeader(headerName) {
				continue
			}
			if p.HeaderExists(headers, headerName) {
				continue
			}
			headers[strings.ToLower(headerName)] = values[0]
		}
	}

	p.applyFixedHeaders(headers)

	return headers
}

// isProviderManagedHeader 判定一个客户端入站头是否禁止原样透传到上游。
// 包含两类：
//  1. 由本 Provider 显式管理的协议头（host / x-api-key / anthropic-version / anthropic-beta）
//  2. 安全敏感或 HTTP 传输层头，原样转发会出问题
func isProviderManagedHeader(name string) bool {
	switch strings.ToLower(name) {
	// Provider 显式管理：必须由本函数控制
	case "host",
		"x-api-key",
		"anthropic-version",
		"anthropic-beta":
		return true
	// 安全敏感：done-hub 用户的认证凭据，禁止泄漏给上游
	case "authorization",
		"cookie",
		"proxy-authorization":
		return true
	// HTTP 传输层：由 http.Client 控制，禁止透传客户端原值
	case "content-length",
		"connection",
		"transfer-encoding",
		"accept-encoding",
		"upgrade",
		"keep-alive",
		"te",
		"trailer":
		return true
	}
	return false
}

func (p *ClaudeProvider) applyFixedHeaders(headers map[string]string) {
	p.SetHeader(headers, "x-api-key", p.Channel.Key)
	anthropicVersion := ""
	if p.Context != nil {
		anthropicVersion = p.Context.Request.Header.Get("anthropic-version")
	}
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	p.SetHeader(headers, "anthropic-version", anthropicVersion)
}

func (p *ClaudeProvider) GetFullRequestURL(requestURL string) string {
	baseURL := strings.TrimSuffix(p.GetBaseURL(), "/")
	if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
		requestURL = strings.TrimPrefix(requestURL, "/v1")
	}

	return fmt.Sprintf("%s%s", baseURL, requestURL)
}

func stopReasonClaude2OpenAI(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return types.FinishReasonStop
	case "max_tokens":
		return types.FinishReasonLength
	case "tool_use":
		return types.FinishReasonToolCalls
	case "refusal":
		return types.FinishReasonContentFilter
	default:
		return reason
	}
}

func convertRole(role string) string {
	switch role {
	case types.ChatMessageRoleUser, types.ChatMessageRoleTool, types.ChatMessageRoleFunction:
		return types.ChatMessageRoleUser
	default:
		return types.ChatMessageRoleAssistant
	}
}
