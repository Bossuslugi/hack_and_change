package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
)

func TestRegistryConformsToSpec(t *testing.T) {
	f := newFixture(t)
	if missing := f.reg.Unbound(); len(missing) != 0 {
		t.Fatalf("инструменты спецификации без обработчика: %v", missing)
	}
	tools := f.reg.Tools()
	if len(tools) != len(f.bundle.Tools.Tools) || tools[0].Name != ToolRequestRepoAccess {
		t.Fatalf("реестр должен повторять спецификацию: %d инструментов", len(tools))
	}
	for _, name := range []string{
		ToolRequestRepoAccess, ToolExportProdPII, ToolSearchDocs, ToolCreateDayOff, ToolReviewDayOff, ToolSendBrief,
		ToolSendNotification, ToolTaskActivity, ToolListFlags,
	} {
		if _, ok := f.reg.Tool(name); !ok {
			t.Errorf("константа %s не соответствует спецификации", name)
		}
	}
	if _, ok := f.reg.Tool("ghost.tool"); ok {
		t.Error("несуществующий инструмент")
	}
	if len(f.reg.Servers()) != len(f.bundle.Tools.Servers) {
		t.Error("серверы реестра не совпадают со спецификацией")
	}
	if f.gw.Registry() != f.reg {
		t.Error("Gateway.Registry")
	}
}

func TestRegistryBindErrors(t *testing.T) {
	reg := NewRegistry(spec.MustLoad().Tools)
	noop := func(context.Context, Call) (map[string]any, error) { return nil, nil }
	if err := reg.Bind("ghost.tool", noop, nil); err == nil {
		t.Error("привязка к инструменту вне спецификации должна быть ошибкой")
	}
	if err := reg.Bind(ToolListFlags, nil, nil); err == nil {
		t.Error("пустой обработчик")
	}
	if err := reg.Bind(ToolListFlags, noop, nil); err != nil {
		t.Fatal(err)
	}
	if err := reg.Bind(ToolListFlags, noop, nil); err == nil {
		t.Error("повторная привязка")
	}
	if got := reg.Unbound(); len(got) != len(reg.Tools())-1 {
		t.Errorf("Unbound = %v", got)
	}
}

func TestBindAllDetectsSpecDrift(t *testing.T) {
	f := newFixture(t)
	// Повторная привязка к тому же реестру — ошибка.
	if err := f.services.BindAll(f.reg); err == nil {
		t.Error("повторный BindAll должен вернуть ошибку")
	}
	// В спецификации появился инструмент без обработчика — сборка должна упасть.
	ts := f.bundle.Tools
	ts.Tools = append(append([]spec.Tool{}, ts.Tools...), spec.Tool{Name: "tracker.close_task", Server: "tracker"})
	if err := f.services.BindAll(NewRegistry(ts)); err == nil || !strings.Contains(err.Error(), "tracker.close_task") {
		t.Errorf("err = %v", err)
	}
}

