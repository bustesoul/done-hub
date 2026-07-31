package relay

import (
	"context"
	"errors"
	"fmt"

	"done-hub/internal/gateway/domain"
	gatewayregistry "done-hub/internal/gateway/registry"
	"done-hub/model"
	"done-hub/providers/claude"
	"done-hub/providers/gemini"
	"done-hub/types"

	"github.com/gin-gonic/gin"
)

var relayProtocolRegistry = newRelayProtocolRegistry()

type relayProtocolConverter struct {
	id                string
	requestConverter  func(context.Context, any) (any, error)
	responseConverter func(context.Context, any) (any, error)
	streamConverter   func(context.Context, any) (any, error)
}

func (c relayProtocolConverter) ID() string {
	return c.id
}

func (c relayProtocolConverter) ConvertRequest(ctx context.Context, value any) (any, error) {
	if c.requestConverter == nil {
		return value, nil
	}
	return c.requestConverter(ctx, value)
}

func (c relayProtocolConverter) ConvertResponse(ctx context.Context, value any) (any, error) {
	if c.responseConverter == nil {
		return value, nil
	}
	return c.responseConverter(ctx, value)
}

func (c relayProtocolConverter) ConvertStream(ctx context.Context, value any) (any, error) {
	if c.streamConverter == nil {
		return nil, gatewayregistry.ErrStreamUnsupported
	}
	return c.streamConverter(ctx, value)
}

type chatToResponsesResult struct {
	Response *types.ChatCompletionResponse
	Request  *types.OpenAIResponsesRequest
}

func identityProtocolConverter(id string) relayProtocolConverter {
	identity := func(_ context.Context, value any) (any, error) { return value, nil }
	return relayProtocolConverter{
		id:                id,
		requestConverter:  identity,
		responseConverter: identity,
		streamConverter:   identity,
	}
}

// providerAdapterProtocolConverter binds a protocol route to the concrete
// provider/relay adapter that executes it. The request codec is run here as an
// admission check; the returned value stays in the normalized capability
// format consumed by ChatInterface, whose implementation performs the same
// native conversion when issuing the upstream request.
func providerAdapterProtocolConverter(from, to domain.Protocol) relayProtocolConverter {
	converter := identityProtocolConverter("capability-adapter:" + string(from) + "->" + string(to))
	converter.requestConverter = func(ctx context.Context, value any) (any, error) {
		switch request := value.(type) {
		case *types.ChatCompletionRequest:
			switch to {
			case domain.ProtocolClaudeMessages:
				if _, apiErr := claude.ConvertFromChatOpenai(request); apiErr != nil {
					return nil, apiErr
				}
			case domain.ProtocolGemini:
				if _, apiErr := gemini.ConvertFromChatOpenai(request); apiErr != nil {
					return nil, apiErr
				}
			}
			return request, nil
		case *types.OpenAIResponsesRequest:
			chatRequest, err := request.ToChatCompletionRequest()
			if err != nil {
				return nil, err
			}
			return converter.ConvertRequest(ctx, chatRequest)
		case *claude.ClaudeRequest:
			relay := &relayClaudeOnly{claudeRequest: request}
			chatRequest, apiErr := relay.convertClaudeToOpenAI()
			if apiErr != nil {
				return nil, apiErr
			}
			switch to {
			case domain.ProtocolOpenAIResponses:
				return chatRequest.ToResponsesRequest(), nil
			case domain.ProtocolGemini:
				if _, apiErr := gemini.ConvertFromChatOpenai(chatRequest); apiErr != nil {
					return nil, apiErr
				}
			}
			return chatRequest, nil
		default:
			return nil, fmt.Errorf("unsupported adapter request payload %T", value)
		}
	}
	converter.responseConverter = func(_ context.Context, value any) (any, error) {
		if payload, ok := value.(chatToResponsesResult); ok && from == domain.ProtocolOpenAIResponses {
			if payload.Response == nil || payload.Request == nil {
				return nil, errors.New("invalid chat to responses conversion payload")
			}
			return payload.Response.ToResponses(payload.Request), nil
		}
		return value, nil
	}
	return converter
}

func chatResponsesProtocolConverter(from, to domain.Protocol) relayProtocolConverter {
	converter := identityProtocolConverter("gateway-codec:" + string(from) + "->" + string(to))
	converter.requestConverter = func(_ context.Context, value any) (any, error) {
		switch request := value.(type) {
		case *types.ChatCompletionRequest:
			if to != domain.ProtocolOpenAIResponses {
				return nil, fmt.Errorf("chat request cannot convert to %s", to)
			}
			return request.ToResponsesRequest(), nil
		case *types.OpenAIResponsesRequest:
			if to != domain.ProtocolOpenAIChat {
				return nil, fmt.Errorf("responses request cannot convert to %s", to)
			}
			return request.ToChatCompletionRequest()
		default:
			return nil, fmt.Errorf("unsupported request payload %T", value)
		}
	}
	converter.responseConverter = func(_ context.Context, value any) (any, error) {
		switch response := value.(type) {
		case *types.OpenAIResponsesResponses:
			if to != domain.ProtocolOpenAIChat {
				return nil, fmt.Errorf("responses response cannot convert to %s", to)
			}
			return response.ToChat(), nil
		case chatToResponsesResult:
			if to != domain.ProtocolOpenAIResponses || response.Response == nil || response.Request == nil {
				return nil, errors.New("invalid chat to responses conversion payload")
			}
			return response.Response.ToResponses(response.Request), nil
		default:
			return nil, fmt.Errorf("unsupported response payload %T", value)
		}
	}
	return converter
}

