// Package httpapi — REST API AI-Buddy для фронтенда и MCP JSON-RPC endpoint.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/usecase"
)

// Options — параметры HTTP-слоя.
type Options struct {
	Service *usecase.Service
	Logger  *slog.Logger
	// CORSOrigins — разрешённые источники; "*" — любой (демо-режим).
	CORSOrigins []string
	// StreamChunkDelay — пауза между фрагментами текста в SSE (эффект «печатает»).
	StreamChunkDelay time.Duration
}

// Server — HTTP-обработчик API.
type Server struct {
	svc        *usecase.Service
	log        *slog.Logger
	origins    []string
	chunkDelay time.Duration
	mux        *http.ServeMux
}

const maxBodyBytes = 64 << 10

type ctxKey struct{}

// New создаёт HTTP-обработчик со всеми маршрутами.
func New(o Options) http.Handler {
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if len(o.CORSOrigins) == 0 {
		o.CORSOrigins = []string{"*"}
	}
	s := &Server{svc: o.Service, log: o.Logger, origins: o.CORSOrigins, chunkDelay: o.StreamChunkDelay, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.Handle("GET /api/v1/workspace", s.auth(s.workspace))
	s.mux.Handle("POST /api/v1/chat", s.auth(s.chat))
	s.mux.Handle("POST /api/v1/chat/stream", s.auth(s.chatStream))
	s.mux.Handle("POST /api/v1/mcp/execute", s.auth(s.execute))
	s.mux.Handle("POST /api/v1/absences/review", s.auth(s.reviewAbsence))
	s.mux.Handle("POST /api/v1/cron/trigger", s.auth(s.cron))
	s.mux.Handle("POST /api/v1/reset", s.auth(s.reset))
	s.mux.Handle("GET /api/v1/telemetry", s.auth(s.telemetry))
	s.mux.Handle("PUT /api/v1/feature-flags/{key}", s.auth(s.setFlag))
	s.mux.Handle("POST /api/v1/events", s.auth(s.trackEvent))
	s.mux.Handle("POST /mcp", s.auth(s.mcpRPC))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Маршрут не найден", nil)
	})
	return s.middleware(s.mux)
}

// statusRecorder запоминает код ответа для журнала.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush пробрасывает сброс буфера (нужно для SSE).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) allowOrigin(origin string) string {
	for _, o := range s.origins {
		if o == "*" {
			return "*"
		}
		if origin != "" && strings.EqualFold(o, origin) {
			return origin
		}
	}
	return ""
}

// middleware: восстановление после паники, CORS, лимит тела, журнал и время ответа.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panic", "path", r.URL.Path, "panic", fmt.Sprint(p))
				writeError(rec, http.StatusInternalServerError, "INTERNAL", "Внутренняя ошибка сервера", nil)
			}
			s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
				"ms", float64(time.Since(started).Microseconds())/1000)
		}()

		if allowed := s.allowOrigin(r.Header.Get("Origin")); allowed != "" {
			rec.Header().Set("Access-Control-Allow-Origin", allowed)
			rec.Header().Set("Vary", "Origin")
			rec.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			rec.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			rec.Header().Set("Access-Control-Max-Age", "600")
		}
		rec.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodOptions {
			rec.WriteHeader(http.StatusNoContent)
			return
		}
		r.Body = http.MaxBytesReader(rec, r.Body, maxBodyBytes)
		next.ServeHTTP(rec, r)
	})
}

// auth — имитация корпоративной авторизации: демо-токен → сотрудник → роль.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, domain.Principal)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		p, err := s.svc.Authenticate(token)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)), p)
	})
}

type errorBody struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	RBAC    *domain.RBACVerdict `json:"rbac,omitempty"`
}