func TestValidateArgs(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": []any{"name", "count"},
		"properties": map[string]any{
			"name":   map[string]any{"type": "string", "minLength": 2, "maxLength": 5, "pattern": "^[a-z]+$"},
			"kind":   map[string]any{"type": "string", "enum": []any{"a", "b"}},
			"count":  map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
			"ratio":  map[string]any{"type": "number"},
			"flag":   map[string]any{"type": "boolean"},
			"tags":   map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []any{"x", "y"}}},
			"nested": map[string]any{"type": "object", "required": []any{"id"}, "properties": map[string]any{"id": map[string]any{"type": "string"}}},
			"broken": map[string]any{"type": "string", "pattern": "("},
			"free":   map[string]any{},
		},
	}
	valid := func() map[string]any { return map[string]any{"name": "abc", "count": 3} }
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string // подстрока ожидаемой ошибки; пусто — ошибок нет
	}{
		{"корректные аргументы", func(map[string]any) {}, ""},
		{"все необязательные поля", func(a map[string]any) {
			a["kind"], a["ratio"], a["flag"], a["tags"] = "a", 0.5, true, []any{"x", "y"}
			a["nested"], a["free"] = map[string]any{"id": "1"}, 42
		}, ""},
		{"числа из JSON и Go", func(a map[string]any) { a["count"], a["ratio"] = json.Number("2"), int64(7) }, ""},
		{"массив строк Go", func(a map[string]any) { a["tags"] = []string{"x"} }, ""},
		{"null у необязательного поля", func(a map[string]any) { a["kind"] = nil }, ""},
		{"нет обязательного поля", func(a map[string]any) { delete(a, "name") }, "name: обязательное поле"},
		{"null у обязательного поля", func(a map[string]any) { a["count"] = nil }, "count: обязательное поле"},
		{"неизвестное поле", func(a map[string]any) { a["extra"] = 1 }, "extra: неизвестное поле"},
		{"не строка", func(a map[string]any) { a["name"] = 5 }, "name: ожидается строка"},
		{"слишком короткая строка", func(a map[string]any) { a["name"] = "a" }, "минимум 2"},
		{"слишком длинная строка", func(a map[string]any) { a["name"] = "abcdef" }, "максимум 5"},
		{"не соответствует pattern", func(a map[string]any) { a["name"] = "AB1" }, "не соответствует формату"},
		{"вне enum", func(a map[string]any) { a["kind"] = "z" }, "не входит в допустимый список"},
		{"не число", func(a map[string]any) { a["count"] = "3" }, "count: ожидается число"},
		{"дробное вместо целого", func(a map[string]any) { a["count"] = 2.5 }, "ожидается целое"},
		{"меньше минимума", func(a map[string]any) { a["count"] = 0 }, "минимум 1"},
		{"больше максимума", func(a map[string]any) { a["count"] = float32(9) }, "максимум 5"},
		{"не boolean", func(a map[string]any) { a["flag"] = "да" }, "flag: ожидается true/false"},
		{"не массив", func(a map[string]any) { a["tags"] = "x" }, "tags: ожидается массив"},
		{"элемент массива вне enum", func(a map[string]any) { a["tags"] = []any{"x", "q"} }, "tags[1]"},
		{"вложенный объект", func(a map[string]any) { a["nested"] = map[string]any{} }, "nested.id: обязательное поле"},
		{"вложенный не объект", func(a map[string]any) { a["nested"] = "x" }, "nested: ожидается объект"},
		{"битый pattern в спецификации", func(a map[string]any) { a["broken"] = "x" }, "некорректный pattern"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := valid()
			tc.mutate(args)
			errs := ValidateArgs(schema, args)
			joined := strings.Join(errs, "; ")
			if tc.want == "" && len(errs) > 0 {
				t.Fatalf("неожиданные ошибки: %s", joined)
			}
			if tc.want != "" && !strings.Contains(joined, tc.want) {
				t.Fatalf("ошибки %q не содержат %q", joined, tc.want)
			}
		})
	}
	if errs := ValidateArgs(schema, nil); len(errs) != 2 {
		t.Fatalf("nil-аргументы: ожидались 2 ошибки обязательных полей, получено %v", errs)
	}
	if errs := ValidateArgs(nil, map[string]any{"x": 1}); len(errs) != 0 {
		t.Fatalf("без схемы валидация пропускается: %v", errs)
	}
}

func TestArgHelpers(t *testing.T) {
	args := map[string]any{"s": "v", "list": []any{"a", 1, "b"}, "strs": []string{"x"}, "n": 3.0, "bad": make(chan int)}
	if argString(args, "s") != "v" || argString(args, "n") != "" || argString(args, "нет") != "" {
		t.Error("argString")
	}
	if got := argStrings(args, "list"); len(got) != 2 || got[1] != "b" {
		t.Errorf("argStrings([]any) = %v", got)
	}
	if got := argStrings(args, "strs"); len(got) != 1 || argStrings(args, "s") != nil {
		t.Errorf("argStrings([]string) = %v", got)
	}
	if argInt(args, "n", 9) != 3 || argInt(args, "s", 9) != 9 {
		t.Error("argInt")
	}
	if len(toMap(args["bad"])) != 0 || len(toMap([]int{1})) != 0 {
		t.Error("toMap должен возвращать пустую map для несериализуемых и не-объектов")
	}
}

