package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestToolExtraContentRemainsOpaque(t *testing.T) {
	for _, extra := range []string{`null`, `"vendor-string"`, `42`, `[]`, `{"vendor":{"x":1}}`, `{"google":{"thought_signature":"opaque+/="},"vendor":{"x":1}}`} {
		t.Run(extra, func(t *testing.T) {
			var call ChatCompletionToolCalls
			if err := json.Unmarshal([]byte(`{"id":"call_a","type":"function","function":{"name":"echo","arguments":"{}"},"extra_content":`+extra+`}`), &call); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(call)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			var want any
			json.Unmarshal(encoded, &got)
			json.Unmarshal([]byte(extra), &want)
			if !reflect.DeepEqual(got["extra_content"], want) {
				t.Fatalf("extension changed: %s", encoded)
			}
		})
	}
	encoded, _ := json.Marshal(ChatCompletionToolCalls{Id: "call_a"})
	var got map[string]any
	json.Unmarshal(encoded, &got)
	if _, exists := got["extra_content"]; exists {
		t.Fatalf("ordinary OpenAI call gained extension: %s", encoded)
	}
}
