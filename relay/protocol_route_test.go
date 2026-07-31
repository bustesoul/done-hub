package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"done-hub/internal/gateway/domain"
	"done-hub/model"
	"done-hub/types"

	"github.com/gin-gonic/gin"
)

func TestPath2RelaySetsFirstClassInboundProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		path string
		want domain.Protocol
	}{
		{"/v1/chat/completions", domain.ProtocolOpenAIChat},
		{"/v1/responses", domain.ProtocolOpenAIResponses},
		{"/v1/responses/compact", domain.ProtocolOpenAIResponses},
		{"/claude/v1/messages", domain.ProtocolClaudeMessages},
		{"/gemini/v1beta/models/gemini:generateContent", domain.ProtocolGemini},
	}
	for _, test := range tests {
		context, _ := gin.CreateTestContext(nil)
		if relay := Path2Relay(context, test.path); relay == nil {
			t.Fatalf("%s did not resolve a relay", test.path)
		}
		if got := inboundProtocol(context); got != test.want {
			t.Fatalf("%s: expected %q, got %q", test.path, test.want, got)
		}
	}
}

func TestProtocolCompatibilityAllowsOnlyDeclaredSingleConversion(t *testing.T) {
	tests := []struct {
		from domain.Protocol
		to   domain.Protocol
		want bool
	}{
		{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIChat, true},
		{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses, true},
		{domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat, true},
		{domain.ProtocolOpenAIResponses, domain.ProtocolClaudeMessages, true},
		{domain.ProtocolGemini, domain.ProtocolOpenAIChat, false},
	}
	for _, test := range tests {
		if got := protocolCompatible(test.from, test.to); got != test.want {
			t.Fatalf("%s -> %s: expected %v, got %v", test.from, test.to, test.want, got)
		}
	}
}

func TestBindProtocolRouteAcceptsDeclaredConversion(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	setInboundProtocol(context, domain.ProtocolOpenAIChat)
	channel := &model.Channel{ProtocolProfileID: string(domain.ProfileOpenAIResponses)}

	if err := bindProtocolRoute(context, channel); err != nil {
		t.Fatalf("bind route: %v", err)
	}
	spec, ok := boundProtocolRoute(context)
	if !ok || spec.Converter == nil || spec.Converter.ID() == "" {
		t.Fatalf("bound route is not executable: %#v", spec)
	}
}

func TestChatResponsesRegistryExecutesRequestConversion(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	setInboundProtocol(context, domain.ProtocolOpenAIResponses)
	channel := &model.Channel{ProtocolProfileID: string(domain.ProfileOpenAIChat)}
	if err := bindProtocolRoute(context, channel); err != nil {
		t.Fatalf("bind route: %v", err)
	}

	converted, err := convertProtocolRequest(context, &types.OpenAIResponsesRequest{
		Model: "test-model",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	request, ok := converted.(*types.ChatCompletionRequest)
	if !ok || request.Model != "test-model" {
		t.Fatalf("unexpected conversion result: %#v", converted)
	}
}

func TestProtocolProfileFilterRejectsUndeclaredConversion(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	setInboundProtocol(context, domain.ProtocolOpenAIResponses)
	filter := filterProtocolProfile(context)

	if filter(1, &model.ChannelChoice{Channel: &model.Channel{ProtocolProfileID: string(domain.ProfileOpenAIChat)}}) {
		t.Fatal("Responses -> Chat is a declared conversion and should remain selectable")
	}
	if filter(2, &model.ChannelChoice{Channel: &model.Channel{ProtocolProfileID: string(domain.ProfileGoogleGemini)}}) {
		t.Fatal("Responses -> Gemini is a declared capability adapter")
	}
	if !filter(3, &model.ChannelChoice{Channel: &model.Channel{}}) {
		t.Fatal("cutover requires an explicit protocol profile")
	}
}

func TestFourProtocolRegistryBindsExecutableAdapters(t *testing.T) {
	tests := []struct {
		from domain.Protocol
		to   domain.Protocol
	}{
		{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIChat},
		{domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIResponses},
		{domain.ProtocolClaudeMessages, domain.ProtocolClaudeMessages},
		{domain.ProtocolGemini, domain.ProtocolGemini},
		{domain.ProtocolOpenAIChat, domain.ProtocolClaudeMessages},
		{domain.ProtocolOpenAIChat, domain.ProtocolGemini},
		{domain.ProtocolOpenAIResponses, domain.ProtocolClaudeMessages},
		{domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat},
	}
	for _, test := range tests {
		spec, err := relayProtocolRegistry.Get(test.from, test.to)
		if err != nil {
			t.Fatalf("%s -> %s: %v", test.from, test.to, err)
		}
		if spec.Converter == nil || spec.Converter.ID() == "" {
			t.Fatalf("%s -> %s has no executable converter", test.from, test.to)
		}
		if test.from != test.to && strings.HasPrefix(spec.Converter.ID(), "direct:") {
			t.Fatalf("%s -> %s incorrectly bound as direct", test.from, test.to)
		}
	}
}
