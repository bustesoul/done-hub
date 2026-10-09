package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChatToolsFunctionJSONRoundTrip(t *testing.T) {
	for _, strict := range []string{"", `,"strict":true`, `,"strict":false`} {
		t.Run("strict="+strict, func(t *testing.T) {
			raw := []byte(`{"model":"test","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"echo","description":"echo text","parameters":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}` + strict + `}}]}`)
			var request ChatCompletionRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			tool := request.Tools[0]
			if tool.Type != "function" || tool.Function.Name != "echo" || tool.ResponsesTools.Type != "function" {
				t.Fatalf("Chat 工具字段未正确解析: %#v", tool)
			}
			if _, exists := tool.ExtraFields["function"]; exists {
				t.Fatal("function 不应同时存入 Responses 扩展字段")
			}
			encoded, err := json.Marshal(&request)
			if err != nil {
				t.Fatal(err)
			}
			assertToolsJSONEqual(t, raw, encoded)

			responses := request.ToResponsesRequest()
			if responses.Tools[0].Name != "echo" || !reflect.DeepEqual(responses.Tools[0].Strict, tool.Function.Strict) {
				t.Fatalf("Chat 转 Responses 丢失工具字段: %#v", responses.Tools)
			}
			chat, err := responses.ToChatCompletionRequest()
			if err != nil {
				t.Fatal(err)
			}
			encoded, err = json.Marshal(chat)
			if err != nil {
				t.Fatal(err)
			}
			assertToolsJSONEqual(t, raw, encoded)
		})
	}
}

func TestChatToolsResponsesExtensionsSurviveConversion(t *testing.T) {
	raw := []byte(`{"model":"test","tools":[
		{"type":"namespace","name":"empty","tools":[],"x-meta":{"version":1}},
		{"type":"namespace","name":"nested","tools":[{"type":"function","name":"echo","parameters":{"type":"object"},"x-nested":true}]},
		{"type":"tool_search","execution":"client","name":"find","description":"find tools","parameters":{"type":"object"},"strict":false},
		{"type":"custom","name":"grammar","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}}
	]}`)
	var chat ChatCompletionRequest
	if err := json.Unmarshal(raw, &chat); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(chat.ToResponsesRequest())
	if err != nil {
		t.Fatal(err)
	}
	assertToolsJSONEqual(t, raw, encoded)
}

func TestResponsesFunctionToolsRoundTripThroughChat(t *testing.T) {
	raw := []byte(`{"model":"test","input":"hello","tools":[
		{"type":"function","name":"first","description":"first tool","parameters":{"type":"object","properties":{}},"strict":true},
		{"type":"function","name":"second","description":"second tool","parameters":{"type":"object","properties":{"n":{"type":"integer"}}},"strict":false}
	]}`)
	var responses OpenAIResponsesRequest
	if err := json.Unmarshal(raw, &responses); err != nil {
		t.Fatal(err)
	}
	chat, err := responses.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a Chat protocol hop rather than only converting in-memory structs.
	chatJSON, err := json.Marshal(chat)
	if err != nil {
		t.Fatal(err)
	}
	var received ChatCompletionRequest
	if err := json.Unmarshal(chatJSON, &received); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(received.ToResponsesRequest())
	if err != nil {
		t.Fatal(err)
	}
	assertToolsJSONEqual(t, raw, encoded)
}

func TestChatToolDecodeReplacesPreviousState(t *testing.T) {
	var tool ChatCompletionTool
	for _, raw := range []string{
		`{"type":"namespace","tools":[],"x-old":1}`,
		`{"type":"function","function":{"name":"echo","parameters":{"type":"object"},"strict":true},"x-new":2}`,
		`{"type":"custom"}`,
	} {
		if err := json.Unmarshal([]byte(raw), &tool); err != nil {
			t.Fatal(err)
		}
		var fresh ChatCompletionTool
		if err := json.Unmarshal([]byte(raw), &fresh); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tool, fresh) {
			t.Fatalf("重复解码残留旧字段: %#v", tool)
		}
	}
}

func TestChatToolDecodeInvalidFields(t *testing.T) {
	for _, raw := range []string{`{"type":3}`, `{"type":"function","function":[]}`, `{"type":"function","function":{"strict":"false"}}`} {
		var tool ChatCompletionTool
		if err := json.Unmarshal([]byte(raw), &tool); err == nil {
			t.Fatalf("应拒绝错误字段类型: %s", raw)
		}
	}
	var tool ChatCompletionTool
	if err := json.Unmarshal([]byte(`{"function":{"name":"echo"}}`), &tool); err != nil {
		t.Fatal(err)
	}
	if tool.Type != "" {
		t.Fatal("不应擅自补充缺失的工具类型")
	}
}

func TestChatToolsAbsentAndNull(t *testing.T) {
	for _, raw := range []string{`{}`, `{"tools":null}`, `{"tools":[]}`} {
		var request ChatCompletionRequest
		if err := json.Unmarshal([]byte(raw), &request); err != nil {
			t.Fatal(err)
		}
		if len(request.Tools) != 0 || len(request.ToResponsesRequest().Tools) != 0 {
			t.Fatalf("无工具请求不应产生工具: %s", raw)
		}
	}
}
