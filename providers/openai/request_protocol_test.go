package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"done-hub/common/config"
	"done-hub/common/requester"
	"done-hub/internal/gateway/requeststate"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/types"
	"github.com/tidwall/gjson"
)

func TestActualConstructedRequestProtocol(t *testing.T) {
	for _, test := range []struct {
		mode            int
		protocol, field string
	}{
		{config.RelayModeChatCompletions, "openai_chat", "messages"},
		{config.RelayModeResponses, "openai_responses", "input"},
	} {
		t.Run(test.protocol, func(t *testing.T) {
			var gotProtocol string
			incoming, state := requeststate.Ensure(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			state.SetUpstreamObserver(func(protocol string) { gotProtocol = protocol })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
			defer server.Close()
			old := requester.HTTPClient
			requester.HTTPClient = server.Client()
			defer func() { requester.HTTPClient = old }()
			provider := CreateOpenAIProvider(&model.Channel{Type: config.ChannelTypeOpenAI}, server.URL)
			provider.SetContext(base.NewRequestContext(incoming, state))
			chat := &types.ChatCompletionRequest{Model: "test", Messages: []types.ChatCompletionMessage{{Role: "user", Content: "hello"}}}
			var payload any = chat
			if test.mode == config.RelayModeResponses {
				payload = chat.ToResponsesRequest()
			}
			req, err := provider.GetRequestTextBody(test.mode, "test", payload)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := req.GetBody()
			defer body.Close()
			// Verify the serialized wire fields rather than the configured channel profile.
			bytes, _ := io.ReadAll(body)
			if !gjson.GetBytes(bytes, test.field).Exists() {
				t.Fatalf("missing wire field %s: %s", test.field, bytes)
			}
			resp, sendErr := provider.Requester.SendRequestRaw(req)
			if sendErr != nil {
				t.Fatal(sendErr)
			}
			resp.Body.Close()
			if gotProtocol != test.protocol {
				t.Fatalf("protocol=%s want=%s", gotProtocol, test.protocol)
			}
		})
	}
}
