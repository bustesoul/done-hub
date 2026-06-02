package relay

import (
	"done-hub/common/config"
	"testing"
)

func TestShouldSendChatAsResponses_KeepsGpt55CompatibleResponsePath(t *testing.T) {
	if !config.ShouldSendChatAsResponses(true, true, "gpt-5.5") {
		t.Fatal("gpt-5.5 should keep the existing compatible response path")
	}
}

func TestShouldSendChatAsResponses_SkipsBuiltinMimoCompatibleResponsePath(t *testing.T) {
	if config.ShouldSendChatAsResponses(true, true, "mimo-v2.5-pro") {
		t.Fatal("mimo built-in models should use chat compatibility instead of native responses")
	}
}
