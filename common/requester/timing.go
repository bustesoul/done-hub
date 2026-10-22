package requester

import (
	"io"
	"net/http"

	"done-hub/internal/gateway/requeststate"
)

func doObservedRequest(req *http.Request) (*http.Response, error) {
	var timing *requeststate.UpstreamTiming
	if observeRequestProtocol(req) {
		timing = requeststate.From(req.Context()).BeginUpstreamRequest()
	}
	resp, err := HTTPClient.Do(req)
	if err == nil && timing != nil && resp.Body != nil {
		resp.Body = &timedResponseBody{ReadCloser: resp.Body, timing: timing}
	}
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
