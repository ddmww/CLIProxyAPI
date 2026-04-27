package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestAntigravityBuildRequest_SanitizesGeminiToolSchema(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "gemini-2.5-pro")

	decl := extractFirstFunctionDeclaration(t, body)
	if _, ok := decl["parametersJsonSchema"]; ok {
		t.Fatalf("parametersJsonSchema should be renamed to parameters")
	}

	params, ok := decl["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or invalid type")
	}
	assertSchemaSanitizedAndPropertyPreserved(t, params)
}

func TestAntigravityBuildRequest_SanitizesAntigravityToolSchema(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "claude-opus-4-6")

	decl := extractFirstFunctionDeclaration(t, body)
	params, ok := decl["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or invalid type")
	}
	assertSchemaSanitizedAndPropertyPreserved(t, params)
}

func TestAntigravityBuildRequest_SkipsSchemaSanitizationWithoutToolsField(t *testing.T) {
	body := buildRequestBodyFromRawPayload(t, "gemini-3.1-flash-image", []byte(`{
		"request": {
			"contents": [
				{
					"role": "user",
					"x-debug": "keep-me",
					"parts": [
						{
							"text": "hello"
						}
					]
				}
			],
			"nonSchema": {
				"nullable": true,
				"x-extra": "keep-me"
			},
			"generationConfig": {
				"maxOutputTokens": 128
			}
		}
	}`))

	assertNonSchemaRequestPreserved(t, body)
}

func TestAntigravityBuildRequest_SkipsSchemaSanitizationWithEmptyToolsArray(t *testing.T) {
	body := buildRequestBodyFromRawPayload(t, "gemini-3.1-flash-image", []byte(`{
		"request": {
			"tools": [],
			"contents": [
				{
					"role": "user",
					"x-debug": "keep-me",
					"parts": [
						{
							"text": "hello"
						}
					]
				}
			],
			"nonSchema": {
				"nullable": true,
				"x-extra": "keep-me"
			},
			"generationConfig": {
				"maxOutputTokens": 128
			}
		}
	}`))

	assertNonSchemaRequestPreserved(t, body)
}

func TestAntigravityBuildRequest_DefaultAlignmentPreservesLegacyEnvelope(t *testing.T) {
	req, body := buildRequestFromRawPayload(t, &AntigravityExecutor{}, "gemini-2.5-pro", []byte(`{
		"project": "caller-project",
		"requestId": "caller-request",
		"userAgent": "caller-agent",
		"requestType": "caller-type",
		"request": {
			"sessionId": "caller-session",
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
		}
	}`), true, "sse")

	if got := req.URL.RawQuery; got != "$alt=sse" {
		t.Fatalf("default alignment query = %q, want $alt=sse", got)
	}
	if got := req.Header.Get("User-Agent"); !strings.Contains(got, "darwin/arm64") {
		t.Fatalf("default User-Agent should keep legacy platform, got %q", got)
	}
	if got := body["project"]; got == "caller-project" {
		t.Fatalf("legacy envelope should overwrite caller project")
	}
	if got := body["requestId"]; got == "caller-request" {
		t.Fatalf("legacy envelope should overwrite caller requestId")
	}
	if got := body["userAgent"]; got != "antigravity" {
		t.Fatalf("legacy envelope userAgent = %v, want antigravity", got)
	}
	if _, ok := body["enabledCreditTypes"]; ok {
		t.Fatalf("legacy envelope should not inject enabledCreditTypes")
	}
}

func TestAntigravityBuildRequest_OfficialAlignmentPreservesEnvelope(t *testing.T) {
	req, body := buildRequestFromRawPayload(t, &AntigravityExecutor{cfg: &config.Config{AntigravityOfficialAlignment: true}}, "gemini-2.5-pro", []byte(`{
		"project": "caller-project",
		"requestId": "caller-request",
		"userAgent": "caller-agent",
		"requestType": "caller-type",
		"request": {
			"sessionId": "caller-session",
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
		}
	}`), true, "sse")

	if got := req.URL.RawQuery; got != "alt=sse" {
		t.Fatalf("official alignment query = %q, want alt=sse", got)
	}
	if got := req.Header.Get("User-Agent"); !strings.Contains(got, "windows/amd64") {
		t.Fatalf("official alignment User-Agent should use Windows app platform, got %q", got)
	}
	for key, want := range map[string]string{
		"project":     "caller-project",
		"requestId":   "caller-request",
		"userAgent":   "caller-agent",
		"requestType": "caller-type",
	} {
		if got := body[key]; got != want {
			t.Fatalf("official alignment %s = %v, want %s", key, got, want)
		}
	}
	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}
	if got := request["sessionId"]; got != "caller-session" {
		t.Fatalf("official alignment request.sessionId = %v, want caller-session", got)
	}
	credits, ok := body["enabledCreditTypes"].([]any)
	if !ok || len(credits) != 1 || credits[0] != "GOOGLE_ONE_AI" {
		t.Fatalf("official alignment enabledCreditTypes = %#v, want [GOOGLE_ONE_AI]", body["enabledCreditTypes"])
	}
}

func TestNewAntigravityHTTPClient_DefaultForcesHTTP11(t *testing.T) {
	client := newAntigravityHTTPClient(context.Background(), nil, nil, 0)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatalf("default client should disable ForceAttemptHTTP2")
	}
	if transport.TLSClientConfig == nil {
		t.Fatalf("default client should configure TLS ALPN")
	}
	if got := strings.Join(transport.TLSClientConfig.NextProtos, ","); got != "http/1.1" {
		t.Fatalf("default client ALPN = %q, want http/1.1", got)
	}
	if transport.TLSNextProto == nil {
		t.Fatalf("default client should set empty TLSNextProto map to block implicit HTTP/2")
	}
}

