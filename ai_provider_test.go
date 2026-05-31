package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnit_NormalizeAPIFormat(t *testing.T) {
	if got := normalizeAPIFormat("chat"); got != AIAPIFormatChatCompletions {
		t.Fatalf("expected chat alias to normalize to chat_completions, got %s", got)
	}
	if got := normalizeAPIFormat("responses"); got != AIAPIFormatResponses {
		t.Fatalf("expected responses to normalize to responses, got %s", got)
	}
	if got := normalizeAPIFormat("unknown"); got != "" {
		t.Fatalf("expected unknown format to normalize to empty, got %s", got)
	}
}

func TestUnit_NormalizeProviderBaseURL(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{input: "", want: "https://api.openai.com/v1"},
		{input: "https://www.ananapi.com/", want: "https://www.ananapi.com"},
		{input: "https://api.openai.com/v1/", want: "https://api.openai.com/v1"},
		{input: "http://127.0.0.1:48760/v1/", want: "http://127.0.0.1:48760/v1"},
	}

	for _, testCase := range testCases {
		if got := normalizeProviderBaseURL(testCase.input); got != testCase.want {
			t.Fatalf("normalizeProviderBaseURL(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}
}

func TestUnit_ExtractResponsesText(t *testing.T) {
	raw := map[string]any{
		"output": []any{
			map[string]any{
				"content": []any{
					map[string]any{"type": "output_text", "text": "{\"ok\":true}"},
				},
			},
		},
	}
	text, err := extractResponsesText(raw)
	if err != nil {
		t.Fatalf("extractResponsesText returned error: %v", err)
	}
	if text != "{\"ok\":true}" {
		t.Fatalf("unexpected responses text: %s", text)
	}
}

func TestUnit_ExtractResponsesText_PrefersNestedOutputOverDuplicatedOutputText(t *testing.T) {
	raw := map[string]any{
		"output_text": "{\"ok\":true}{\"ok\":true}{\"ok\":true}",
		"output": []any{
			map[string]any{
				"content": []any{
					map[string]any{"type": "output_text", "text": "{\"ok\":true}"},
				},
			},
		},
	}
	text, err := extractResponsesText(raw)
	if err != nil {
		t.Fatalf("extractResponsesText returned error: %v", err)
	}
	if text != "{\"ok\":true}" {
		t.Fatalf("expected nested content to win over duplicated output_text, got %s", text)
	}
}

func TestUnit_ExtractChatCompletionsText_ArrayContent(t *testing.T) {
	raw := map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": []any{
						map[string]any{"text": "第一段"},
						map[string]any{"text": "第二段"},
					},
				},
			},
		},
	}
	text, err := extractChatCompletionsText(raw)
	if err != nil {
		t.Fatalf("extractChatCompletionsText returned error: %v", err)
	}
	if text != "第一段\n第二段" {
		t.Fatalf("unexpected chat text: %s", text)
	}
}

func TestUnit_RepositoryPersistsProviderAPIFormat(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	defer func() {
		_ = repo.Close()
	}()

	config, err := repo.SaveProviderConfig(context.Background(), ProviderConfig{
		Name:         "Responses Provider",
		ProviderType: "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatResponses,
		Enabled:      true,
		UseMock:      false,
	})
	if err != nil {
		t.Fatalf("SaveProviderConfig returned error: %v", err)
	}
	if config.APIFormat != AIAPIFormatResponses {
		t.Fatalf("expected saved api format to be responses, got %s", config.APIFormat)
	}

	stored, err := repo.GetProviderConfig(context.Background())
	if err != nil {
		t.Fatalf("GetProviderConfig returned error: %v", err)
	}
	if stored.APIFormat != AIAPIFormatResponses {
		t.Fatalf("expected stored api format to be responses, got %s", stored.APIFormat)
	}
}

func TestUnit_RepositoryPreservesRootBaseURL(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	defer func() {
		_ = repo.Close()
	}()

	config, err := repo.SaveProviderConfig(context.Background(), ProviderConfig{
		Name:         "Root Provider",
		ProviderType: "openai-compatible",
		BaseURL:      "https://www.ananapi.com/",
		APIKey:       "test-key",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatChatCompletions,
		Enabled:      true,
		UseMock:      false,
	})
	if err != nil {
		t.Fatalf("SaveProviderConfig returned error: %v", err)
	}
	if config.BaseURL != "https://www.ananapi.com" {
		t.Fatalf("expected root base url to be preserved, got %s", config.BaseURL)
	}
}

