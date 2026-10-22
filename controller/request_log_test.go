package controller

import (
	"done-hub/model"
	"testing"
)

func TestRequestLogCSVFields(t *testing.T) {
	pending := &model.Log{Type: model.LogTypeRequest, RequestStatus: "processing", StartedAt: 1792612800000, InboundProtocol: "openai_chat", UpstreamProtocol: "openai_responses"}
	fields := requestLogCSVFields(pending)
	if fields[1] != "处理中" || fields[2] != "Chat → Responses" || fields[3] != "-" {
		t.Fatalf("fields=%v", fields)
	}
	admin, user := formatLogToCSVRow(pending, true), formatLogToCSVRow(pending, false)
	if len(admin) != 17 || len(user) != 15 {
		t.Fatalf("column counts: %d %d", len(admin), len(user))
	}
	for _, index := range []int{7, 8, 9, 10} {
		if admin[index] != "-" {
			t.Errorf("unknown value=%q", admin[index])
		}
	}
	pending.UpstreamProtocol = "openai_chat"
	if requestLogCSVFields(pending)[2] != "Chat" {
		t.Fatal("direct protocol should only appear once")
	}
	old := requestLogCSVFields(&model.Log{})
	for _, value := range old {
		if value != "-" {
			t.Fatal("historical metadata must remain unknown")
		}
	}
}
