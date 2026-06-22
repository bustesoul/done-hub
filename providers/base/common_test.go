package base

import (
	"done-hub/model"
	"testing"
)

func TestApplyCustomHeadersOverwriteRemovesCaseInsensitiveDuplicate(t *testing.T) {
	modelHeaders := `{"content-type":"text/plain"}`
	provider := &BaseProvider{
		Channel: &model.Channel{ModelHeaders: &modelHeaders},
	}
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	provider.ApplyCustomHeaders(headers)

	if _, exists := headers["Content-Type"]; exists {
		t.Fatalf("expected canonical duplicate to be removed: %#v", headers)
	}
	if got := headers["content-type"]; got != "text/plain" {
		t.Fatalf("expected custom content-type to win, got %q from %#v", got, headers)
	}
	if matches := matchingHeaderKeys(headers, "Content-Type"); len(matches) != 1 {
		t.Fatalf("expected exactly one content-type key, got %#v in %#v", matches, headers)
	}
}

func TestApplyCustomHeadersSkipUsesCaseInsensitiveExistingHeader(t *testing.T) {
	modelHeaders := `{"content-type":{"value":"text/plain","skip":true}}`
	provider := &BaseProvider{
		Channel: &model.Channel{ModelHeaders: &modelHeaders},
	}
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	provider.ApplyCustomHeaders(headers)

	if got := headers["Content-Type"]; got != "application/json" {
		t.Fatalf("expected existing Content-Type to be preserved, got %q from %#v", got, headers)
	}
	if _, exists := headers["content-type"]; exists {
		t.Fatalf("skip should not create duplicate content-type key: %#v", headers)
	}
}
