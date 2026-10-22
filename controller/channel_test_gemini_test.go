package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"

	"go.uber.org/zap"
)

func TestChannelGeminiProbeRequestBody(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requestModel string
		mapping      string
	}{
		{name: "configured_model"},
		{name: "explicit_mapped_model", requestModel: "client-model", mapping: `{"client-model":"upstream-model"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1beta/models/upstream-model:generateContent" {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("x-goog-api-key") != "test-key" {
					t.Error("missing Gemini credential header")
				}
				// Decode independently of the provider's request types.
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode upstream body: %v", err)
				}
				received <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}`))
			}))
			defer server.Close()

			oldClient, oldLogger := requester.HTTPClient, logger.Logger
			requester.HTTPClient, logger.Logger = server.Client(), zap.NewNop()
			defer func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger }()

			channel := &model.Channel{
				Type:              config.ChannelTypeGemini,
				ProtocolProfileID: "google-gemini",
				Key:               "test-key",
				BaseURL:           &server.URL,
				TestModel:         "upstream-model",
			}
			if tc.mapping != "" {
				channel.ModelMapping = &tc.mapping
			}
			if apiErr, err := testChannel(channel, tc.requestModel); apiErr != nil || err != nil {
				t.Fatalf("Gemini probe failed: apiErr=%+v err=%v", apiErr, err)
			}

			var want map[string]any
			if err := json.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"You just need to output 'hi' next."}]}]}`), &want); err != nil {
				t.Fatal(err)
			}
			select {
			case body := <-received:
				if !reflect.DeepEqual(body["contents"], want["contents"]) {
					t.Errorf("upstream contents = %#v, want %#v", body["contents"], want["contents"])
				}
			default:
				t.Fatal("Gemini probe did not reach upstream")
			}
		})
	}
}
