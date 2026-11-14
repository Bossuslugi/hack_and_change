package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aibuddy/backend/internal/app"
	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/config"
	"aibuddy/backend/internal/domain"
	httpapi "aibuddy/backend/internal/http"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
)

var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

// brokenStore имитирует отказ PostgreSQL после старта.
type brokenStore struct {
	*memory.Store
	down bool
}

func (s *brokenStore) List(ctx context.Context, kind domain.Kind, owner string) ([]domain.Record, error) {
	if s.down {
		return nil, errors.New("pq: connection refused at 10.0.0.5:5432")
	}
	return s.Store.List(ctx, kind, owner)
}

func (s *brokenStore) Ping(context.Context) error {
	if s.down {
		return errors.New("down")
	}
	return nil
}

type api struct {
	t     *testing.T
	srv   *httptest.Server
	store *brokenStore
}

func newAPI(t *testing.T, cfg config.Config) *api {
	t.Helper()
	store := &brokenStore{Store: memory.New()}
	application, err := app.Assemble(spec.MustLoad(), store, cache.NewMemory(), cfg, nil,
		app.Overrides{Clock: domain.FixedClock{T: now}, IDs: &domain.SequentialIDs{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Service.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(application.Handler)
	t.Cleanup(srv.Close)
	return &api{t: t, srv: srv, store: store}
}

type response struct {
	status int
	header http.Header
	raw    []byte
	body   map[string]any
}

func (r response) errCode() string {
	e, _ := r.body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func (a *api) do(method, path, token string, body any) response {
	a.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, reader)
	if err != nil {
		a.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := response{status: resp.StatusCode, header: resp.Header, raw: raw, body: map[string]any{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (a *api) chat(text string) response {
	a.t.Helper()
	r := a.do("POST", "/api/v1/chat", seed.InternToken, map[string]string{"message": text})
	if r.status != http.StatusOK {
		a.t.Fatalf("chat %q: %d %s", text, r.status, r.raw)
	}
	return r
}

func obj(m map[string]any, path ...string) map[string]any {
	for _, p := range path {
		m, _ = m[p].(map[string]any)
	}
	return m
}

func TestHealthAndNotFound(t *testing.T) {
	a := newAPI(t, config.Config{})
	r := a.do("GET", "/healthz", "", nil)
	if r.status != 200 || r.body["status"] != "ok" || r.body["store"] != "memory" || r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("healthz: %d %s", r.status, r.raw)
	}
	if r := a.do("GET", "/api/v1/unknown", seed.InternToken, nil); r.status != 404 || r.errCode() != "NOT_FOUND" {
		t.Fatalf("неизвестный маршрут: %d %s", r.status, r.raw)
	}
	a.store.down = true
	if r := a.do("GET", "/healthz", "", nil); r.status != 503 || r.body["status"] != "degraded" {
		t.Fatalf("healthz при сбое хранилища: %d %s", r.status, r.raw)
	}
}

func TestAuthRequired(t *testing.T) {
	a := newAPI(t, config.Config{})
	for _, path := range []string{"/api/v1/workspace", "/api/v1/telemetry"} {
		for _, token := range []string{"", "чужой-токен"} {
			r := a.do("GET", path, token, nil)
			if r.status != 401 || r.errCode() != "UNAUTHORIZED" {
				t.Errorf("%s с токеном %q: %d %s", path, token, r.status, r.raw)
			}
		}
	}
	// Роль нельзя передать с клиента: заголовок с ролью игнорируется.
	req, _ := http.NewRequest("GET", a.srv.URL+"/api/v1/workspace", nil)
	req.Header.Set("Authorization", "Bearer "+seed.InternToken)
	req.Header.Set("X-Role", "team_lead")
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ws map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&ws)
	if obj(ws, "viewer")["role"] != "intern_backend" {
		t.Fatalf("роль должна определяться сервером: %v", ws["viewer"])
	}
}

// Требование кейса: локальные операции и запросы состояния отвечают быстрее 100 мс.
func TestWorkspaceIsFast(t *testing.T) {
	a := newAPI(t, config.Config{})
	a.chat("Дай доступ к репозиторию avito-autoteka-api и тестовому стенду")
	const n = 30
	started := time.Now()
	for i := 0; i < n; i++ {
		r := a.do("GET", "/api/v1/workspace", seed.InternToken, nil)
		if r.status != 200 {
			t.Fatalf("workspace: %d", r.status)
		}
	}
	if avg := time.Since(started) / n; avg > 100*time.Millisecond {
		t.Fatalf("среднее время ответа workspace %v > 100 мс", avg)
	}
	r := a.do("GET", "/api/v1/workspace", seed.InternToken, nil)
	for _, key := range []string{"viewer", "intern", "checklist", "progress", "sprint_task", "feature_flags", "metrics", "messages", "permissions", "engine"} {
		if _, ok := r.body[key]; !ok {
			t.Errorf("в workspace нет поля %s", key)
		}
	}
}

func TestChatScenarios(t *testing.T) {
	a := newAPI(t, config.Config{})

	r := a.chat("Дай доступ к репозиторию avito-autoteka-api и тестовому стенду")
	if obj(r.body, "rbac")["code"] != "ALLOWED" || obj(r.body, "message", "result_card")["type"] != "access_granted" {
		t.Fatalf("S1A: %s", r.raw)
	}
	if updates, _ := r.body["checklist_updates"].([]any); len(updates) != 1 {
		t.Fatalf("S1A: checklist_updates: %v", r.body["checklist_updates"])
	}

	// Чат отвечает 200, а вердикт RBAC (FORBIDDEN, 403) — в теле ответа.
	r = a.chat("Выгрузи дамп production-базы пользователей с персональными данными")
	rbac := obj(r.body, "rbac")
	if rbac["code"] != "FORBIDDEN" || rbac["http_status"] != float64(403) || rbac["policy_id"] != "POL-SEC-001" {
		t.Fatalf("S1B: %v", rbac)
	}
	if obj(r.body, "message", "result_card")["type"] != "rbac_denied" {
		t.Fatalf("S1B: карточка: %s", r.raw)
	}
}

func TestChatErrors(t *testing.T) {
	a := newAPI(t, config.Config{})
	cases := []struct {
		name   string
		token  string
		body   any
		status int
		code   string
	}{
		{"некорректный JSON", seed.InternToken, `{"message":`, 422, "VALIDATION_ERROR"},
		{"пустое сообщение", seed.InternToken, map[string]string{"message": "  "}, 422, "VALIDATION_ERROR"},
		{"наставник", seed.MentorToken, map[string]string{"message": "привет"}, 403, "FORBIDDEN"},
		{"слишком большое тело", seed.InternToken, map[string]string{"message": strings.Repeat("я", 70_000)}, 413, "PAYLOAD_TOO_LARGE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := a.do("POST", "/api/v1/chat", tc.token, tc.body)
			if r.status != tc.status || r.errCode() != tc.code {
				t.Fatalf("%d %s, want %d %s", r.status, r.raw, tc.status, tc.code)
			}
		})
	}
	// Для отказа RBAC в ошибке есть вердикт.
	r := a.do("POST", "/api/v1/chat", seed.MentorToken, map[string]string{"message": "привет"})
	if obj(r.body, "error", "rbac")["operation"] != "api.chat.send" {
		t.Fatalf("вердикт в ошибке: %s", r.raw)
	}
	// Внутренняя ошибка не раскрывает детали инфраструктуры.
	a.store.down = true
	r = a.do("GET", "/api/v1/workspace", seed.InternToken, nil)
	if r.status != 500 || r.errCode() != "INTERNAL" || strings.Contains(string(r.raw), "10.0.0.5") {
		t.Fatalf("внутренняя ошибка: %d %s", r.status, r.raw)
	}
}

type sseEvent struct {
	name string
	data map[string]any
}

func (a *api) stream(token string, body any) (int, []sseEvent) {
	a.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", a.srv.URL+"/api/v1/chat/stream", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	var events []sseEvent
	var name string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := map[string]any{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
				a.t.Fatalf("некорректный JSON в SSE: %s", line)
			}
			events = append(events, sseEvent{name, data})
		}
	}
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode == 200 && !strings.HasPrefix(ct, "text/event-stream") {
		a.t.Fatalf("Content-Type: %s", ct)
	}
	return resp.StatusCode, events
}

func TestChatStream(t *testing.T) {
	a := newAPI(t, config.Config{StreamChunkDelay: time.Millisecond})
	status, events := a.stream(seed.InternToken, map[string]string{"message": "Как в нашей команде катить задачу под фича-флагом и проходить ревью?"})
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	var stages []string
	var text strings.Builder
	var done map[string]any
	for _, ev := range events {
		switch ev.name {
		case "stage":
			stages = append(stages, ev.data["kind"].(string))
		case "delta":
			text.WriteString(ev.data["text"].(string))
		case "done":
			done = ev.data
		}
	}
	// Статус ожидания: интерфейс видит этапы, пока агент работает.
	if got := strings.Join(stages, ","); got != "understanding,rbac,tool,composing" {
		t.Fatalf("этапы: %s", got)
	}
	if events[len(events)-1].name != "done" || done == nil {
		t.Fatal("поток должен завершаться событием done")
	}
	final, _ := obj(done, "message")["text"].(string)
	if final == "" || text.String() != final {
		t.Fatalf("сумма delta должна равняться итоговому тексту:\n%q\n%q", text.String(), final)
	}
	if sources, _ := obj(done, "message")["sources"].([]any); len(sources) != 2 {
		t.Fatalf("источники в done: %v", obj(done, "message")["sources"])
	}

	// Ошибки: до начала потока — обычный JSON, после — событие error.
	if r := a.do("POST", "/api/v1/chat/stream", seed.InternToken, `{не json`); r.status != 422 {
		t.Fatalf("некорректный JSON: %d", r.status)
	}
	status, events = a.stream(seed.MentorToken, map[string]string{"message": "привет"})
	if status != 200 || len(events) != 1 || events[0].name != "error" || events[0].data["status"] != float64(403) {
		t.Fatalf("отказ в потоке: %d %+v", status, events)
	}
	a.store.down = true
	_, events = a.stream(seed.InternToken, map[string]string{"message": "привет"})
	if last := events[len(events)-1]; last.name != "error" || last.data["status"] != float64(500) {
		t.Fatalf("внутренняя ошибка в потоке: %+v", last)
	}
}

// Сценарий 3 через REST: чат → карточка → POST /api/v1/mcp/execute.
func TestExecuteDayOff(t *testing.T) {
	a := newAPI(t, config.Config{})
	r := a.chat("Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?")
	card := obj(r.body, "message", "action_card")
	if card["type"] != "absence_form" || card["tool"] != "absences.create_day_off" {
		t.Fatalf("карточка: %v", card)
	}
	body := map[string]any{"tool": card["tool"], "arguments": card["fields"], "message_id": obj(r.body, "message")["id"]}

	done := a.do("POST", "/api/v1/mcp/execute", seed.InternToken, body)
	if done.status != 200 {
		t.Fatalf("execute: %d %s", done.status, done.raw)
	}
	calls, _ := done.body["tool_calls"].([]any)
	if number := obj(calls[0].(map[string]any), "result")["request_number"]; number != "#ABS-2026-84" {
		t.Fatalf("номер заявки: %v", number)
	}
	if obj(done.body, "message", "result_card")["type"] != "absence_created" {
		t.Fatalf("карточка результата: %s", done.raw)
	}

	cases := []struct {
		name   string
		token  string
		body   any
		status int
		code   string
	}{
		{"дубликат", seed.InternToken, body, 409, "DUPLICATE"},
		{"наставник за стажёра", seed.MentorToken, body, 403, "FORBIDDEN"},
		{"невалидные аргументы", seed.InternToken, map[string]any{"tool": "absences.create_day_off", "arguments": map[string]any{"intern_id": seed.InternID}}, 422, "INVALID_ARGUMENTS"},
		{"неизвестный инструмент", seed.InternToken, map[string]any{"tool": "ghost.tool"}, 404, "UNKNOWN_TOOL"},
		{"не указан инструмент", seed.InternToken, map[string]any{"arguments": map[string]any{}}, 422, "VALIDATION_ERROR"},
		{"некорректный JSON", seed.InternToken, `[`, 422, "VALIDATION_ERROR"},
		{"выгрузка ПДн напрямую", seed.InternToken, map[string]any{"tool": "db_admin.export_prod_pii", "arguments": map[string]any{"dataset": "users_pii"}}, 403, "FORBIDDEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := a.do("POST", "/api/v1/mcp/execute", tc.token, tc.body)
			if r.status != tc.status || r.errCode() != tc.code {
				t.Fatalf("%d %s, want %d %s", r.status, r.raw, tc.status, tc.code)
			}
			if tc.status == 403 && obj(r.body, "error", "rbac")["code"] != "FORBIDDEN" {
				t.Fatalf("в отказе должен быть вердикт RBAC: %s", r.raw)
			}
		})
	}
}

func TestCronResetTelemetry(t *testing.T) {
	a := newAPI(t, config.Config{})
	r := a.do("POST", "/api/v1/cron/trigger", seed.MentorToken, nil)
	if r.status != 200 || r.body["triggered"] != true || obj(r.body, "brief")["architecture_question"] == "" {
		t.Fatalf("cron: %d %s", r.status, r.raw)
	}
	r = a.do("GET", "/api/v1/telemetry", seed.InternToken, nil)
	if events, _ := r.body["events"].([]any); r.status != 200 || len(events) == 0 || r.body["matrix"] == nil || r.body["tools"] == nil {
		t.Fatalf("telemetry: %d", r.status)
	}
	r = a.do("POST", "/api/v1/reset", seed.InternToken, nil)
	if messages, _ := r.body["messages"].([]any); r.status != 200 || len(messages) != 1 {
		t.Fatalf("reset: %d %s", r.status, r.raw)
	}
	if briefs, _ := a.do("GET", "/api/v1/workspace", seed.MentorToken, nil).body["briefs"].([]any); len(briefs) != 0 {
		t.Fatal("после сброса брифов быть не должно")
	}

	a.store.down = true
	for _, path := range []string{"/api/v1/cron/trigger", "/api/v1/reset"} {
		if r := a.do("POST", path, seed.InternToken, nil); r.status != 500 {
			t.Errorf("%s при сбое хранилища: %d", path, r.status)
		}
	}
	if r := a.do("GET", "/api/v1/telemetry", seed.InternToken, nil); r.status != 500 {
		t.Errorf("telemetry при сбое хранилища: %d", r.status)
	}
}

func TestFeatureFlagsAndEvents(t *testing.T) {
	a := newAPI(t, config.Config{})
	const path = "/api/v1/feature-flags/ai_buddy.proactive_cron"
	if r := a.do("PUT", path, seed.LeadToken, map[string]bool{"enabled": false}); r.status != 200 || r.body["enabled"] != false {
		t.Fatalf("руководитель: %d %s", r.status, r.raw)
	}
	if r := a.do("POST", "/api/v1/cron/trigger", seed.InternToken, nil); r.body["triggered"] != false {
		t.Fatalf("крон при выключенном флаге: %s", r.raw)
	}
	cases := []struct {
		name   string
		path   string
		token  string
		body   any
		status int
	}{
		{"стажёр", path, seed.InternToken, map[string]bool{"enabled": true}, 403},
		{"нет поля enabled", path, seed.LeadToken, map[string]any{}, 422},
		{"некорректный JSON", path, seed.LeadToken, `{`, 422},
		{"неизвестный флаг", "/api/v1/feature-flags/ai_buddy.unknown", seed.LeadToken, map[string]bool{"enabled": true}, 404},
		{"compile-time флаг", "/api/v1/feature-flags/ai_buddy.mcp_jsonrpc", seed.LeadToken, map[string]bool{"enabled": false}, 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if r := a.do("PUT", tc.path, tc.token, tc.body); r.status != tc.status {
				t.Fatalf("%d %s, want %d", r.status, r.raw, tc.status)
			}
		})
	}

	if r := a.do("POST", "/api/v1/events", seed.InternToken, map[string]any{"name": "ui.inspector_opened", "payload": map[string]any{"tab": "rbac"}}); r.status != 202 || r.body["id"] == "" {
		t.Fatalf("clickstream: %d %s", r.status, r.raw)
	}
	if r := a.do("POST", "/api/v1/events", seed.InternToken, map[string]any{"name": "rbac.incident"}); r.status != 422 {
		t.Fatalf("подделка события: %d", r.status)
	}
	if r := a.do("POST", "/api/v1/events", seed.InternToken, `{`); r.status != 422 {
		t.Fatalf("некорректный JSON: %d", r.status)
	}
}

