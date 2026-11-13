// Package mcp — реестр MCP-инструментов, RBAC-шлюз и имитации корпоративных сервисов Авито
// (Отсутствия, IAM/Доступы, Playbook, Уведомления, Трекер, Feature Flags, Clickstream).
// Реестр строится из specs/02_mcp_tools.openspec.yaml; каждый вызов проходит RBAC и аудит.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/spec"
)

// Call — контекст вызова инструмента.
type Call struct {
	Principal domain.Principal
	InternID  string
	Args      map[string]any
	Now       time.Time
}

// Handler — обработчик инструмента.
type Handler func(ctx context.Context, c Call) (map[string]any, error)

// AttrResolver извлекает атрибуты ресурса для ABAC-ограничений RBAC.
type AttrResolver func(args map[string]any) rbac.Resource

// Error — бизнес-ошибка инструмента с кодом из спецификации.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf создаёт бизнес-ошибку инструмента.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Коды ошибок шлюза.
const (
	CodeUnknownTool = "UNKNOWN_TOOL"
	CodeForbidden   = "FORBIDDEN"
	CodeInvalidArgs = "INVALID_ARGUMENTS"
	CodeInternal    = "INTERNAL"
	CodeDuplicate   = "DUPLICATE"
	CodeNotFound    = "NOT_FOUND"
)

type boundTool struct {
	spec    spec.Tool
	handler Handler
	attrs   AttrResolver
}

// Registry — реестр MCP-инструментов, построенный по спецификации.
type Registry struct {
	servers []spec.Server
	tools   map[string]*boundTool
	order   []string
}

// NewRegistry создаёт реестр из спецификации (обработчики привязываются через Bind).
func NewRegistry(ts spec.ToolsSpec) *Registry {
	r := &Registry{servers: append([]spec.Server{}, ts.Servers...), tools: map[string]*boundTool{}}
	for _, t := range ts.Tools {
		r.tools[t.Name] = &boundTool{spec: t}
		r.order = append(r.order, t.Name)
	}
	return r
}

// Bind привязывает обработчик к инструменту из спецификации.
func (r *Registry) Bind(name string, h Handler, attrs AttrResolver) error {
	t, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("mcp: инструмент %q отсутствует в спецификации", name)
	}
	if h == nil {
		return fmt.Errorf("mcp: пустой обработчик для %q", name)
	}
	if t.handler != nil {
		return fmt.Errorf("mcp: обработчик для %q уже привязан", name)
	}
	t.handler, t.attrs = h, attrs
	return nil
}

// Unbound возвращает инструменты спецификации без обработчика (должно быть пусто).
func (r *Registry) Unbound() []string {
	var out []string
	for _, name := range r.order {
		if r.tools[name].handler == nil {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Tools возвращает контракты инструментов в порядке спецификации.
func (r *Registry) Tools() []spec.Tool {
	out := make([]spec.Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].spec)
	}
	return out
}

// Tool возвращает контракт инструмента.
func (r *Registry) Tool(name string) (spec.Tool, bool) {
	t, ok := r.tools[name]
	if !ok {
		return spec.Tool{}, false
	}
	return t.spec, true
}

// Servers возвращает описания MCP-серверов.
func (r *Registry) Servers() []spec.Server { return append([]spec.Server{}, r.servers...) }

// Telemetry — имитация Clickstream: приёмник событий телеметрии и аудита.
type Telemetry struct {
	store domain.Store
	clock domain.Clock
	ids   domain.IDGenerator
}

// NewTelemetry создаёт приёмник событий.
func NewTelemetry(store domain.Store, clock domain.Clock, ids domain.IDGenerator) *Telemetry {
	return &Telemetry{store: store, clock: clock, ids: ids}
}

// Track записывает событие. Запись best-effort: сбой телеметрии не ломает сценарий.
func (t *Telemetry) Track(ctx context.Context, internID string, actor domain.Principal, kind, name, severity string, payload map[string]any) domain.TelemetryEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	ev := domain.TelemetryEvent{
		ID: t.ids.NewID("evt"), InternID: internID, ActorID: actor.ID, ActorRole: actor.Role,
		Kind: kind, Name: name, Severity: severity, Payload: payload, CreatedAt: t.clock.Now(),
	}
	_ = domain.Save(ctx, t.store, domain.KindEvent, ev.ID, internID, ev.CreatedAt, ev)
	return ev
}

// Gateway — RBAC-шлюз MCP: проверка прав → валидация аргументов → обработчик → аудит.
type Gateway struct {
	reg   *Registry
	rbac  *rbac.Engine
	tel   *Telemetry
	store domain.Store
	clock domain.Clock
	ids   domain.IDGenerator
}

// NewGateway создаёт шлюз.
func NewGateway(reg *Registry, engine *rbac.Engine, tel *Telemetry, store domain.Store, clock domain.Clock, ids domain.IDGenerator) *Gateway {
	return &Gateway{reg: reg, rbac: engine, tel: tel, store: store, clock: clock, ids: ids}
}

