package controller

import (
	"done-hub/types"
	"testing"
)

func TestClassifyResponsesReasoningCapability(t *testing.T) {
	t.Run("visible summary wins over opaque data", func(t *testing.T) {
		encrypted := "opaque"
		report := classifyResponsesReasoningCapability(&types.OpenAIResponsesResponses{
			Output: []types.ResponsesOutput{{
				Type:             types.InputTypeReasoning,
				EncryptedContent: &encrypted,
				Summary:          []types.SummaryResponses{{Type: types.ContentTypeSummaryText, Text: "visible"}},
			}},
		}, nil)
		if report.Visibility != reasoningVisibilityVisible || report.Source != "summary" {
			t.Fatalf("unexpected report: %#v", report)
		}
	})

	t.Run("opaque encrypted content stays opaque", func(t *testing.T) {
		encrypted := "opaque"
		report := classifyResponsesReasoningCapability(&types.OpenAIResponsesResponses{
			Output: []types.ResponsesOutput{{Type: types.InputTypeReasoning, EncryptedContent: &encrypted}},
		}, nil)
		if report.Visibility != reasoningVisibilityOpaque || report.Source != "encrypted_content" {
			t.Fatalf("unexpected report: %#v", report)
		}
	})

	t.Run("reasoning tokens without summary are opaque", func(t *testing.T) {
		report := classifyResponsesReasoningCapability(&types.OpenAIResponsesResponses{
			Usage: &types.ResponsesUsage{OutputTokensDetails: &types.ResponsesUsageOutputTokensDetails{ReasoningTokens: 4}},
		}, nil)
		if report.Visibility != reasoningVisibilityOpaque || report.Source != "reasoning_tokens" {
			t.Fatalf("unexpected report: %#v", report)
		}
	})

	t.Run("ordinary response remains valid and unknown", func(t *testing.T) {
		report := classifyResponsesReasoningCapability(&types.OpenAIResponsesResponses{
			Output: []types.ResponsesOutput{{Type: types.InputTypeMessage}},
		}, &types.Usage{})
		if report.Visibility != reasoningVisibilityUnknown || report.Source != "not_observed" {
			t.Fatalf("unexpected report: %#v", report)
		}
	})
}

func TestClassifyChatReasoningCapability(t *testing.T) {
	visible := classifyChatReasoningCapability(&types.ChatCompletionResponse{
		Choices: []types.ChatCompletionChoice{{Message: types.ChatCompletionMessage{ReasoningContent: "thinking"}}},
	}, nil)
	if visible.Visibility != reasoningVisibilityVisible || visible.Source != "reasoning_content" {
		t.Fatalf("unexpected visible report: %#v", visible)
	}

	opaque := classifyChatReasoningCapability(&types.ChatCompletionResponse{
		Usage: &types.Usage{CompletionTokensDetails: types.CompletionTokensDetails{ReasoningTokens: 3}},
	}, nil)
	if opaque.Visibility != reasoningVisibilityOpaque || opaque.Source != "reasoning_tokens" {
		t.Fatalf("unexpected opaque report: %#v", opaque)
	}

	unknown := classifyChatReasoningCapability(&types.ChatCompletionResponse{}, &types.Usage{})
	if unknown.Visibility != reasoningVisibilityUnknown || unknown.Source != "not_observed" {
		t.Fatalf("unexpected unknown report: %#v", unknown)
	}
}
