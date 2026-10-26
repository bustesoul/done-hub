package vertexai

import (
	"net/http"
	"testing"

	"done-hub/model"
)

func TestVertexAIRequestURLIgnoresStaleChannelBaseURL(t *testing.T) {
	t.Parallel()

	for _, region := range []string{"global", "asia-southeast1"} {
		for _, staleBaseURL := range []string{"", "https://api.openai.com", "https://generativelanguage.googleapis.com/"} {
			for _, action := range []string{"generateContent", "streamGenerateContent?alt=sse", "countTokens", "predict"} {
				t.Run(region+"/"+staleBaseURL+"/"+action, func(t *testing.T) {
					channel := &model.Channel{BaseURL: &staleBaseURL, Other: region + "|test-project"}
					provider := (VertexAIProviderFactory{}).CreateAdapter(channel).(*VertexAIProvider)
					got := provider.GetFullRequestURL("gemini-4-argon", action)
					host := "aiplatform.googleapis.com"
					if region != "global" {
						host = region + "-" + host
					}
					want := "https://" + host + "/v1/projects/test-project/locations/" + region + "/publishers/google/models/gemini-4-argon:" + action
					if got != want {
						t.Fatalf("request URL: got %q, want %q", got, want)
					}
					if _, err := http.NewRequest(http.MethodPost, got, nil); err != nil {
						t.Fatalf("request URL is invalid: %v", err)
					}
					if channel.GetBaseURL() != staleBaseURL {
						t.Fatal("building the request mutated the channel")
					}
				})
			}
		}
	}
}
