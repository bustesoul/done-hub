package requeststate

import (
	"net/http"
	"testing"

	"done-hub/internal/gateway/domain"
)

func TestStateKeepsGatewayLifecycleOutOfHTTPFrameworkStore(t *testing.T) {
	request, state := Ensure(&http.Request{})
	state.SetInboundProtocol(domain.ProtocolOpenAIResponses)
	state.SetAttemptCount(2)
	state.Skip(7)
	state.SetSelection(Selection{
		ChannelID:     9,
		ChannelType:   1,
		OriginalModel: "public-model",
		UpstreamModel: "upstream-model",
	})

	loaded := From(request.Context())
	if loaded == nil {
		t.Fatal("request state was not attached")
	}
	if loaded.InboundProtocol() != domain.ProtocolOpenAIResponses {
		t.Fatalf("unexpected inbound protocol: %q", loaded.InboundProtocol())
	}
	if loaded.AttemptCount() != 2 {
		t.Fatalf("unexpected attempt count: %d", loaded.AttemptCount())
	}
	if selection := loaded.Selection(); selection.ChannelID != 9 || selection.UpstreamModel != "upstream-model" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	skipped := loaded.SkippedEndpointIDs()
	if len(skipped) != 1 || skipped[0] != 7 {
		t.Fatalf("unexpected skipped endpoints: %#v", skipped)
	}
}