func TestIntegration_OpenAICompatibleProvider_ChatCompletions(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions path, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "{\"artifactType\":\"PRD\",\"title\":\"Demo\",\"summary\":\"ok\",\"sections\":[{\"id\":\"background\",\"title\":\"背景\",\"body\":\"内容\",\"html\":\"<p>内容</p>\"}]}",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Chat Provider",
			BaseURL:      server.URL,
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatChatCompletions,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	payload, err := provider.GenerateStructured(context.Background(), "prd", "{\"project_name\":\"demo\"}")
	if err != nil {
		t.Fatalf("GenerateStructured returned error: %v", err)
	}
	if !strings.Contains(payload, "\"artifactType\":\"PRD\"") {
		t.Fatalf("expected JSON payload, got %s", payload)
	}
	if requestBody["model"] != "gpt-4.1-mini" {
		t.Fatalf("expected model to be forwarded, got %v", requestBody["model"])
	}
}

func TestIntegration_OpenAICompatibleProvider_FallsBackFromRootBaseURLToV1(t *testing.T) {
	var rootAttempted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			rootAttempted = true
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not api</html>"))
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected fallback /v1/chat/completions path, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "{\"artifactType\":\"PRD\",\"title\":\"Demo\",\"summary\":\"ok\",\"sections\":[{\"id\":\"background\",\"title\":\"背景\",\"body\":\"内容\",\"html\":\"<p>内容</p>\"}]}",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Chat Provider",
			BaseURL:      server.URL + "/",
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatChatCompletions,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	if _, err := provider.GenerateStructured(context.Background(), "prd", "{\"project_name\":\"demo\"}"); err != nil {
		t.Fatalf("GenerateStructured returned error: %v", err)
	}
	if !rootAttempted {
		t.Fatalf("expected provider to try the root endpoint before falling back to /v1")
	}
}

func TestIntegration_OpenAICompatibleProvider_Responses(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"content": []map[string]any{
						{"type": "output_text", "text": "{\"summary\":{\"productName\":\"demo\"},\"questions\":[{\"id\":\"q1\",\"question\":\"目标是什么？\",\"rationale\":\"补齐范围\",\"priority\":\"high\"}]}"},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Responses Provider",
			BaseURL:      server.URL,
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatResponses,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	payload, err := provider.GenerateStructured(context.Background(), "clarifications", "{\"project_name\":\"demo\"}")
	if err != nil {
		t.Fatalf("GenerateStructured returned error: %v", err)
	}
	if !strings.Contains(payload, "\"questions\"") {
		t.Fatalf("expected responses JSON payload, got %s", payload)
	}
	if requestBody["model"] != "gpt-4.1-mini" {
		t.Fatalf("expected model to be forwarded, got %v", requestBody["model"])
	}
	if _, ok := requestBody["instructions"].(string); !ok {
		t.Fatalf("expected responses request to contain instructions, got %+v", requestBody)
	}
}

func TestIntegration_OpenAICompatibleProvider_ResponsesWithDuplicatedOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_text": "{\"summary\":{\"productName\":\"demo\"}}{\"summary\":{\"productName\":\"demo\"}}",
			"output": []map[string]any{
				{
					"content": []map[string]any{
						{"type": "output_text", "text": "{\"summary\":{\"productName\":\"demo\"},\"questions\":[{\"id\":\"q1\",\"question\":\"目标是什么？\",\"rationale\":\"补齐范围\",\"priority\":\"high\"}]}"},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Responses Provider",
			BaseURL:      server.URL,
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatResponses,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	payload, err := provider.GenerateStructured(context.Background(), "clarifications", "{\"project_name\":\"demo\"}")
	if err != nil {
		t.Fatalf("GenerateStructured returned error: %v", err)
	}
	if !strings.Contains(payload, "\"questions\"") {
		t.Fatalf("expected responses JSON payload, got %s", payload)
	}
}

func TestIntegration_OpenAICompatibleProvider_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name      string
		apiFormat AIAPIFormat
		handler   func(w http.ResponseWriter, r *http.Request)
		wantErr   string
	}{
		{
			name:      "chat provider error message",
			apiFormat: AIAPIFormatChatCompletions,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"message": "chat failed"},
				})
			},
			wantErr: "chat failed",
		},
		{
			name:      "responses empty output",
			apiFormat: AIAPIFormatResponses,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{}})
			},
			wantErr: "responses api output array is empty",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(testCase.handler))
			defer server.Close()

			provider := &OpenAICompatibleProvider{
				config: ProviderConfig{
					Name:         "Test Provider",
					BaseURL:      server.URL,
					APIKey:       "test-key",
					Model:        "gpt-4.1-mini",
					APIFormat:    testCase.apiFormat,
					Enabled:      true,
					ProviderType: "openai-compatible",
				},
				httpClient: server.Client(),
			}

			_, err := provider.GenerateStructured(context.Background(), "prd", "{\"project_name\":\"demo\"}")
			if err == nil {
				t.Fatalf("expected GenerateStructured to fail")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("expected error to contain %q, got %v", testCase.wantErr, err)
			}
		})
	}
}

