package middleware

import (
	"bytes"
	"context"
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/requester"
	"done-hub/internal/audit"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	"done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/types"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type auditRunner func(context.Context, domain.Attempt, *gatewaystream.Session) (domain.AttemptResult, *retry.UpstreamError)

func (r auditRunner) Run(ctx context.Context, a domain.Attempt, s *gatewaystream.Session) (domain.AttemptResult, *retry.UpstreamError) {
	return r(ctx, a, s)
}

func installTestAudit(t *testing.T, writer io.Writer, config audit.Config) *audit.Recorder {
	t.Helper()
	oldService, oldClient := audit.Default, requester.HTTPClient
	audit.Default = audit.NewService()
	audit.Default.Apply(config)
	r := audit.NewRecorder(writer)
	audit.Default.SetRecorder(r)
	requester.HTTPClient = http.DefaultClient
	t.Cleanup(func() { audit.Default, requester.HTTPClient = oldService, oldClient })
	return r
}

func closeAudit(t *testing.T, r *audit.Recorder) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func auditTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestId(), func(c *gin.Context) { c.Set("token_id", 7); c.Next() }, AuditLog())
	r.POST("/v1/chat/completions", handler)
	return r
}

func readAuditEvents(t *testing.T, output *bytes.Buffer) []audit.Event {
	t.Helper()
	decoder := json.NewDecoder(output)
	var events []audit.Event
	for decoder.More() {
		var event audit.Event
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestAuditHTTPRetryRecordsOriginalBeforeMapping(t *testing.T) {
	for _, channelOnly := range []bool{true, false} {
		t.Run(strconv.FormatBool(channelOnly), func(t *testing.T) {
			var output bytes.Buffer
			filter := audit.Config{TokenIDs: []int{7}}
			if channelOnly {
				filter.ChannelIDs = []int{25}
			}
			r := installTestAudit(t, &output, filter)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"model":"mapped"}` {
					t.Errorf("provider request altered: %s", body)
				}
				if r.Header.Get("X-Attempt") == "1" {
					w.WriteHeader(503)
					io.WriteString(w, "  {\"error\":\"busy\"} \n")
					return
				}
				io.WriteString(w, " {\"ok\":true} \n")
			}))
			defer upstream.Close()
			original := " {\"model\": \"original\", \"messages\": []} \n"
			router := auditTestRouter(func(c *gin.Context) {
				body, err := common.ReadBodyRaw(c)
				if err != nil || string(body) != original {
					t.Error("incoming body changed", err)
				}
				// Simulates the production pre_add cache replacement.
				c.Set(config.GinRequestBodyKey, []byte(`{"model":"mapped"}`))
				engine := execution.NewGatewayEngine(retry.DefaultPolicy())
				plan := domain.RoutePlan{Endpoints: []domain.Endpoint{{ID: 25}, {ID: 26}}}
				result := engine.Run(c.Request.Context(), plan, auditRunner(func(ctx context.Context, a domain.Attempt, _ *gatewaystream.Session) (domain.AttemptResult, *retry.UpstreamError) {
					client := requester.NewHTTPRequester("", func(resp *http.Response) *types.OpenAIError {
						body, _ := io.ReadAll(resp.Body)
						return &types.OpenAIError{Message: string(body)}
					})
					client.Context = ctx
					wire, _ := client.NewRequest("POST", upstream.URL+"/custom-provider-url", client.WithBody([]byte(`{"model":"mapped"}`)), client.WithHeader(map[string]string{"X-Attempt": strconv.Itoa(a.Number)}))
					resp, apiErr := client.SendRequestRaw(wire)
					if apiErr != nil {
						return domain.AttemptResult{}, retry.ClassifyHTTP(apiErr.StatusCode, nil)
					}
					defer resp.Body.Close()
					_, _ = io.Copy(c.Writer, resp.Body)
					return domain.AttemptResult{}, nil
				}))
				if result.Err != nil || len(result.Attempts) != 2 {
					t.Error("business retry changed", result.Err, len(result.Attempts))
				}
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(original)))
			if response.Code != 200 || response.Body.String() != " {\"ok\":true} \n" {
				t.Fatal("client response changed", response.Code, response.Body.String())
			}
			closeAudit(t, r)
			var inbound []byte
			responses := map[int][]byte{}
			for _, event := range readAuditEvents(t, &output) {
				if event.Type == "request_body" {
					inbound = append(inbound, event.Data...)
				}
				if event.Type == "response_body" {
					responses[event.Attempt] = append(responses[event.Attempt], event.Data...)
				}
				if event.Attempt == 1 && event.ChannelID != 25 || event.Attempt == 2 && event.ChannelID != 26 {
					t.Fatal("attempt channel attribution", event)
				}
			}
			if string(inbound) != original || string(responses[1]) != "  {\"error\":\"busy\"} \n" {
				t.Fatal("original/error bytes missing", string(inbound), responses)
			}
			if channelOnly && len(responses[2]) != 0 || !channelOnly && string(responses[2]) != " {\"ok\":true} \n" {
				t.Fatal("incorrect retry filtering", responses)
			}
		})
	}
}

func TestAuditSSEIsIncrementalAndPreservesRawBytes(t *testing.T) {
	var output bytes.Buffer
	r := installTestAudit(t, &output, audit.Config{TokenIDs: []int{7}})
	continueStream := make(chan struct{})
	var once sync.Once
	raw := "data:  first \r\n\r\n:heartbeat\n\ndata: second\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data:  first \r\n\r\n")
		w.(http.Flusher).Flush()
		select {
		case <-continueStream:
		case <-r.Context().Done():
			return
		case <-time.After(3 * time.Second):
			t.Error("SSE was buffered before parsing")
			return
		}
		io.WriteString(w, ":heartbeat\n\ndata: second\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	router := auditTestRouter(func(c *gin.Context) {
		_, _ = common.ReadBodyRaw(c)
		engine := execution.NewGatewayEngine(retry.DefaultPolicy())
		result := engine.Run(c.Request.Context(), domain.RoutePlan{Endpoints: []domain.Endpoint{{ID: 25}}}, auditRunner(func(ctx context.Context, _ domain.Attempt, _ *gatewaystream.Session) (domain.AttemptResult, *retry.UpstreamError) {
			client := requester.NewHTTPRequester("", nil)
			client.Context = ctx
			req, _ := client.NewRequest("POST", upstream.URL+"/v1/chat/completions", client.WithBody([]byte(`{}`)))
			resp, apiErr := client.SendRequestRaw(req)
			if apiErr != nil {
				t.Error(apiErr)
				return domain.AttemptResult{}, retry.ClassifyHTTP(500, nil)
			}
			stream, apiErr := requester.RequestStream(client, resp, func(line *[]byte, data chan string, _ chan error) {
				if string(*line) == "data: [DONE]" {
					*line = requester.StreamClosed
					return
				}
				data <- string(*line)
			})
			if apiErr != nil {
				t.Error(apiErr)
			}
			defer stream.Close()
			data, errs := stream.Recv()
			for data != nil || errs != nil {
				select {
				case text, ok := <-data:
					if !ok {
						data = nil
						continue
					}
					once.Do(func() { close(continueStream) })
					io.WriteString(c.Writer, strings.ToUpper(text)+"\n")
				case err, ok := <-errs:
					if !ok {
						errs = nil
						continue
					}
					if !errors.Is(err, io.EOF) {
						t.Error(err)
					}
				}
			}
			return domain.AttemptResult{}, nil
		}))
		if result.Err != nil {
			t.Error(result.Err)
		}
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`)))
	closeAudit(t, r)
	var observed []byte
	for _, event := range readAuditEvents(t, &output) {
		if event.Type == "response_body" {
			observed = append(observed, event.Data...)
		}
	}
	if string(observed) != raw || !strings.Contains(response.Body.String(), "DATA: SECOND") {
		t.Fatal("SSE raw bytes or business parsing changed", string(observed), response.Body.String())
	}
}

type auditFailedWriter struct{}

func (auditFailedWriter) Write([]byte) (int, error) { return 0, errors.New("disk unavailable") }

func TestAuditWriteFailureDoesNotChangeHTTPResult(t *testing.T) {
	r := installTestAudit(t, auditFailedWriter{}, audit.Config{ChannelIDs: []int{25}, TokenIDs: []int{7}})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"result":"ok"}`) }))
	defer upstream.Close()
	router := auditTestRouter(func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		finish := audit.BeginAttempt(c.Request.Context(), 1, 25)
		defer finish()
		client := requester.NewHTTPRequester("", nil)
		client.Context = c.Request.Context()
		req, _ := client.NewRequest("POST", upstream.URL+"/v1/chat/completions")
		resp, apiErr := client.SendRequestRaw(req)
		if apiErr != nil {
			t.Error(apiErr)
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(c.Writer, resp.Body)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`)))
	closeAudit(t, r)
	if response.Code != 200 || response.Body.String() != `{"result":"ok"}` || r.Status().WriteErrors == 0 {
		t.Fatal("audit failure changed business response", response.Code, response.Body.String(), r.Status())
	}
}