func newRelayProtocolRegistry() *gatewayregistry.ProtocolRegistry {
	registry := gatewayregistry.NewProtocolRegistry()
	specs := []gatewayregistry.ConverterSpec{
		{From: domain.ProtocolOpenAIChat, To: domain.ProtocolOpenAIChat, Stream: true, Quality: gatewayregistry.ConversionGood, Converter: identityProtocolConverter("direct:openai-chat")},
		{From: domain.ProtocolOpenAIResponses, To: domain.ProtocolOpenAIResponses, Stream: true, Quality: gatewayregistry.ConversionGood, Converter: identityProtocolConverter("direct:openai-responses")},
		{From: domain.ProtocolClaudeMessages, To: domain.ProtocolClaudeMessages, Stream: true, Quality: gatewayregistry.ConversionGood, Converter: identityProtocolConverter("direct:claude-messages")},
		{From: domain.ProtocolGemini, To: domain.ProtocolGemini, Stream: true, Quality: gatewayregistry.ConversionGood, Converter: identityProtocolConverter("direct:gemini")},
		{From: domain.ProtocolOpenAIChat, To: domain.ProtocolOpenAIResponses, Stream: true, Quality: gatewayregistry.ConversionGood, Converter: chatResponsesProtocolConverter(domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses)},
		{From: domain.ProtocolOpenAIChat, To: domain.ProtocolClaudeMessages, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolOpenAIChat, domain.ProtocolClaudeMessages)},
		{From: domain.ProtocolOpenAIChat, To: domain.ProtocolGemini, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolOpenAIChat, domain.ProtocolGemini)},
		{From: domain.ProtocolOpenAIResponses, To: domain.ProtocolOpenAIChat, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: chatResponsesProtocolConverter(domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat)},
		{From: domain.ProtocolOpenAIResponses, To: domain.ProtocolClaudeMessages, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolOpenAIResponses, domain.ProtocolClaudeMessages)},
		{From: domain.ProtocolOpenAIResponses, To: domain.ProtocolGemini, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolOpenAIResponses, domain.ProtocolGemini)},
		{From: domain.ProtocolClaudeMessages, To: domain.ProtocolOpenAIChat, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat)},
		{From: domain.ProtocolClaudeMessages, To: domain.ProtocolOpenAIResponses, Stream: true, Quality: gatewayregistry.ConversionDiscouraged, Converter: providerAdapterProtocolConverter(domain.ProtocolClaudeMessages, domain.ProtocolOpenAIResponses)},
		{From: domain.ProtocolClaudeMessages, To: domain.ProtocolGemini, Stream: true, Quality: gatewayregistry.ConversionFair, Converter: providerAdapterProtocolConverter(domain.ProtocolClaudeMessages, domain.ProtocolGemini)},
	}
	for _, spec := range specs {
		if err := registry.Register(spec); err != nil {
			panic(err)
		}
	}
	return registry
}

func setInboundProtocol(c *gin.Context, protocol domain.Protocol) {
	if c != nil && protocol != "" {
		gatewayRequestState(c).SetInboundProtocol(protocol)
	}
}

func inboundProtocol(c *gin.Context) domain.Protocol {
	if c == nil {
		return ""
	}
	return gatewayRequestState(c).InboundProtocol()
}

func targetProtocol(channel *model.Channel) (domain.Protocol, bool) {
	if channel == nil || channel.ProtocolProfileID == "" {
		return "", false
	}
	protocol, ok := domain.ProtocolForProfile(domain.ProtocolProfileID(channel.ProtocolProfileID))
	return protocol, ok
}

func protocolCompatible(inbound, target domain.Protocol) bool {
	if inbound == "" || target == "" {
		return true
	}
	_, err := relayProtocolRegistry.Get(inbound, target)
	return err == nil
}

func bindProtocolRoute(c *gin.Context, channel *model.Channel) error {
	inbound := inboundProtocol(c)
	target, explicit := targetProtocol(channel)
	if !explicit {
		return nil
	}
	if !protocolCompatible(inbound, target) {
		return fmt.Errorf("protocol profile %q does not accept %q requests", channel.ProtocolProfileID, inbound)
	}

	if inbound == "" {
		return nil
	}
	spec, err := relayProtocolRegistry.Get(inbound, target)
	if err != nil {
		return err
	}
	gatewayRequestState(c).BindProtocolRoute(spec)
	return nil
}

func boundProtocolRoute(c *gin.Context) (gatewayregistry.ConverterSpec, bool) {
	if c == nil {
		return gatewayregistry.ConverterSpec{}, false
	}
	return gatewayRequestState(c).ProtocolRoute()
}

func convertProtocolRequest(c *gin.Context, value any) (any, error) {
	spec, ok := boundProtocolRoute(c)
	if !ok {
		return value, nil
	}
	return spec.Converter.ConvertRequest(c.Request.Context(), value)
}

func convertProtocolResponse(c *gin.Context, value any) (any, error) {
	spec, ok := boundProtocolRoute(c)
	if !ok {
		return value, nil
	}
	return spec.Converter.ConvertResponse(c.Request.Context(), value)
}

func convertProtocolStream(c *gin.Context, value any) (any, error) {
	spec, ok := boundProtocolRoute(c)
	if !ok {
		return value, nil
	}
	converter, ok := spec.Converter.(gatewayregistry.StreamConverter)
	if !ok {
		return nil, gatewayregistry.ErrStreamUnsupported
	}
	return converter.ConvertStream(c.Request.Context(), value)
}

func filterProtocolProfile(c *gin.Context) model.ChannelsFilterFunc {
	inbound := inboundProtocol(c)
	return func(_ int, choice *model.ChannelChoice) bool {
		if inbound == "" || inbound == domain.ProtocolNative {
			return false
		}
		target, explicit := targetProtocol(choice.Channel)
		return !explicit || !protocolCompatible(inbound, target)
	}
}
