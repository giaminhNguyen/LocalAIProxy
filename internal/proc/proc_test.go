package proc

import (
	"os/exec"
	"testing"

	"LocalAIProxy/internal/provider"
)

func TestClassifyExitErrorPatterns(t *testing.T) {
	err := &exec.ExitError{}
	cases := []struct {
		code string
		want string
	}{
		{`You've hit your weekly limit · resets Sep 14`, "provider_rate_limited"},
		{"monthly quota exceeded, please wait", "provider_rate_limited"},
		{"429 Too Many Requests", "provider_rate_limited"},
		{"401 unauthorized", "provider_authentication_error"},
		{"not logged in", "provider_authentication_error"},
		{"permission denied for that tool", "provider_authentication_error"},
		{"billing problem", "provider_authentication_error"},
		{"some random failure", "provider_process_error"},
	}
	for _, tc := range cases {
		e := classifyExitError(err, []byte(tc.code), "")
		if e.Code != tc.want {
			t.Errorf("input %q -> code %q, want %q", tc.code, e.Code, tc.want)
		}
		if e.Details != tc.code {
			t.Errorf("details = %q, want sanitized input", e.Details)
		}
	}
}

func TestSanitizeRedactsHome(t *testing.T) {
	out := sanitize(`error at C:\Users\ming\AppData\probe`, `C:\Users\ming`)
	if out != `error at <user>\AppData\probe` {
		t.Fatalf("sanitize = %q", out)
	}
}

func TestClassifyExitIsRateLimitedByStatusCode(t *testing.T) {
	// claude api_error_status=429 hides inside stdout JSON; the string match
	// on result text must land on the rate-limit branch.
	e := classifyExitError(&exec.ExitError{}, []byte(`{"is_error":true,"api_error_status":429,"result":"You've hit your weekly limit"}`), "")
	if e.Code != provider.ErrProviderRateLimited {
		t.Fatalf("code = %q", e.Code)
	}
	if e.Status != 429 {
		t.Fatalf("status = %d", e.Status)
	}
}
