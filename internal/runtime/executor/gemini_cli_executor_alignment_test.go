package executor

import (
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGeminiCLIHeaders_DefaultKeepsLegacySurface(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://cloudcode-pa.googleapis.com/v1internal:generateContent", nil)
	if err != nil {
		t.Fatal(err)
	}

	applyGeminiCLIHeaders(req, "gemini-2.5-pro", false)

	if got := req.Header.Get("User-Agent"); strings.Contains(got, "; terminal)") {
		t.Fatalf("default Gemini CLI User-Agent should keep legacy shape, got %q", got)
	}
	if got := req.Header.Get("X-Goog-Api-Client"); got == "" {
		t.Fatalf("default Gemini CLI headers should include X-Goog-Api-Client")
	}
}

func TestGeminiCLIHeaders_OfficialAlignmentUsesOfficialSurface(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://cloudcode-pa.googleapis.com/v1internal:generateContent", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Goog-Api-Client", "legacy")

	applyGeminiCLIHeaders(req, "gemini-2.5-pro", true)

	if got := req.Header.Get("User-Agent"); !strings.Contains(got, "GeminiCLI/0.41.0-nightly.20260423.gaa05b4583/gemini-2.5-pro") || !strings.Contains(got, "; terminal)") {
		t.Fatalf("official Gemini CLI User-Agent = %q", got)
	}
	if got := req.Header.Get("X-Goog-Api-Client"); got != "" {
		t.Fatalf("official Gemini CLI headers should omit X-Goog-Api-Client, got %q", got)
	}
}

func TestAppendGeminiCLIAltParam_DefaultUsesDollarAlt(t *testing.T) {
	got := appendGeminiCLIAltParam("https://cloudcode-pa.googleapis.com/v1internal:generateContent", "sse", nil)
	if want := "https://cloudcode-pa.googleapis.com/v1internal:generateContent?$alt=sse"; got != want {
		t.Fatalf("default alt query = %q, want %q", got, want)
	}
}

func TestAppendGeminiCLIAltParam_OfficialAlignmentUsesAlt(t *testing.T) {
	got := appendGeminiCLIAltParam("https://cloudcode-pa.googleapis.com/v1internal:generateContent", "sse", &config.Config{GeminiCLIOfficialAlignment: true})
	if want := "https://cloudcode-pa.googleapis.com/v1internal:generateContent?alt=sse"; got != want {
		t.Fatalf("official alt query = %q, want %q", got, want)
	}
}