func TestNewAntigravityHTTPClient_OfficialAlignmentKeepsDefaultTransport(t *testing.T) {
	client := newAntigravityHTTPClient(context.Background(), &config.Config{AntigravityOfficialAlignment: true}, nil, 0)

	if client.Transport != nil {
		t.Fatalf("official alignment without proxy should keep nil transport for http.DefaultTransport, got %T", client.Transport)
	}
}

func TestNewAntigravityHTTPClient_OfficialAlignmentKeepsProxyTransportHTTP2Capable(t *testing.T) {
	client := newAntigravityHTTPClient(context.Background(), &config.Config{
		AntigravityOfficialAlignment: true,
		SDKConfig:                    config.SDKConfig{ProxyURL: "http://127.0.0.1:7897"},
	}, nil, 0)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("official alignment proxy transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatalf("official alignment proxy transport should preserve proxy settings")
	}
	if transport.TLSClientConfig != nil && strings.Join(transport.TLSClientConfig.NextProtos, ",") == "http/1.1" {
		t.Fatalf("official alignment proxy transport should not force ALPN to http/1.1")
	}
	if transport.TLSNextProto != nil && len(transport.TLSNextProto) == 0 {
		t.Fatalf("official alignment proxy transport should not install empty TLSNextProto map")
	}
}

func assertNonSchemaRequestPreserved(t *testing.T, body map[string]any) {
	t.Helper()

	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}

	contents, ok := request["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("contents missing or empty")
	}
	content, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("content missing or invalid type")
	}
	if got, ok := content["x-debug"].(string); !ok || got != "keep-me" {
		t.Fatalf("x-debug should be preserved when no tool schema exists, got=%v", content["x-debug"])
	}

	nonSchema, ok := request["nonSchema"].(map[string]any)
	if !ok {
		t.Fatalf("nonSchema missing or invalid type")
	}
	if _, ok := nonSchema["nullable"]; !ok {
		t.Fatalf("nullable should be preserved outside schema cleanup path")
	}
	if got, ok := nonSchema["x-extra"].(string); !ok || got != "keep-me" {
		t.Fatalf("x-extra should be preserved outside schema cleanup path, got=%v", nonSchema["x-extra"])
	}

	if generationConfig, ok := request["generationConfig"].(map[string]any); ok {
		if _, ok := generationConfig["maxOutputTokens"]; ok {
			t.Fatalf("maxOutputTokens should still be removed for non-Claude requests")
		}
	}
}

func buildRequestBodyFromPayload(t *testing.T, modelName string) map[string]any {
	t.Helper()
	return buildRequestBodyFromRawPayload(t, modelName, []byte(`{
		"request": {
			"tools": [
				{
					"function_declarations": [
						{
							"name": "tool_1",
							"parametersJsonSchema": {
								"$schema": "http://json-schema.org/draft-07/schema#",
								"$id": "root-schema",
								"type": "object",
								"properties": {
									"$id": {"type": "string"},
									"arg": {
										"type": "object",
										"prefill": "hello",
										"properties": {
											"mode": {
												"type": "string",
												"deprecated": true,
												"enum": ["a", "b"],
												"enumTitles": ["A", "B"]
											}
										}
									}
								},
								"patternProperties": {
									"^x-": {"type": "string"}
								}
							}
						}
					]
				}
			]
		}
	}`))
}

func buildRequestBodyFromRawPayload(t *testing.T, modelName string, payload []byte) map[string]any {
	t.Helper()

	_, body := buildRequestFromRawPayload(t, &AntigravityExecutor{}, modelName, payload, false, "")
	return body
}

func buildRequestFromRawPayload(t *testing.T, executor *AntigravityExecutor, modelName string, payload []byte, stream bool, alt string) (*http.Request, map[string]any) {
	t.Helper()

	auth := &cliproxyauth.Auth{}

	req, err := executor.buildRequest(context.Background(), auth, "token", modelName, payload, stream, alt, "https://example.com")
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body error: %v, body=%s", err, string(raw))
	}
	return req, body
}

func extractFirstFunctionDeclaration(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}
	tools, ok := request["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools missing or empty")
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("first tool invalid type")
	}
	decls, ok := tool["function_declarations"].([]any)
	if !ok || len(decls) == 0 {
		t.Fatalf("function_declarations missing or empty")
	}
	decl, ok := decls[0].(map[string]any)
	if !ok {
		t.Fatalf("first function declaration invalid type")
	}
	return decl
}

func assertSchemaSanitizedAndPropertyPreserved(t *testing.T, params map[string]any) {
	t.Helper()

	if _, ok := params["$id"]; ok {
		t.Fatalf("root $id should be removed from schema")
	}
	if _, ok := params["patternProperties"]; ok {
		t.Fatalf("patternProperties should be removed from schema")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing or invalid type")
	}
	if _, ok := props["$id"]; !ok {
		t.Fatalf("property named $id should be preserved")
	}

	arg, ok := props["arg"].(map[string]any)
	if !ok {
		t.Fatalf("arg property missing or invalid type")
	}
	if _, ok := arg["prefill"]; ok {
		t.Fatalf("prefill should be removed from nested schema")
	}

	argProps, ok := arg["properties"].(map[string]any)
	if !ok {
		t.Fatalf("arg.properties missing or invalid type")
	}
	mode, ok := argProps["mode"].(map[string]any)
	if !ok {
		t.Fatalf("mode property missing or invalid type")
	}
	if _, ok := mode["enumTitles"]; ok {
		t.Fatalf("enumTitles should be removed from nested schema")
	}
	if _, ok := mode["deprecated"]; ok {
		t.Fatalf("deprecated should be removed from nested schema")
	}
}
