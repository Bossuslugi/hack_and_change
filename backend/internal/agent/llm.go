package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMToolCall — вызов функции, запрошенный моделью (формат OpenAI).
type LLMToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// LLMMessage — сообщение диалога в формате OpenAI Chat Completions.
type LLMMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []LLMToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// LLMFunction — описание функции для function calling.
type LLMFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// LLMTool — инструмент, доступный модели.
type LLMTool struct {
	Type     string      `json:"type"`
	Function LLMFunction `json:"function"`
}

// LLMRequest — запрос к модели.
type LLMRequest struct {
	Messages []LLMMessage
	Tools    []LLMTool
}

// LLMResponse — ответ модели.
type LLMResponse struct {
	Content   string
	ToolCalls []LLMToolCall
}

// LLM — порт внешней языковой модели.
type LLM interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
	Model() string
}

// OpenAIClient — клиент OpenAI-совместимого API (/chat/completions).
type OpenAIClient struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	http    *http.Client
}

// DefaultLLMBaseURL — базовый URL по умолчанию.
const DefaultLLMBaseURL = "https://api.openai.com/v1"

// NewOpenAIClient создаёт клиента. Возвращает nil, если ключ или модель не заданы
// (тогда агент работает на детерминированном Smart Engine).
func NewOpenAIClient(baseURL, apiKey, model string, timeout time.Duration) *OpenAIClient {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(model) == "" {
		return nil
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultLLMBaseURL
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &OpenAIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), apiKey: strings.TrimSpace(apiKey),
		model: strings.TrimSpace(model), timeout: timeout, http: &http.Client{},
	}
}

// Model возвращает имя модели.
func (c *OpenAIClient) Model() string { return c.model }

// Complete выполняет запрос /chat/completions с таймаутом.
func (c *OpenAIClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body := map[string]any{"model": c.model, "messages": req.Messages, "temperature": 0.2}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("llm: сериализация запроса: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return LLMResponse{}, fmt.Errorf("llm: создание запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("llm: сеть: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return LLMResponse{}, fmt.Errorf("llm: чтение ответа: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return LLMResponse{}, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   *string       `json:"content"`
				ToolCalls []LLMToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return LLMResponse{}, fmt.Errorf("llm: разбор ответа: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return LLMResponse{}, errors.New("llm: пустой ответ (нет choices)")
	}
	msg := parsed.Choices[0].Message
	out := LLMResponse{ToolCalls: msg.ToolCalls}
	if msg.Content != nil {
		out.Content = strings.TrimSpace(*msg.Content)
	}
	return out, nil
}