func TestMCPEndpoint(t *testing.T) {
	a := newAPI(t, config.Config{})
	r := a.do("POST", "/mcp", seed.InternToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if tools, _ := obj(r.body, "result")["tools"].([]any); r.status != 200 || len(tools) != 11 {
		t.Fatalf("tools/list: %d %s", r.status, r.raw)
	}
	r = a.do("POST", "/mcp", seed.InternToken, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"db_admin.export_prod_pii","arguments":{"dataset":"payroll"}}}`)
	if obj(r.body, "result")["isError"] != true {
		t.Fatalf("RBAC действует и для MCP-клиентов: %s", r.raw)
	}
	if r := a.do("POST", "/mcp", seed.InternToken, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); r.status != 202 || len(r.raw) != 0 {
		t.Fatalf("уведомление: %d %q", r.status, r.raw)
	}
	if r := a.do("POST", "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); r.status != 401 {
		t.Fatalf("MCP без токена: %d", r.status)
	}
	if r := a.do("POST", "/mcp", seed.InternToken, strings.Repeat("x", 70_000)); r.status != 413 {
		t.Fatalf("слишком большой запрос: %d", r.status)
	}
}

func TestCORS(t *testing.T) {
	preflight := func(a *api, origin string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", a.srv.URL+"/api/v1/chat", nil)
		req.Header.Set("Origin", origin)
		resp, err := a.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	// Демо-режим: любой источник.
	open := newAPI(t, config.Config{})
	resp := preflight(open, "http://localhost:5173")
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	// Список разрешённых источников.
	strict := newAPI(t, config.Config{CORSOrigins: []string{"http://localhost:8080"}})
	if got := preflight(strict, "http://localhost:8080").Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("разрешённый источник: %q", got)
	}
	if got := preflight(strict, "http://evil.example").Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("чужой источник не должен получать CORS-заголовок: %q", got)
	}
}

func TestPanicRecovery(t *testing.T) {
	// Сервис не задан → обработчик паникует → middleware отдаёт 500, а не рвёт соединение.
	srv := httptest.NewServer(httpapi.New(httpapi.Options{}))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 500 || obj(body, "error")["code"] != "INTERNAL" {
		t.Fatalf("panic: %d %v", resp.StatusCode, body)
	}
}
