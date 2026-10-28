package requester

import (
	"done-hub/internal/audit"
	"io"
	"net/http"

	"done-hub/internal/gateway/requeststate"
)

func doObservedRequest(req *http.Request) (*http.Response, error) {
	return (ObservedClient{Client: HTTPClient}).Do(req)
}

// ObservedClient also lets SDKs reuse the same passive HTTP observation point.
// The caller's client, proxy, timeouts and SDK decoding remain unchanged.
type ObservedClient struct{ Client *http.Client }

func (client ObservedClient) Do(req *http.Request) (*http.Response, error) {
	var timing *requeststate.UpstreamTiming
	if observeRequestProtocol(req) {
		timing = requeststate.From(req.Context()).BeginUpstreamRequest()
	}
	exchange := audit.BeginHTTP(req.Context())
	resp, err := client.Client.Do(req)
	if err == nil && timing != nil && resp.Body != nil {
		resp.Body = &timedResponseBody{ReadCloser: resp.Body, timing: timing}
	}
	exchange.Response(resp, err)
	return resp, err
}

type timedResponseBody struct {
	io.ReadCloser
	timing *requeststate.UpstreamTiming
}

func (b *timedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timing.RecordData()
	}
	return n, err
}
