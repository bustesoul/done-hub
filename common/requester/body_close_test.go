package requester

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type closeObservedBody struct {
	io.Reader
	closes int
}

func (b *closeObservedBody) Close() error { b.closes++; return nil }

type failedResponseReader struct{}

func (failedResponseReader) Read(p []byte) (int, error) {
	return copy(p, `{"ok":true}`), errors.New("upstream read failed")
}

func TestOutputResponseClosesOriginalBodyBeforeReplay(t *testing.T) {
	oldClient := HTTPClient
	defer func() { HTTPClient = oldClient }()
	for _, broken := range []bool{false, true} {
		var reader io.Reader = strings.NewReader(" {\"ok\":true} \n")
		if broken {
			reader = failedResponseReader{}
		}
		body := &closeObservedBody{Reader: reader}
		HTTPClient = &http.Client{Transport: timingTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
		})}
		client := NewHTTPRequester("", nil)
		req, _ := client.NewRequest("POST", "http://upstream/v1/chat/completions")
		var decoded map[string]bool
		resp, apiErr := client.SendRequest(req, &decoded, true)
		if body.closes != 1 {
			t.Fatal("original body leaked", broken, body.closes)
		}
		if broken {
			if apiErr == nil {
				t.Fatal("read failure hidden")
			}
			continue
		}
		if apiErr != nil || !decoded["ok"] {
			t.Fatal(decoded, apiErr)
		}
		raw, _ := io.ReadAll(resp.Body)
		if string(raw) != " {\"ok\":true} \n" {
			t.Fatal("raw replay changed", string(raw))
		}
		_ = resp.Body.Close()
		if body.closes != 1 {
			t.Fatal("original body closed twice")
		}
	}
}
