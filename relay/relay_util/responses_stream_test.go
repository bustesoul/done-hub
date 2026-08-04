package relay_util

import (
	"done-hub/types"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOpenAIResponsesStreamConverterEmitsStrictReasoningLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	converter := NewOpenAIResponsesStreamConverter(context, &types.OpenAIResponsesRequest{Model: "deepseek-v4-flash"}, &types.Usage{
		PromptTokens:     3,
		CompletionTokens: 5,
		TotalTokens:      8,
		CompletionTokensDetails: types.CompletionTokensDetails{
			ReasoningTokens: 2,
		},
	})

	converter.ProcessStreamData(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`)
	converter.ProcessStreamData(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`)
	converter.ProcessStreamData(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":null}]}`)
	converter.ProcessStreamData(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	converter.ProcessStreamData("[DONE]")

	events := parseResponsesSSEEvents(t, recorder.Body.String())
	wantOrder := []string{
		"response.output_item.added",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_text.delta",
		"response.reasoning_summary_text.done",
		"response.reasoning_summary_part.done",
		"response.output_item.done",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	assertEventSubsequence(t, events, wantOrder)

	for _, event := range events {
		if event.Type == "response.reasoning_summary_part.added" || event.Type == "response.reasoning_summary_text.delta" {
			if event.SummaryIndex == nil || *event.SummaryIndex != 0 {
				t.Fatalf("%s 缺少 summary_index: %#v", event.Type, event)
			}
			if event.ContentIndex != nil {
				t.Fatalf("%s 不应使用 content_index: %#v", event.Type, event)
			}
		}
	}

	completed := events[len(events)-1]
	if completed.Type != "response.completed" || completed.Response == nil {
		t.Fatalf("缺少 response.completed: %#v", completed)
	}
	if len(completed.Response.Output) != 2 {
		t.Fatalf("最终输出应包含 reasoning 和 message: %#v", completed.Response.Output)
	}
	reasoning := completed.Response.Output[0]
	if reasoning.Type != types.InputTypeReasoning || reasoning.GetSummaryString() != "think" {
		t.Fatalf("最终 reasoning summary 错误: %#v", reasoning)
	}
	if reasoning.Content != nil || reasoning.EncryptedContent != nil {
		t.Fatalf("最终 reasoning 不应包含 message content 或伪造密文: %#v", reasoning)
	}
	if completed.Response.Usage == nil || completed.Response.Usage.OutputTokensDetails == nil || completed.Response.Usage.OutputTokensDetails.ReasoningTokens != 2 {
		t.Fatalf("reasoning_tokens 未保留: %#v", completed.Response.Usage)
	}
}

func TestOpenAIResponsesStreamConverterWithoutReasoningEmitsOnlyMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	converter := NewOpenAIResponsesStreamConverter(context, &types.OpenAIResponsesRequest{Model: "plain-model"}, &types.Usage{})

	converter.ProcessStreamData(`{"id":"chatcmpl-2","created":1,"model":"plain-model","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`)
	converter.ProcessStreamData(`{"id":"chatcmpl-2","created":1,"model":"plain-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	converter.ProcessStreamData("[DONE]")

	body := recorder.Body.String()
	if strings.Contains(body, "reasoning_summary") || strings.Contains(body, `"type":"reasoning"`) {
		t.Fatalf("无 reasoning 上游不应生成空 reasoning 事件: %s", body)
	}
	events := parseResponsesSSEEvents(t, body)
	completed := events[len(events)-1]
	if completed.Response == nil || len(completed.Response.Output) != 1 || completed.Response.Output[0].Type != types.InputTypeMessage {
		t.Fatalf("无 reasoning 响应应只有 message: %#v", completed.Response)
	}
}

func parseResponsesSSEEvents(t *testing.T, body string) []types.OpenAIResponsesStreamResponses {
	t.Helper()
	var events []types.OpenAIResponsesStreamResponses
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event types.OpenAIResponsesStreamResponses
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatalf("解析 SSE 事件失败: %v\n%s", err, line)
			}
			events = append(events, event)
		}
	}
	return events
}

func assertEventSubsequence(t *testing.T, events []types.OpenAIResponsesStreamResponses, want []string) {
	t.Helper()
	position := 0
	for _, event := range events {
		if position < len(want) && event.Type == want[position] {
			position++
		}
	}
	if position != len(want) {
		got := make([]string, 0, len(events))
		for _, event := range events {
			got = append(got, event.Type)
		}
		t.Fatalf("事件顺序不完整\nwant subsequence: %v\ngot: %v", want, got)
	}
}