// Registry возвращает реестр инструментов.
func (g *Gateway) Registry() *Registry { return g.reg }

func defaultAttrs(args map[string]any) rbac.Resource {
	return rbac.Resource{InternID: argString(args, "intern_id")}
}

// Execute выполняет вызов инструмента от имени субъекта. Всегда возвращает запись аудита;
// итог — в полях Status / RBAC / Error.
func (g *Gateway) Execute(ctx context.Context, p domain.Principal, internID, tool string, args map[string]any) domain.MCPToolCall {
	started := time.Now()
	if args == nil {
		args = map[string]any{}
	}
	call := domain.MCPToolCall{
		ID: g.ids.NewID("call"), InternID: internID, ActorID: p.ID, ActorRole: p.Role,
		Tool: tool, Arguments: args, Result: map[string]any{}, CreatedAt: g.clock.Now(),
	}
	if i := strings.Index(tool, "."); i > 0 {
		call.Server = tool[:i]
	}

	bt, known := g.reg.tools[tool]
	switch {
	case !known || bt.handler == nil:
		// Неизвестный инструмент: default deny, обработчика нет.
		call.RBAC = g.rbac.Check(p, tool, rbac.Resource{})
		call.Status, call.ErrorCode = domain.ToolError, CodeUnknownTool
		call.Error = fmt.Sprintf("Инструмент %q не зарегистрирован", tool)
	default:
		call.Server = bt.spec.Server
		attrs := defaultAttrs
		if bt.attrs != nil {
			attrs = bt.attrs
		}
		call.RBAC = g.rbac.Check(p, tool, attrs(args))
		switch {
		case !call.RBAC.Allowed:
			call.Status, call.ErrorCode, call.Error = domain.ToolForbidden, CodeForbidden, call.RBAC.Reason
		default:
			if problems := ValidateArgs(bt.spec.InputSchema, args); len(problems) > 0 {
				call.Status, call.ErrorCode = domain.ToolInvalid, CodeInvalidArgs
				call.Error = strings.Join(problems, "; ")
				break
			}
			result, err := bt.handler(ctx, Call{Principal: p, InternID: internID, Args: args, Now: call.CreatedAt})
			var toolErr *Error
			switch {
			case err == nil:
				call.Status = domain.ToolSuccess
				if result != nil {
					call.Result = result
				}
			case errors.As(err, &toolErr):
				call.Status, call.ErrorCode, call.Error = domain.ToolError, toolErr.Code, toolErr.Message
				if toolErr.Code == CodeInvalidArgs {
					call.Status = domain.ToolInvalid
				}
			case errors.Is(err, domain.ErrValidation):
				call.Status, call.ErrorCode, call.Error = domain.ToolInvalid, CodeInvalidArgs, err.Error()
			default:
				call.Status, call.ErrorCode, call.Error = domain.ToolError, CodeInternal, err.Error()
			}
		}
	}

	call.DurationMS = float64(time.Since(started).Microseconds()) / 1000
	g.audit(ctx, p, internID, &call)
	return call
}

func (g *Gateway) audit(ctx context.Context, p domain.Principal, internID string, call *domain.MCPToolCall) {
	if err := domain.Save(ctx, g.store, domain.KindToolCall, call.ID, internID, call.CreatedAt, call); err != nil {
		call.Error = strings.TrimSpace(call.Error + " (аудит не записан: " + err.Error() + ")")
	}
	v := call.RBAC
	rbacSeverity := domain.SeverityInfo
	if !v.Allowed {
		rbacSeverity = domain.SeverityWarn
	}
	g.tel.Track(ctx, internID, p, domain.EventKindRBAC, "rbac.decision", rbacSeverity, map[string]any{
		"operation": v.Operation, "role": string(v.Role), "allowed": v.Allowed, "code": v.Code,
		"policy_id": v.PolicyID, "reason": v.Reason, "constraint": v.Constraint, "tool_call_id": call.ID,
	})
	if v.Incident {
		g.tel.Track(ctx, internID, p, domain.EventKindRBAC, "rbac.incident", domain.SeveritySecurity, map[string]any{
			"operation": v.Operation, "role": string(v.Role), "policy_id": v.PolicyID,
			"reason": v.Reason, "actor": p.Name, "tool_call_id": call.ID, "arguments": call.Arguments,
		})
	}
	severity := domain.SeverityInfo
	if call.Status != domain.ToolSuccess {
		severity = domain.SeverityWarn
	}
	g.tel.Track(ctx, internID, p, domain.EventKindMCP, "mcp.call", severity, map[string]any{
		"tool": call.Tool, "server": call.Server, "status": string(call.Status),
		"error_code": call.ErrorCode, "duration_ms": call.DurationMS, "tool_call_id": call.ID,
	})
}
