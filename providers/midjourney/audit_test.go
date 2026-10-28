package midjourney

import (
	"bytes"
	"context"
	"done-hub/common/config"
	"done-hub/common/requester"
	"done-hub/internal/audit"
	"done-hub/model"
	"done-hub/providers/base"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type auditContextKey struct{}
type auditTransport func(*http.Request) (*http.Response, error)

func (f auditTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSubmitRetainsAuditWithoutInheritingClientCancellation(t *testing.T) {
	rawRequest := " {\"prompt\": \"draw a landscape --fast\"} \n"
	rawResponse := " {\"code\":1,\"result\":\"local-task-id\"} \n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
		}
		var sent map[string]any
		if err = json.Unmarshal(body, &sent); err != nil {
			t.Error(err)
		}
		if sent["prompt"] != "draw a landscape " || sent["accountFilter"] == nil {
			t.Error("existing submit mapping changed", sent)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, rawResponse)
	}))
	defer upstream.Close()

	oldClient := requester.HTTPClient
	t.Cleanup(func() { requester.HTTPClient = oldClient })
	transport := upstream.Client().Transport
	requester.HTTPClient = &http.Client{Transport: auditTransport(func(req *http.Request) (*http.Response, error) {
		if req.Context().Err() != nil || req.Context().Value(auditContextKey{}) != "preserved" {
			t.Error("submit context values or cancellation changed")
		}
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
			t.Error("independent submit timeout changed", ok, deadline)
		}
		return transport.RoundTrip(req)
	})}

	var output bytes.Buffer
	recorder := audit.NewRecorder(&output)
	service := audit.NewService()
	service.SetRecorder(recorder)
	service.Apply(audit.Config{ChannelIDs: []int{25}, TokenIDs: []int{7}})
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), auditContextKey{}, "preserved"))
	defer cancel()
	incoming := httptest.NewRequest("POST", "/mj/submit/imagine", strings.NewReader(rawRequest)).WithContext(ctx)
	incoming.Header.Set("Content-Type", "application/json")
	incoming, session := service.Observe(incoming, "mj-submit", 7)
	requestContext := base.NewMemoryRequestContext(incoming)
	if _, err := requestContext.GetRawData(); err != nil {
		t.Fatal(err)
	}
	channel := &model.Channel{Id: 25, Type: config.ChannelTypeMidjourney, BaseURL: &upstream.URL, Key: "local-fixture-key"}
	provider := (MidjourneyProviderFactory{}).CreateAdapter(channel).(*MidjourneyProvider)
	provider.SetContext(requestContext)
	finish := audit.BeginAttempt(incoming.Context(), 1, 25)
	// Submit historically finishes independently after a client disconnect.
	cancel()
	result, body, err := provider.Send(5, "/mj/submit/imagine")
	finish()
	session.Finish()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	defer closeCancel()
	if closeErr := recorder.Close(closeCtx); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || result == nil || result.Response.Code != 1 || string(body) != rawResponse {
		t.Fatal("submit or response changed", result, string(body), err)
	}
	var requestBody, responseBody []byte
	responseEnds := 0
	decoder := json.NewDecoder(&output)
	for decoder.More() {
		var event audit.Event
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		switch event.Type {
		case "request_body":
			requestBody = append(requestBody, event.Data...)
		case "response_body":
			responseBody = append(responseBody, event.Data...)
		case "response_end":
			responseEnds++
			if event.Attempt != 1 || event.ChannelID != 25 || event.Incomplete {
				t.Fatal("submit attribution/completeness changed", event)
			}
		}
	}
	if string(requestBody) != rawRequest || string(responseBody) != rawResponse || responseEnds != 1 {
		t.Fatal("submit audit context lost", string(requestBody), string(responseBody), responseEnds)
	}
}