// customGateway собирает шлюз с одним произвольным обработчиком на месте feature_flags.list.
func customGateway(t *testing.T, f *fixture, h Handler) *Gateway {
	t.Helper()
	ts := spec.ToolsSpec{
		Servers: f.bundle.Tools.Servers,
		Tools:   []spec.Tool{{Name: ToolListFlags, Server: "feature_flags", InputSchema: map[string]any{"type": "object"}}},
	}
	reg := NewRegistry(ts)
	if err := reg.Bind(ToolListFlags, h, func(map[string]any) rbac.Resource { return rbac.Resource{} }); err != nil {
		t.Fatal(err)
	}
	clock, ids := domain.FixedClock{T: testNow}, &domain.SequentialIDs{}
	return NewGateway(reg, rbac.New(f.bundle.RBAC), NewTelemetry(f.store, clock, ids), f.store, clock, ids)
}

func TestGatewayMapsHandlerOutcomes(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name       string
		handler    Handler
		wantStatus domain.ToolStatus
		wantCode   string
	}{
		{"успех", func(context.Context, Call) (map[string]any, error) { return map[string]any{"ok": true}, nil }, domain.ToolSuccess, ""},
		{"успех без результата", func(context.Context, Call) (map[string]any, error) { return nil, nil }, domain.ToolSuccess, ""},
		{"бизнес-ошибка", func(context.Context, Call) (map[string]any, error) {
			return nil, Errorf(CodeDuplicate, "уже есть")
		}, domain.ToolError, CodeDuplicate},
		{"невалидные аргументы из обработчика", func(context.Context, Call) (map[string]any, error) {
			return nil, Errorf(CodeInvalidArgs, "плохая дата")
		}, domain.ToolInvalid, CodeInvalidArgs},
		{"доменная валидация", func(context.Context, Call) (map[string]any, error) { return nil, domain.ErrValidation }, domain.ToolInvalid, CodeInvalidArgs},
		{"внутренняя ошибка", func(context.Context, Call) (map[string]any, error) { return nil, errors.New("boom") }, domain.ToolError, CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := customGateway(t, f, tc.handler).Execute(context.Background(), f.intern(t), seed.InternID, ToolListFlags, nil)
			if call.Status != tc.wantStatus || call.ErrorCode != tc.wantCode {
				t.Fatalf("status=%s code=%s (%s), want %s/%s", call.Status, call.ErrorCode, call.Error, tc.wantStatus, tc.wantCode)
			}
			if call.Result == nil || call.Arguments == nil || call.ID == "" || call.Server != "feature_flags" {
				t.Fatalf("запись аудита заполнена не полностью: %+v", call)
			}
			if (tc.wantStatus == domain.ToolSuccess) != call.OK() {
				t.Fatal("OK() не согласован со статусом")
			}
		})
	}
	if e := Errorf("X", "msg %d", 1); e.Error() != "X: msg 1" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestGatewayUnknownTool(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.intern(t), "ghost.do_magic", map[string]any{})
	if call.Status != domain.ToolError || call.ErrorCode != CodeUnknownTool || call.RBAC.Allowed || call.Server != "ghost" {
		t.Fatalf("неизвестный инструмент: %+v", call)
	}
	if call := f.exec(f.intern(t), "nodot", nil); call.Server != "" || call.ErrorCode != CodeUnknownTool {
		t.Fatalf("имя без сервера: %+v", call)
	}
}

func TestGatewayAuditsEveryCall(t *testing.T) {
	f := newFixture(t)
	ok := f.exec(f.intern(t), ToolListFlags, map[string]any{})
	if !ok.OK() {
		t.Fatalf("feature_flags.list: %+v", ok)
	}
	stored, err := domain.Load[domain.MCPToolCall](context.Background(), f.store.Store, domain.KindToolCall, seed.InternID)
	if err != nil || len(stored) != 1 || stored[0].ID != ok.ID || stored[0].ActorRole != domain.RoleInternBackend {
		t.Fatalf("вызов должен попасть в аудит: %+v, %v", stored, err)
	}
	if got := f.events(t, "mcp.call"); len(got) != 1 || got[0].Payload["tool"] != ToolListFlags || got[0].Severity != domain.SeverityInfo {
		t.Fatalf("событие mcp.call: %+v", got)
	}
	if got := f.events(t, "rbac.decision"); len(got) != 1 || got[0].Payload["allowed"] != true {
		t.Fatalf("событие rbac.decision: %+v", got)
	}
	if got := f.events(t, "rbac.incident"); len(got) != 0 {
		t.Fatalf("разрешённый вызов не должен порождать инцидент: %+v", got)
	}

	// Сбой записи аудита не роняет вызов, но отражается в записи.
	f.store.failPut[domain.KindToolCall] = true
	if call := f.exec(f.intern(t), ToolListFlags, nil); !strings.Contains(call.Error, "аудит не записан") {
		t.Fatalf("сбой аудита должен быть виден: %+v", call)
	}
}

