package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestResponsesTools_FunctionToolRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4",
		"input":"test",
		"tools":[
			{
				"type":"function",
				"name":"hello",
				"description":"say hello",
				"parameters":{"type":"object","properties":{}}
			}
		]
	}`)

	var request OpenAIResponsesRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if len(request.Tools) != 1 {
		t.Fatalf("tools 数量不正确: %d", len(request.Tools))
	}
	if request.Tools[0].Type != "function" {
		t.Fatalf("tool 类型错误: %s", request.Tools[0].Type)
	}
	if request.Tools[0].Name != "hello" {
		t.Fatalf("function name 错误: %s", request.Tools[0].Name)
	}

	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	assertToolsJSONEqual(t, raw, encoded)
}

func TestResponsesTools_NamespaceToolRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4",
		"input":"test",
		"tools":[
			{
				"type":"namespace",
				"name":"mcp_test",
				"description":"test namespace",
				"x-codex-meta":{"server":"demo"},
				"tools":[
					{
						"type":"function",
						"name":"hello",
						"description":"say hello",
						"parameters":{"type":"object","properties":{}},
						"x-origin":"nested"
					}
				]
			}
		]
	}`)

	var request OpenAIResponsesRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if len(request.Tools) != 1 {
		t.Fatalf("tools 数量不正确: %d", len(request.Tools))
	}
	namespaceTool := request.Tools[0]
	if namespaceTool.Type != "namespace" {
		t.Fatalf("tool 类型错误: %s", namespaceTool.Type)
	}
	if len(namespaceTool.Tools) != 1 {
		t.Fatalf("namespace 内部 tools 丢失，实际数量: %d", len(namespaceTool.Tools))
	}
	if namespaceTool.Tools[0].Name != "hello" {
		t.Fatalf("namespace 内部 function name 错误: %s", namespaceTool.Tools[0].Name)
	}
	if _, ok := namespaceTool.ExtraFields["x-codex-meta"]; !ok {
		t.Fatal("namespace 未保留未知字段 x-codex-meta")
	}
	if _, ok := namespaceTool.Tools[0].ExtraFields["x-origin"]; !ok {
		t.Fatal("嵌套 function 未保留未知字段 x-origin")
	}

	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	assertToolsJSONEqual(t, raw, encoded)
}

func TestResponsesTools_EmptyNamespaceToolsArrayPreserved(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4",
		"input":"test",
		"tools":[
			{
				"type":"namespace",
				"name":"mcp_empty",
				"tools":[]
			}
		]
	}`)

	var request OpenAIResponsesRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	assertToolsJSONEqual(t, raw, encoded)
}

func TestResponsesTools_MixedToolsRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4",
		"input":"test",
		"tools":[
			{
				"type":"function",
				"name":"plain_fn",
				"description":"plain function",
				"parameters":{"type":"object","properties":{}}
			},
			{
				"type":"namespace",
				"name":"mcp_test",
				"description":"test namespace",
				"tools":[
					{
						"type":"function",
						"name":"hello",
						"description":"say hello",
						"parameters":{"type":"object","properties":{}}
					}
				]
			}
		]
	}`)

	var request OpenAIResponsesRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if len(request.Tools) != 2 {
		t.Fatalf("tools 数量不正确: %d", len(request.Tools))
	}
	if request.Tools[0].Type != "function" || request.Tools[1].Type != "namespace" {
		t.Fatalf("mixed tools 类型顺序错误: %#v", request.Tools)
	}
	if len(request.Tools[1].Tools) != 1 {
		t.Fatalf("mixed tools 中 namespace 内部 tools 丢失，实际数量: %d", len(request.Tools[1].Tools))
	}

	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	assertToolsJSONEqual(t, raw, encoded)
}

func assertToolsJSONEqual(t *testing.T, expectedJSON, actualJSON []byte) {
	t.Helper()

	expectedTools := extractToolsJSON(t, expectedJSON)
	actualTools := extractToolsJSON(t, actualJSON)

	if !reflect.DeepEqual(expectedTools, actualTools) {
		expectedBytes, _ := json.Marshal(expectedTools)
		actualBytes, _ := json.Marshal(actualTools)
		t.Fatalf("tools 结构不一致\n期望: %s\n实际: %s", string(expectedBytes), string(actualBytes))
	}
}

func extractToolsJSON(t *testing.T, payload []byte) any {
	t.Helper()

	var wrapper map[string]any
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		t.Fatalf("解析 payload 失败: %v", err)
	}

	return wrapper["tools"]
}
