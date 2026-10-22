package requester

import (
	"done-hub/internal/gateway/requeststate"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWireProtocolObservation(t *testing.T) {
	for _, test := range []struct{ path, hint, want string }{
		{"/v1/chat/completions", "", "openai_chat"},
		{"/v1/completions", "", "openai_completions"},
		{"/backend-api/codex/responses", "", "openai_responses"},
		{"/models/gemini:streamGenerateContent", "", "google_gemini"},
		{"/v1/messages", "", "anthropic_messages"},
		{"/nonstandard/inference", "openai_responses", "openai_responses"},
		{"/v1/chat/completions", "anthropic_messages", "anthropic_messages"},
		{"/oauth/token", "", ""},
	} {
		t.Run(test.path+test.hint, func(t *testing.T) {
			var observation atomic.Value
			observation.Store("")
			observed := func() string { return observation.Load().(string) }
			incoming, state := requeststate.Ensure(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			state.SetUpstreamObserver(func(p string) { observation.Store(p) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if observed() != test.want {
					t.Errorf("protocol not observed before send: %q", observed())
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"input":"untouched"}` {
					t.Errorf("body altered: %s", body)
				}
				io.WriteString(w, `{}`)
			}))
			defer server.Close()
			oldClient := HTTPClient
			HTTPClient = server.Client()
			defer func() { HTTPClient = oldClient }()
			client := NewHTTPRequester("", nil)
			client.Context = incoming.Context()
			req, err := client.NewRequest(http.MethodPost, server.URL+test.path, client.WithBody([]byte(`{"input":"untouched"}`)))
			if err != nil {
				t.Fatal(err)
			}
			req = WithRequestProtocol(req, test.hint)
			if observed() != "" {
				t.Fatal("request construction must not publish a send")
			}
			resp, apiErr := client.SendRequestRaw(req)
			if apiErr != nil {
				t.Fatal(apiErr)
			}
			resp.Body.Close()
			if observed() != test.want {
				t.Errorf("got=%s want=%s", observed(), test.want)
			}
		})
	}
}
