package relay

import (
	"done-hub/common/config"
	"done-hub/common/requester"
	"done-hub/internal/gateway/domain"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/providers/openai"
	"done-hub/types"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func TestWrappedChatResponseConvertsWithoutRawPassthrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"data":{"id":"chat-1","object":"chat.completion","model":"test-model","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":89,"completion_tokens":59,"total_tokens":148}}}`)
	}))
	defer server.Close()
	oldClient := requester.HTTPClient
	requester.HTTPClient = server.Client()
	t.Cleanup(func() { requester.HTTPClient = oldClient })

	channel := &model.Channel{Type: config.ChannelTypeOpenAI, ProtocolProfileID: string(domain.ProfileOpenAIChat), PassThroughBody: true}
	provider := openai.CreateOpenAIProvider(channel, server.URL)
	providerContext := base.NewMemoryRequestContext(httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	provider.SetContext(providerContext)
	provider.SetUsage(&types.Usage{PromptTokens: 17})
	response, apiErr := provider.CreateChatCompletion(&types.ChatCompletionRequest{Model: "test-model"})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, exists := providerContext.Get(config.GinRawResponseBodyKey); exists {
		t.Fatal("协议转换路径不应透传 Chat 原始响应")
	}

	context, _ := gin.CreateTestContext(nil)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	setInboundProtocol(context, domain.ProtocolOpenAIResponses)
	if err := bindProtocolRoute(context, channel); err != nil {
		t.Fatal(err)
	}
	converted, err := convertProtocolResponse(context, chatToResponsesResult{
		Response: response,
		Request:  &types.OpenAIResponsesRequest{Model: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := converted.(*types.OpenAIResponsesResponses)
	wireBody, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	if gjson.GetBytes(wireBody, "output.0.content.0.text").String() != "OK" || responses.Usage.InputTokens != 89 || responses.Usage.OutputTokens != 59 {
		t.Fatalf("Responses 转换丢失正文或 usage: %#v", responses)
	}
	claudeResponse := (&relayClaudeOnly{}).convertOpenAIResponseToClaude(response)
	if len(claudeResponse.Content) != 1 || claudeResponse.Content[0].Text != "OK" || claudeResponse.Usage.InputTokens != 89 || claudeResponse.Usage.OutputTokens != 59 {
		t.Fatalf("Claude 转换丢失正文或 usage: %#v", claudeResponse)
	}
}
