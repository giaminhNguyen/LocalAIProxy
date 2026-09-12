package main

import (
	"os"
	"strings"
	"testing"
)

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestFrontendTabs ensures the 5-tab IA exists (Dashboard/Models/Providers/Activity/Settings).
func TestFrontendTabs(t *testing.T) {
	html := mustRead(t, "frontend/dist/index.html")
	for _, id := range []string{"tab-dashboard", "tab-models", "tab-providers", "tab-activity", "tab-settings",
		"view-dashboard", "view-models", "view-providers", "view-activity", "view-settings"} {
		if !strings.Contains(html, id) {
			t.Errorf("index.html missing %q", id)
		}
	}
}

// TestFrontendModelEditor ensures the model editor + connect helper fields exist.
func TestFrontendModelEditor(t *testing.T) {
	html := mustRead(t, "frontend/dist/index.html")
	for _, id := range []string{"in-model-id", "in-model-provider", "in-model-upstream",
		"in-model-timeout", "in-model-stream", "in-model-system", "in-model-temp",
		"in-model-maxtok", "in-model-ctx", "model-caps", "connect-backdrop",
		"connect-client", "connect-snippet", "in-global-conc", "load-line"} {
		if !strings.Contains(html, id) {
			t.Errorf("index.html missing %q", id)
		}
	}
}

// TestFrontendA11y ensures dialogs, focus styles and reduced-motion guards exist.
func TestFrontendA11y(t *testing.T) {
	html := mustRead(t, "frontend/dist/index.html")
	for _, s := range []string{`role="dialog"`, "aria-modal", "aria-checked", `role="tablist"`} {
		if !strings.Contains(html, s) {
			t.Errorf("index.html missing a11y marker %q", s)
		}
	}
	css := mustRead(t, "frontend/dist/css/app.css")
	for _, s := range []string{"prefers-reduced-motion", ":focus-visible", "--font-mono"} {
		if !strings.Contains(css, s) {
			t.Errorf("app.css missing %q", s)
		}
	}
	js := mustRead(t, "frontend/dist/js/app.js")
	for _, s := range []string{"openConnect", "renderConnectSnippet", "renderActivityFull",
		"renderModelsFull", "activeRequests", "contextWindow"} {
		if !strings.Contains(js, s) {
			t.Errorf("app.js missing %q", s)
		}
	}
}
