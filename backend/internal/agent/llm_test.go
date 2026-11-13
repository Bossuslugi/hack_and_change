package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewOpenAIClient(t *testing.T) {
	if NewOpenAIClient("", "", "model", 0) != nil || NewOpenAIClient("", "key", " ", 0) != nil {
		t.Fatal("без ключа или модели клиент не создаётся (работает Smart Engine)")
	}
	c := NewOpenAIClient("", " key ", " model-x ", 0)
	if c == nil || c.baseURL != DefaultLLMBaseURL || c.apiKey != "key" || c.Model() != "model-x" || c.timeout != 12*time.Second {
		t.Fatalf("значения по умолчанию: %+v", c)
	}
	c = NewOpenAIClient("https://llm.example/v1/", "key", "m", time.Second)
	if c.baseURL != "https://llm.example/v1" || c.timeout != time.Second {
		t.Fatalf("%+v", c)
	}
}

func TestOpenAIClientComplete(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	reply := `{"choices":[{"message":{"content":"  Привет!  "}}]}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = map[string]any{}
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	defer srv.Close()
	c := NewOpenAIClient(srv.URL+"/v1", "secret", "model-x", 2*time.Second)
	ctx := context.Background()

	resp, err := c.Complete(ctx, LLMRequest{Messages: []LLMMessage{{Role: "user", Content: "привет"}}})
	if err != nil || resp.Content != "Привет!" || len(resp.ToolCalls) != 0 {
		t.Fatalf("resp = %+v, %v", resp, err)
	}
	if gotAuth != "Bearer secret" || gotPath != "/v1/chat/completions" || gotBody["model"] != "model-x" {
		t.Fatalf("запрос: auth=%q path=%q body=%v", gotAuth, gotPath, gotBody)
	}
	if _, hasTools := gotBody["tools"]; hasTools {
		t.Fatal("без инструментов поле tools не отправляется")
	}

	// Ответ с вызовом функции и content: null.
	reply = `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"playbook__search_docs","arguments":"{\"query\":\"ревью\"}"}}]}}]}`
	resp, err = c.Complete(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: "ревью"}},
		Tools:    []LLMTool{{Type: "function", Function: LLMFunction{Name: "playbook__search_docs", Parameters: map[string]any{"type": "object"}}}},
	})
	if err != nil || resp.Content != "" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "playbook__search_docs" {
		t.Fatalf("tool call: %+v, %v", resp, err)
	}
	if tools, _ := gotBody["tools"].([]any); len(tools) != 1 {
		t.Fatalf("tools в запросе: %v", gotBody["tools"])
	}

	failures := []struct {
		name   string
		status int
		reply  string
		want   string
	}{
		{"HTTP 401", http.StatusUnauthorized, `{"error":"bad key"}`, "HTTP 401"},
		{"HTTP 500", http.StatusInternalServerError, `oops`, "HTTP 500"},
		{"битый JSON", http.StatusOK, `{не json`, "разбор ответа"},
		{"нет choices", http.StatusOK, `{"choices":[]}`, "пустой ответ"},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			status, reply = tc.status, tc.reply
			if _, err := c.Complete(ctx, LLMRequest{}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOpenAIClientNetworkFailures(t *testing.T) {
	// Таймаут: медленная LLM не блокирует агента дольше LLM_TIMEOUT_MS.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	c := NewOpenAIClient(slow.URL, "k", "m", 50*time.Millisecond)
	started := time.Now()
	if _, err := c.Complete(context.Background(), LLMRequest{}); err == nil || !strings.Contains(err.Error(), "сеть") {
		t.Fatalf("ожидалась сетевая ошибка по таймауту: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("таймаут не сработал")
	}

	// Недоступный хост и некорректный URL.
	if _, err := NewOpenAIClient("http://127.0.0.1:1", "k", "m", time.Second).Complete(context.Background(), LLMRequest{}); err == nil {
		t.Fatal("недоступный хост")
	}
	if _, err := NewOpenAIClient("http://bad host", "k", "m", time.Second).Complete(context.Background(), LLMRequest{}); err == nil {
		t.Fatal("некорректный URL")
	}
	// Несериализуемые параметры инструмента.
	bad := LLMRequest{Tools: []LLMTool{{Function: LLMFunction{Parameters: map[string]any{"x": make(chan int)}}}}}
	if _, err := c.Complete(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "сериализация") {
		t.Fatalf("err = %v", err)
	}
}