func TestIntegration_AIManagerTestProviderConfig_ReportsReadableDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-5.4"},
					{"id": "gpt-5.4-mini"},
				},
			})
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "OK",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	manager := &AIManager{httpClient: server.Client()}
	result, err := manager.TestProviderConfig(context.Background(), ProviderConfig{
		Name:         "Probe Provider",
		ProviderType: "openai-compatible",
		BaseURL:      server.URL,
		APIKey:       "test-key",
		Model:        "gpt-5.4",
		APIFormat:    AIAPIFormatChatCompletions,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("TestProviderConfig returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected probe to succeed, got %+v", result)
	}
	if result.ModelsCount != 2 {
		t.Fatalf("expected 2 models, got %d", result.ModelsCount)
	}
	if result.ResolvedBaseURL != server.URL {
		t.Fatalf("expected resolved base url to stay on the root endpoint, got %s", result.ResolvedBaseURL)
	}
	if !strings.Contains(result.Message, "连接成功") {
		t.Fatalf("expected success message, got %+v", result)
	}
	if !result.ContentReady {
		t.Fatalf("expected content probe to succeed, got %+v", result)
	}
}

func TestIntegration_AIManagerTestProviderConfig_SucceedsWhenModelsEndpointIsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-5.4"},
				},
			})
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
						},
					},
				},
			})
		case "/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []any{},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	manager := &AIManager{httpClient: server.Client()}
	result, err := manager.TestProviderConfig(context.Background(), ProviderConfig{
		Name:         "Probe Provider",
		ProviderType: "openai-compatible",
		BaseURL:      server.URL,
		APIKey:       "test-key",
		Model:        "gpt-5.4",
		APIFormat:    AIAPIFormatChatCompletions,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("expected structured success result, got error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected probe to succeed when models endpoint is reachable, got %+v", result)
	}
	if !strings.Contains(result.Message, "连接成功") {
		t.Fatalf("expected connection success message, got %+v", result)
	}
	if result.ContentReady {
		t.Fatalf("expected content probe to fail when provider omits content, got %+v", result)
	}
	if !strings.Contains(result.ContentMessage, "assistant message has no content field") {
		t.Fatalf("expected content probe message to mention missing content, got %+v", result)
	}
	if !strings.Contains(result.Diagnostic, server.URL) {
		t.Fatalf("expected diagnostic to mention the resolved base url, got %+v", result)
	}
}

func TestIntegration_OpenAICompatibleProvider_FallsBackFromChatToResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
						},
					},
				},
			})
		case "/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]any{
					{
						"content": []map[string]any{
							{"type": "output_text", "text": "{\"summary\":{\"productName\":\"demo\"},\"questions\":[{\"id\":\"q1\",\"question\":\"目标是什么？\",\"rationale\":\"补齐范围\",\"priority\":\"high\"}]}"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Fallback Provider",
			BaseURL:      server.URL,
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatChatCompletions,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	payload, err := provider.GenerateStructured(context.Background(), "clarifications", "{\"project_name\":\"demo\"}")
	if err != nil {
		t.Fatalf("GenerateStructured returned error: %v", err)
	}
	if !strings.Contains(payload, "\"questions\"") {
		t.Fatalf("expected fallback responses payload, got %s", payload)
	}
}

func TestIntegration_OpenAICompatibleProvider_ReportsBothStandardEndpointFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{}})
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         "Failure Provider",
			BaseURL:      server.URL,
			APIKey:       "test-key",
			Model:        "gpt-4.1-mini",
			APIFormat:    AIAPIFormatResponses,
			Enabled:      true,
			ProviderType: "openai-compatible",
		},
		httpClient: server.Client(),
	}

	_, err := provider.GenerateStructured(context.Background(), "clarifications", "{\"project_name\":\"demo\"}")
	if err == nil {
		t.Fatalf("expected GenerateStructured to fail")
	}
	if !strings.Contains(err.Error(), "fallback chat completions also failed") {
		t.Fatalf("expected combined fallback error, got %v", err)
	}
}

func TestBlackBox_AIManagerFallsBackToMockProvider(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	defer func() {
		_ = repo.Close()
	}()

	manager := NewAIManager(repo)
	provider := manager.Provider(context.Background())
	if _, ok := provider.(*MockProvider); !ok {
		t.Fatalf("expected AIManager to fall back to MockProvider when config is absent")
	}
}

func TestBlackBox_ExtractProviderErrorGracefullyHandlesUnknownShape(t *testing.T) {
	if message := extractProviderError(map[string]any{"error": "plain"}); message != "" {
		t.Fatalf("expected empty provider error message for unsupported shape, got %q", message)
	}
	if _, err := extractResponsesText(map[string]any{"output_text": ""}); err == nil {
		t.Fatalf("expected empty output_text to still require nested output parsing")
	}
}