// Сценарий 1Б на уровне шлюза: RBAC блокирует вызов ДО обработчика и фиксирует инцидент.
func TestGatewayBlocksPIIExportBeforeHandler(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{seed.InternID, seed.MentorID, seed.TeamLeadID, seed.CronID} {
		call := f.exec(f.principal(t, id), ToolExportProdPII, map[string]any{"dataset": "users_pii"})
		if call.Status != domain.ToolForbidden || call.ErrorCode != CodeForbidden || call.RBAC.HTTPStatus != 403 || call.RBAC.PolicyID != "POL-SEC-001" {
			t.Fatalf("%s: выгрузка ПДн должна быть запрещена: %+v", id, call)
		}
	}
	// Невалидные аргументы не раскрывают схему неавторизованному: сначала RBAC.
	if call := f.exec(f.intern(t), ToolExportProdPII, map[string]any{"dataset": "что угодно"}); call.Status != domain.ToolForbidden {
		t.Fatalf("RBAC должен проверяться раньше валидации: %+v", call)
	}
	if n := f.services.DBAdmin.Reached(); n != 0 {
		t.Fatalf("обработчик db_admin вызван %d раз — RBAC не сработал", n)
	}
	incidents := f.events(t, "rbac.incident")
	if len(incidents) != 5 || incidents[0].Severity != domain.SeveritySecurity || incidents[0].Payload["policy_id"] != "POL-SEC-001" {
		t.Fatalf("каждая попытка должна быть инцидентом безопасности: %d событий", len(incidents))
	}
	if got := f.events(t, "mcp.call"); got[0].Severity != domain.SeverityWarn || got[0].Payload["status"] != "forbidden" {
		t.Fatalf("mcp.call для запрещённого вызова: %+v", got[0])
	}
	// Эшелонированная защита: даже прямой вызов обработчика ничего не выгружает.
	if _, err := f.services.DBAdmin.ExportProdPII(context.Background(), Call{}); err == nil || f.services.DBAdmin.Reached() != 1 {
		t.Fatal("обработчик db_admin обязан отказывать")
	}
}

func TestGatewayValidatesArgsAfterRBAC(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.intern(t), ToolCreateDayOff, map[string]any{"intern_id": seed.InternID, "date": "13.11.2026", "reason": "Зачёт"})
	if call.Status != domain.ToolInvalid || call.ErrorCode != CodeInvalidArgs || !strings.Contains(call.Error, "date") {
		t.Fatalf("невалидная дата: %+v", call)
	}
	if !call.RBAC.Allowed {
		t.Fatal("RBAC для собственной заявки должен быть ALLOWED")
	}
}

func TestTelemetryTrack(t *testing.T) {
	f := newFixture(t)
	tel := NewTelemetry(f.store, domain.FixedClock{T: testNow}, &domain.SequentialIDs{})
	ev := tel.Track(context.Background(), seed.InternID, f.intern(t), domain.EventKindClickstream, "ui.opened", domain.SeverityInfo, nil)
	if ev.ID == "" || ev.Payload == nil || ev.ActorID != seed.InternID || !ev.CreatedAt.Equal(testNow) {
		t.Fatalf("событие: %+v", ev)
	}
	if got := f.events(t, "ui.opened"); len(got) != 1 {
		t.Fatalf("событие не сохранено: %v", got)
	}
	// Сбой хранилища не должен ломать сценарий.
	f.store.failPut[domain.KindEvent] = true
	if ev := tel.Track(context.Background(), seed.InternID, f.intern(t), "k", "n", "info", map[string]any{"a": 1}); ev.Name != "n" {
		t.Fatal("Track должен вернуть событие даже при сбое записи")
	}
}