type errorEnvelope struct {
	Error  errorBody `json:"error"`
	Result any       `json:"result,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, verdict *domain.RBACVerdict) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message, RBAC: verdict}})
}

// classify сопоставляет ошибку прикладного слоя HTTP-статусу и коду.
func classify(err error) (int, errorBody) {
	var forbidden *usecase.ForbiddenError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, usecase.ErrUnauthorized):
		return http.StatusUnauthorized, errorBody{Code: "UNAUTHORIZED", Message: err.Error()}
	case errors.As(err, &forbidden):
		v := forbidden.Verdict
		return http.StatusForbidden, errorBody{Code: "FORBIDDEN", Message: v.Reason, RBAC: &v}
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, errorBody{Code: "VALIDATION_ERROR", Message: err.Error()}
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, errorBody{Code: "NOT_FOUND", Message: err.Error()}
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, errorBody{Code: "PAYLOAD_TOO_LARGE", Message: "Слишком большой запрос"}
	default:
		return http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: "Внутренняя ошибка сервера. Попробуйте ещё раз."}
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, body := classify(err)
	if status >= 500 {
		s.log.Error("request failed", "path", r.URL.Path, "error", err.Error())
	}
	writeJSON(w, status, errorEnvelope{Error: body})
}

// decode читает JSON-тело запроса в структуру.
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return err
		}
		return fmt.Errorf("%w: некорректный JSON в теле запроса", domain.ErrValidation)
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	info, err := s.svc.Health(r.Context())
	status, state := http.StatusOK, "ok"
	if err != nil {
		status, state = http.StatusServiceUnavailable, "degraded"
	}
	writeJSON(w, status, map[string]any{"status": state, "store": info.Store, "cache": info.Cache})
}

func getInternID(r *http.Request, explicit ...string) string {
	for _, id := range explicit {
		if s := strings.TrimSpace(id); s != "" {
			return s
		}
	}
	if id := strings.TrimSpace(r.URL.Query().Get("intern_id")); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.Header.Get("X-Intern-ID")); id != "" {
		return id
	}
	return ""
}

func (s *Server) workspace(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	internID := getInternID(r)
	view, err := s.svc.Workspace(r.Context(), p, internID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type chatRequest struct {
	Message  string `json:"message"`
	InternID string `json:"intern_id,omitempty"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	internID := getInternID(r, req.InternID)
	res, err := s.svc.Chat(r.Context(), p, req.Message, nil, internID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func sseEvent(w http.ResponseWriter, f http.Flusher, event string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
	f.Flush()
}

// chunks делит текст на фрагменты по границам слов для потоковой отдачи.
func chunks(text string, size int) []string {
	var out []string
	var b strings.Builder
	n := 0
	for _, part := range strings.SplitAfter(text, " ") {
		b.WriteString(part)
		n += len([]rune(part))
		if n >= size {
			out = append(out, b.String())
			b.Reset()
			n = 0
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// chatStream — потоковый вариант чата (SSE): этапы обработки (stage), фрагменты ответа (delta)
// и итог (done). Интерфейс не блокируется и показывает статус ожидания.
func (s *Server) chatStream(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req chatRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, errors.New("потоковая передача не поддерживается"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	internID := getInternID(r, req.InternID)
	res, err := s.svc.Chat(r.Context(), p, req.Message, func(st agent.Stage) { sseEvent(w, flusher, "stage", st) }, internID)
	if err != nil {
		status, body := classify(err)
		if status >= 500 {
			s.log.Error("chat stream failed", "error", err.Error())
		}
		sseEvent(w, flusher, "error", map[string]any{"status": status, "error": body})
		return
	}
	for _, part := range chunks(res.Message.Text, 24) {
		if r.Context().Err() != nil {
			return
		}
		sseEvent(w, flusher, "delta", map[string]string{"text": part})
		if s.chunkDelay > 0 {
			time.Sleep(s.chunkDelay)
		}
	}
	sseEvent(w, flusher, "done", res)
}

type executeRequest struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	MessageID string         `json:"message_id"`
	InternID  string         `json:"intern_id,omitempty"`
}

// execute — прямое подтверждение интерактивной карточки: вызов MCP-инструмента через RBAC-шлюз.
func (s *Server) execute(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req executeRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	internID := getInternID(r, req.InternID)
	res, err := s.svc.Execute(r.Context(), p, req.Tool, req.Arguments, req.MessageID, internID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	call := res.ToolCalls[0]
	if call.OK() {
		writeJSON(w, http.StatusOK, res)
		return
	}
	status, body := http.StatusBadGateway, errorBody{Code: call.ErrorCode, Message: call.Error}
	switch {
	case call.Status == domain.ToolForbidden:
		status, body.RBAC = http.StatusForbidden, res.RBAC
	case call.Status == domain.ToolInvalid:
		status = http.StatusUnprocessableEntity
	case call.ErrorCode == mcp.CodeDuplicate:
		status = http.StatusConflict
	case call.ErrorCode == mcp.CodeUnknownTool:
		status = http.StatusNotFound
	}
	writeJSON(w, status, errorEnvelope{Error: body, Result: res})
}

type reviewAbsenceRequest struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

func (s *Server) reviewAbsence(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req reviewAbsenceRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	updated, err := s.svc.ReviewAbsence(r.Context(), p, req.ID, req.Decision, req.Comment)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) cron(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	res, err := s.svc.CronTrigger(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	view, err := s.svc.Reset(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) telemetry(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	view, err := s.svc.Telemetry(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) setFlag(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Enabled == nil {
		s.fail(w, r, fmt.Errorf("%w: поле enabled обязательно", domain.ErrValidation))
		return
	}
	flag, err := s.svc.SetFlag(r.Context(), p, r.PathValue("key"), *req.Enabled)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, flag)
}

func (s *Server) trackEvent(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	var req struct {
		Name    string         `json:"name"`
		Payload map[string]any `json:"payload"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ev, err := s.svc.TrackUI(r.Context(), p, req.Name, req.Payload)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": ev.ID})
}

// mcpRPC — endpoint протокола MCP (JSON-RPC 2.0): initialize, tools/list, tools/call.
func (s *Server) mcpRPC(w http.ResponseWriter, r *http.Request, p domain.Principal) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := s.svc.HandleMCP(r.Context(), p, raw)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
