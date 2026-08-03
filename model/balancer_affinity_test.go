package model

import (
	"net/http/httptest"
	"testing"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/requeststate"

	"github.com/gin-gonic/gin"
)

func affinityTestContext(t *testing.T, protocol domain.Protocol, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	request, state := requeststate.Ensure(c.Request)
	c.Request = request
	state.SetInboundProtocol(protocol)
	c.Set(config.GinRequestBodyKey, []byte(body))
	c.Set("id", 10)
	c.Set("token_id", 20)
	c.Set("token_group", "default")
	return c
}

func TestSelectAffinityChannelStableWithinPriorityPool(t *testing.T) {
	c := affinityTestContext(t, domain.ProtocolOpenAIResponses, `{"prompt_cache_key":"cache-a"}`)
	weight := uint(1)
	candidates := []*ChannelChoice{
		{Channel: &Channel{Id: 1, Weight: &weight, AffinityEnabled: true}},
		{Channel: &Channel{Id: 2, Weight: &weight}},
	}
	first := selectAffinityChannel(candidates, "gpt-test", c)
	second := selectAffinityChannel(candidates, "gpt-test", c)
	if first == nil || second == nil || first.Id != second.Id {
		t.Fatalf("expected stable affinity selection, got %#v then %#v", first, second)
	}
}

func TestSelectAffinityChannelKeepsPolicyActiveAcrossRetry(t *testing.T) {
	c := affinityTestContext(t, domain.ProtocolOpenAIResponses, `{"prompt_cache_key":"cache-a"}`)
	weight := uint(1)
	initial := []*ChannelChoice{
		{Channel: &Channel{Id: 1, Weight: &weight, AffinityEnabled: true}},
		{Channel: &Channel{Id: 2, Weight: &weight}},
	}
	if selected := selectAffinityChannel(initial, "gpt-test", c); selected == nil {
		t.Fatal("expected initial affinity selection")
	}
	remaining := []*ChannelChoice{{Channel: &Channel{Id: 2, Weight: &weight}}}
	if selected := selectAffinityChannel(remaining, "gpt-test", c); selected == nil || selected.Id != 2 {
		t.Fatalf("expected deterministic retry candidate, got %#v", selected)
	}
}

func TestSelectAffinityChannelDisabledPoolFallsBack(t *testing.T) {
	c := affinityTestContext(t, domain.ProtocolOpenAIChat, `{}`)
	weight := uint(1)
	candidates := []*ChannelChoice{
		{Channel: &Channel{Id: 1, Weight: &weight}},
		{Channel: &Channel{Id: 2, Weight: &weight}},
	}
	if selected := selectAffinityChannel(candidates, "gpt-test", c); selected != nil {
		t.Fatalf("disabled pool must use legacy weighted selection, got %#v", selected)
	}
}
