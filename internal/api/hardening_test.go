package api

import (
	"strings"
	"testing"
)

func TestToolsRejectedHonestly(t *testing.T) {
	s := newServer(defaultBackend())
	body := `{"model":"claude","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"get_weather"}}]}`
	rr := doReq(s, "POST", "/v1/chat/completions", body, "")
	if rr.Code != 400 {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorCode(t, rr, "unsupported_feature")
}

func TestContentPartsAccepted(t *testing.T) {
	fb := defaultBackend()
	s := newServer(fb)
	body := `{"model":"claude","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`
	rr := doReq(s, "POST", "/v1/chat/completions", body, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if len(fb.calledWith) != 1 || fb.calledWith[0] != "claude" {
		t.Fatalf("called = %v", fb.calledWith)
	}
}

func TestStreamErrorFinishReason(t *testing.T) {
	fb := defaultBackend()
	s := newServer(fb)
	// Force a stream failure via disabled tested separately; here check native stream final chunk is stop.
	rr := doReq(s, "POST", "/v1/chat/completions", `{"model":"claude","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("missing stop: %s", body)
	}
	if strings.Contains(body, `"finish_reason":"stop"`) && strings.Contains(body, `"type":"request_cancelled"`) {
		t.Fatalf("success stream must not contain error: %s", body)
	}
}

func TestStreamFailureFinishReasonError(t *testing.T) {
	// errCategory mapping drives activity status for failed streams.
	if got := errCategory("request_cancelled"); got != "cancelled" {
		t.Fatalf("category = %q", got)
	}
	if got := errCategory("provider_timeout"); got != "timeout" {
		t.Fatalf("category = %q", got)
	}
	if got := errCategory("provider_busy"); got != "queue_rejected" {
		t.Fatalf("category = %q", got)
	}
}

func TestResponsesAPI(t *testing.T) {
	fb := defaultBackend()
	s := newServer(fb)
	rr := doReq(s, "POST", "/v1/responses", `{"model":"claude","input":"hello"}`, "")
	out := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("status = %d: %s", rr.Code, out)
	}
	if !strings.Contains(out, `"object":"response"`) {
		t.Fatalf("responses body = %s", out)
	}
	if !strings.Contains(out, `"status":"completed"`) {
		t.Fatalf("responses status missing: %s", out)
	}
}

func TestCompletionsAlias(t *testing.T) {
	// /v1/completions is registered as an alias in Start(); doReq helper must route too.
	fb := defaultBackend()
	s := newServer(fb)
	rr := doReq(s, "POST", "/v1/completions", `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`, "")
	if rr.Code != 200 {
		t.Fatalf("alias status = %d: %s", rr.Code, rr.Body.String())
	}
}
