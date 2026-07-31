package midjourney

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/model"
	provider "done-hub/providers/midjourney"

	"github.com/gin-gonic/gin"
)

func proxyMidjourneyImageViaGateway(c *gin.Context, channelID int, imageURL string) error {
	channel := model.GatewayRoutes.GetChannel(channelID)
	if channel == nil {
		return fmt.Errorf("gateway endpoint %d is unavailable", channelID)
	}
	endpoint := endpointForMidjourney(channel)
	plan := domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:  c.GetString(logger.RequestIdKey),
			UserID:     c.GetInt("id"),
			TokenID:    c.GetInt("token_id"),
			Capability: domain.CapabilityPassthrough,
			Protocol:   domain.ProtocolNative,
			StartedAt:  c.GetTime("requestStartTime"),
		},
		Model:     domain.ModelRoute{Capability: domain.CapabilityPassthrough, Protocol: domain.ProtocolNative, EndpointID: channelID},
		Endpoints: []domain.Endpoint{endpoint},
	}
	runner := &midjourneyImageRunner{context: c, imageURL: imageURL}
	engine := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	engine.MaxAttempts = 1
	result := engine.Run(c.Request.Context(), plan, runner)
	if runner.err != nil {
		return runner.err
	}
	if result.Err != nil {
		return result.Err
	}
	return nil
}

type midjourneyImageRunner struct {
	context  *gin.Context
	imageURL string
	err      error
}

func (r *midjourneyImageRunner) Run(
	ctx context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.imageURL, nil)
	if err != nil {
		r.err = err
		return domain.AttemptResult{Attempt: attempt}, gatewayretry.ClassifyHTTP(0, err)
	}
	resp, err := requester.HTTPClient.Do(req)
	if err != nil {
		r.err = err
		return domain.AttemptResult{Attempt: attempt}, gatewayretry.ClassifyHTTP(0, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		r.err = fmt.Errorf("image upstream status %d: %s", resp.StatusCode, string(body))
		return domain.AttemptResult{Attempt: attempt}, gatewayretry.ClassifyHTTP(resp.StatusCode, r.err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	r.context.Header("Content-Type", contentType)
	_ = session.BeginHeaders()
	r.context.Status(http.StatusOK)
	written, err := io.Copy(r.context.Writer, resp.Body)
	if written > 0 {
		_ = session.RecordData(int(written))
	}
	if err != nil {
		r.err = err
		return domain.AttemptResult{Attempt: attempt}, gatewayretry.ClassifyHTTP(http.StatusInternalServerError, err)
	}
	return domain.AttemptResult{Attempt: attempt}, nil
}

// sendMidjourneyViaGateway makes the legacy Midjourney protocol a normal
// GatewayEngine runner. Channel selection remains in the request adaptor, but
// transport execution and attempt observation no longer bypass the engine.
func sendMidjourneyViaGateway(
	c *gin.Context,
	mjProvider *provider.MidjourneyProvider,
	timeout int,
	requestURL string,
) (*provider.MidjourneyResponseWithStatusCode, []byte, error) {
	channel := mjProvider.GetChannel()
	endpoint := endpointForMidjourney(channel)
	plan := domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:  c.GetString(logger.RequestIdKey),
			UserID:     c.GetInt("id"),
			TokenID:    c.GetInt("token_id"),
			Group:      c.GetString("token_group"),
			Capability: domain.CapabilityTask,
			Protocol:   domain.ProtocolNative,
			StartedAt:  c.GetTime("requestStartTime"),
		},
		Model: domain.ModelRoute{
			Capability: domain.CapabilityTask,
			Protocol:   domain.ProtocolNative,
			EndpointID: endpoint.ID,
		},
		Endpoints: []domain.Endpoint{endpoint},
	}
	runner := &midjourneySendRunner{
		provider:   mjProvider,
		timeout:    timeout,
		requestURL: requestURL,
	}
	engine := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	engine.MaxAttempts = 1 // request bodies and task submissions are not replayed implicitly.
	result := engine.Run(c.Request.Context(), plan, runner)
	if result.Err != nil {
		if runner.err != nil {
			return runner.response, runner.body, runner.err
		}
		return runner.response, runner.body, result.Err
	}
	return runner.response, runner.body, nil
}

func endpointForMidjourney(channel *model.Channel) domain.Endpoint {
	if channel == nil {
		return domain.Endpoint{}
	}
	weight := 0
	if channel.Weight != nil {
		weight = int(*channel.Weight)
	}
	priority := 0
	if channel.Priority != nil {
		priority = int(*channel.Priority)
	}
	return domain.Endpoint{
		ID:         channel.Id,
		ProviderID: domain.ProviderID(channel.Type),
		Name:       channel.Name,
		BaseURL:    channel.GetBaseURL(),
		Group:      channel.Group,
		Weight:     weight,
		Priority:   priority,
		Enabled:    true,
	}
}

type midjourneySendRunner struct {
	provider   *provider.MidjourneyProvider
	timeout    int
	requestURL string
	response   *provider.MidjourneyResponseWithStatusCode
	body       []byte
	err        error
}

func (r *midjourneySendRunner) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	r.response, r.body, r.err = r.provider.Send(r.timeout, r.requestURL)
	result := domain.AttemptResult{Attempt: attempt}
	if r.err != nil {
		return result, gatewayretry.ClassifyHTTP(statusCodeOf(r.response), r.err)
	}
	if r.response == nil {
		r.err = errors.New("midjourney provider returned an empty response")
		return result, gatewayretry.ClassifyHTTP(http.StatusBadGateway, r.err)
	}
	switch r.response.Response.Code {
	case 1, 21, 22:
		_ = session.MarkAccepted()
		result.UpstreamAccepted = true
	}
	if r.response.StatusCode >= http.StatusInternalServerError {
		r.err = fmt.Errorf("midjourney upstream status %d", r.response.StatusCode)
		return result, gatewayretry.ClassifyHTTP(r.response.StatusCode, r.err)
	}
	return result, nil
}

func statusCodeOf(response *provider.MidjourneyResponseWithStatusCode) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
