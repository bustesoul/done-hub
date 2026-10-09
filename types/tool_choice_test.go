package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFunctionToolChoiceConversion(t *testing.T) {
	for _, direction := range []string{"chat_to_responses", "responses_to_chat"} {
		t.Run(direction, func(t *testing.T) {
			chatJSON := `{"type":"function","function":{"name":"echo"},"x-extension":true}`
			responsesJSON := `{"type":"function","name":"echo","x-extension":true}`
			input, expected := chatJSON, responsesJSON
			convert := chatToolChoiceToResponses
			if direction == "responses_to_chat" {
				input, expected, convert = responsesJSON, chatJSON, responsesToolChoiceToChat
			}
			var original, want any
			if err := json.Unmarshal([]byte(input), &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(expected), &want); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(original)
			if got := convert(original); !reflect.DeepEqual(got, want) {
				t.Fatalf("转换后=%v，期望=%v", got, want)
			}
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatal("转换修改了输入，可能污染重试")
			}
		})
	}
}

func TestToolChoiceOtherShapesUnchanged(t *testing.T) {
	for _, raw := range []string{`null`, `"auto"`, `"none"`, `"required"`, `{"type":"web_search"}`, `{"type":"custom","name":"grammar"}`, `{"type":"allowed_tools","tools":[]}`, `{"type":"function"}`, `{"type":"function","name":3}`, `{"type":"function","function":{"name":3}}`} {
		t.Run(raw, func(t *testing.T) {
			var choice any
			if err := json.Unmarshal([]byte(raw), &choice); err != nil {
				t.Fatal(err)
			}
			for _, convert := range []func(any) any{chatToolChoiceToResponses, responsesToolChoiceToChat} {
				if got := convert(choice); !reflect.DeepEqual(got, choice) {
					t.Fatalf("不应改写非目标格式: %s -> %v", raw, got)
				}
			}
		})
	}
}
