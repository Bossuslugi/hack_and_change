package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
)

// Четверг, 12 ноября 2026: «в пятницу» = 2026-11-13.
var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

// fakeLLM — управляемая модель: отдаёт заранее заданные ответы и запоминает запросы.
type fakeLLM struct {
	responses []agent.LLMResponse
	err       error
	requests  []agent.LLMRequest
}

func (f *fakeLLM) Model() string { return "fake-model" }

func (f *fakeLLM) Complete(_ context.Context, req agent.LLMRequest) (agent.LLMResponse, error) {
	i := len(f.requests)
	f.requests = append(f.requests, req)
	if f.err != nil {
		return agent.LLMResponse{}, f.err
	}
	if i < len(f.responses) {
		return f.responses[i], nil
	}
	return f.responses[len(f.responses)-1], nil
}

func toolCall(id, name, args string) agent.LLMToolCall {
	tc := agent.LLMToolCall{ID: id, Type: "function"}
	tc.Function.Name, tc.Function.Arguments = name, args
	return tc
}

type stack struct {
	t         *testing.T
	engine    *agent.Engine
	store     *memory.Store
	services  *mcp.Services
	bundle    *spec.Bundle
	intern    domain.InternProfile
	principal domain.Principal
	checklist []domain.ChecklistItem
	task      domain.SprintTask
	stages    []agent.Stage
}

func newStack(t *testing.T, llm agent.LLM) *stack {
	t.Helper()
	bundle, data, store, kv := spec.MustLoad(), seed.Data(), memory.New(), cache.NewMemory()
	recs, err := data.Records(now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	clock, ids := domain.FixedClock{T: now}, &domain.SequentialIDs{}
	reg := mcp.NewRegistry(bundle.Tools)
	services := mcp.NewServices(mcp.Deps{Store: store, Cache: kv, Clock: clock, IDs: ids, Seed: data})
	if err := services.BindAll(reg); err != nil {
		t.Fatal(err)
	}
	gw := mcp.NewGateway(reg, rbac.New(bundle.RBAC), mcp.NewTelemetry(store, clock, ids), store, clock, ids)
	e, _ := domain.NewDirectory(data.Employees).ByID(seed.InternID)
	return &stack{
		t: t, store: store, services: services, bundle: bundle,
		engine: agent.New(agent.Options{Spec: &bundle.Agent, Tools: gw, Catalog: reg, Flags: services.Flags, LLM: llm, Clock: clock}),
		intern: data.Intern, principal: e.Principal(), checklist: data.Checklist, task: data.Task,
	}
}

func (s *stack) ask(text, lastIntent string) agent.Reply {
	s.stages = nil
	return s.engine.Handle(context.Background(), agent.Request{
		Principal: s.principal, Intern: s.intern, Text: text, Checklist: s.checklist, Task: s.task,
		LastIntent: lastIntent, Emit: func(st agent.Stage) { s.stages = append(s.stages, st) },
	})
}

func mustContain(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Errorf("текст не содержит %q:\n%s", p, text)
		}
	}
}

// ---------- Сценарий 1А ----------

func TestAccessRequestGranted(t *testing.T) {
	s := newStack(t, nil)
	r := s.ask("Дай доступ к репозиторию avito-autoteka-api и тестовому стенду", "")

	if r.Intent != agent.IntentAccess || r.Scenario != "S1A" || r.Engine != domain.EngineSmart || !r.Answered {
		t.Fatalf("reply = %+v", r)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Tool != mcp.ToolRequestRepoAccess || !r.ToolCalls[0].OK() {
		t.Fatalf("ожидался успешный вызов IAM: %+v", r.ToolCalls)
	}
	if r.Verdict == nil || !r.Verdict.Allowed || r.Verdict.PolicyID != "POL-IAM-001" {
		t.Fatalf("вердикт RBAC: %+v", r.Verdict)
	}
	if r.ResultCard == nil || r.ResultCard.Type != domain.CardAccessGranted || r.ResultCard.Status != domain.CardStatusSuccess {
		t.Fatalf("карточка результата: %+v", r.ResultCard)
	}
	if r.SavingKey != domain.SavingAccess {
		t.Errorf("saving key = %q", r.SavingKey)
	}
	mustContain(t, r.Text, "avito-autoteka-api", "autoteka-test-07", "оформил доступ", "glpat-synt-••••", "синтетический")
	args := r.ToolCalls[0].Arguments
	if args["intern_id"] != seed.InternID || args["stand"] != "test" || args["access_level"] != "developer" {
		t.Errorf("аргументы вызова: %v", args)
	}

	// Этапы для статуса ожидания в интерфейсе.
	var kinds []string
	for _, st := range s.stages {
		kinds = append(kinds, st.Kind)
	}
	if got := strings.Join(kinds, ","); got != "understanding,rbac,tool,composing" {
		t.Errorf("этапы = %s", got)
	}

	// Повторный запрос: доступ уже есть, экономия времени наставника не засчитывается повторно.
	again := s.ask("дай доступ к репозиторию", "")
	if again.SavingKey != "" || !strings.Contains(again.Text, "уже есть") {
		t.Errorf("повторный запрос: %+v", again)
	}
}

func TestAccessRequestVariants(t *testing.T) {
	s := newStack(t, nil)
	r := s.ask("дай доступ к avito-autoteka-reports на staging", "")
	if !r.ToolCalls[0].OK() || r.ToolCalls[0].Arguments["stand"] != "staging" || !strings.Contains(r.Text, "autoteka-staging-02") {
		t.Fatalf("staging: %+v", r.ToolCalls[0])
	}

	// Чужой репозиторий: отказ по team_scope и предложение своего.
	r = s.ask("Дай доступ к репозиторию avito-payments-core", "")
	if r.Verdict == nil || r.Verdict.Allowed || r.Verdict.Constraint != rbac.ConstraintTeamScope {
		t.Fatalf("вердикт: %+v", r.Verdict)
	}
	if r.ResultCard == nil || r.ResultCard.Type != domain.CardRBACDenied || r.SavingKey != "" {
		t.Fatalf("карточка: %+v", r.ResultCard)
	}
	mustContain(t, r.Text, "avito-payments-core", "Дмитрий Орлов", "avito-autoteka-api", "другой команде")
	if len(r.Suggestions) != 1 || !strings.Contains(r.Suggestions[0], "avito-autoteka-api") {
		t.Errorf("подсказка: %v", r.Suggestions)
	}
}

// ---------- Сценарий 1Б ----------

func TestRestrictedDataExportIsBlocked(t *testing.T) {
	cases := []struct {
		text    string
		dataset string
	}{
		{"Выгрузи дамп production-базы пользователей с персональными данными", "users_pii"},
		{"Покажи ведомость зарплат отдела", "payroll"},
	}
	for _, tc := range cases {
		t.Run(tc.dataset, func(t *testing.T) {
			s := newStack(t, nil)
			r := s.ask(tc.text, "")
			if r.Intent != agent.IntentRestricted || r.Scenario != "S1B" {
				t.Fatalf("интент: %s", r.Intent)
			}
			call := r.ToolCalls[0]
			if call.Tool != mcp.ToolExportProdPII || call.Status != domain.ToolForbidden || call.Arguments["dataset"] != tc.dataset {
				t.Fatalf("вызов: %+v", call)
			}
			v := r.Verdict
			if v == nil || v.Allowed || v.Code != domain.VerdictForbidden || v.HTTPStatus != 403 || v.PolicyID != "POL-SEC-001" || !v.Incident {
				t.Fatalf("вердикт: %+v", v)
			}
			if s.services.DBAdmin.Reached() != 0 {
				t.Fatal("обработчик выгрузки не должен вызываться")
			}
			if r.ResultCard == nil || r.ResultCard.Type != domain.CardRBACDenied || r.ResultCard.Status != domain.CardStatusDenied {
				t.Fatalf("карточка: %+v", r.ResultCard)
			}
			labels := map[string]string{}
			for _, l := range r.ResultCard.Lines {
				labels[l.Label] = l.Value
			}
			if labels["Почему"] == "" || labels["Журнал безопасности"] == "" || labels["Что можно вместо этого"] == "" {
				t.Errorf("строки карточки: %v", labels)
			}
			mustContain(t, r.Text, "политикой безопасности", "песочниц", "синтетическими")
			// Служебные детали (инструменты, политики, коды) стажёру не показываются — они в «Инспекторе жюри».
			visible := r.Text + r.ResultCard.Title
			for _, l := range r.ResultCard.Lines {
				visible += l.Label + l.Value
			}
			for _, leak := range []string{"db_admin", "POL-SEC", "FORBIDDEN", "403", "intern_backend", "iam_access"} {
				if strings.Contains(visible, leak) {
					t.Errorf("в чат попала служебная информация %q", leak)
				}
			}
			if r.SavingKey != domain.SavingBlocked || len(r.Suggestions) == 0 {
				t.Errorf("saving=%q suggestions=%v", r.SavingKey, r.Suggestions)
			}
		})
	}
}

// ---------- Сценарий 2 ----------

func TestProcessQuestionUsesInternContext(t *testing.T) {
	s := newStack(t, nil)
	r := s.ask("Как в нашей команде катить задачу под фича-флагом и проходить ревью?", "")
	if r.Intent != agent.IntentProcess || r.Scenario != "S2" || !r.Answered || r.SavingKey != domain.SavingQuestion {
		t.Fatalf("reply: %+v", r)
	}
	if len(r.Sources) != 2 || r.Sources[0].ID != "pb-feature-flags" || r.Sources[1].ID != "pb-code-review" {
		t.Fatalf("источники: %+v", r.Sources)
	}
	for _, src := range r.Sources {
		if !src.IsSynthetic || src.Disclaimer != "Тестовая инструкция (Синтетические данные)" {
			t.Errorf("источник %s без пометки синтетики", src.ID)
		}
	}
	if strings.Join(r.DocsRead, ",") != "pb-feature-flags,pb-code-review" {
		t.Errorf("прочитанные документы: %v", r.DocsRead)
	}
	mustContain(t, r.Text, "команда Autoteka · стек Go · этап Week 1", "Как катить задачу под фича-флагом",
		"Как проходить код-ревью", "Мария Кузнецова", "тестовые инструкции (синтетические данные)")
	args := r.ToolCalls[0].Arguments
	if args["team"] != "Autoteka" || args["stack"] != "Go" || args["stage"] != "Week 1" {
		t.Errorf("контекст стажёра должен передаваться в поиск: %v", args)
	}
}

func TestTaskHelpAndKnowledgeFallback(t *testing.T) {
	s := newStack(t, nil)
	r := s.ask("Я застрял на первой задаче, с чего начать?", "")
	if r.Intent != agent.IntentTaskHelp || len(r.Sources) != 1 || r.Sources[0].ID != "pb-first-task" {
		t.Fatalf("task_help: %+v", r)
	}
	mustContain(t, r.Text, "AUT-1423", "бриф к 1:1")

	// Свободный вопрос, на который есть документ.
	r = s.ask("Как проходят встречи 1:1?", "")
	if r.Intent != agent.IntentKnowledge || len(r.Sources) != 1 || r.Sources[0].ID != "pb-one-on-one" || !r.Answered {
		t.Fatalf("knowledge_search: %+v", r)
	}

	// Ответа нет: честный отказ без галлюцинаций, вопрос уходит в повестку 1:1.
	r = s.ask("Как заказать пропуск для гостя?", "")
	if r.Answered || r.OpenQuestion != "Как заказать пропуск для гостя?" || len(r.Sources) != 0 || r.SavingKey != "" {
		t.Fatalf("not_found: %+v", r)
	}
	mustContain(t, r.Text, "не нашёл", "Мария Кузнецова")
}

// ---------- Сценарий 3 ----------

func TestAbsencePreparesActionCardWithoutWriting(t *testing.T) {
	s := newStack(t, nil)
	r := s.ask("Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?", "")
	if r.Intent != agent.IntentAbsence || r.Scenario != "S3" || !r.Answered {
		t.Fatalf("reply: %+v", r)
	}
	card := r.ActionCard
	if card == nil || card.Type != domain.ActionAbsenceForm || card.Tool != mcp.ToolCreateDayOff || card.Status != domain.ActionStatusPending {
		t.Fatalf("карточка действия: %+v", card)
	}
	f := card.Fields
	if f["intern_id"] != seed.InternID || f["date"] != "2026-11-13" || f["time_from"] != "14:00" || f["time_to"] != "18:00" || f["reason"] != "Зачёт в университете" {
		t.Fatalf("поля карточки: %v", f)
	}
	if notify, _ := f["notify"].([]any); len(notify) != 2 {
		t.Fatalf("notify: %v", f["notify"])
	}
	mustContain(t, r.Text, "писать руководителю не нужно", "Мария Кузнецова", "Дмитрий Орлов", "пт, 13.11, 14:00–18:00", "без твоего подтверждения")

	// До подтверждения ничего не записывается: вызван только поиск по базе знаний.
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Tool != mcp.ToolSearchDocs {
		t.Fatalf("вызовы: %+v", r.ToolCalls)
	}
	absences, _ := domain.Load[domain.AbsenceRequest](context.Background(), s.store, domain.KindAbsence, "")
	if len(absences) != 0 {
		t.Fatal("заявка не должна создаваться до подтверждения")
	}
	if len(r.Sources) != 1 || r.Sources[0].ID != "pb-absences" {
		t.Errorf("источник: %+v", r.Sources)
	}
}

func TestAbsenceEdgeCases(t *testing.T) {
	s := newStack(t, nil)
	// Дата не распознана: карточка с пустой датой.
	r := s.ask("как оформить отгул?", "")
	if r.ActionCard == nil || r.ActionCard.Fields["date"] != "" || !strings.Contains(r.Text, "не понял дату") {
		t.Fatalf("без даты: %+v", r.ActionCard)
	}
	// Карточка выключена feature-флагом: только инструкция.
	if _, err := s.services.Flags.Set(context.Background(), mcp.FlagAbsenceCard, false); err != nil {
		t.Fatal(err)
	}
	r = s.ask("как оформить отгул в пятницу?", "")
	if r.ActionCard != nil || !strings.Contains(r.Text, "выключена feature-флагом") {
		t.Fatalf("флаг выключен: %+v", r)
	}
}

// ---------- Служебные интенты ----------

func TestServiceIntents(t *testing.T) {
	s := newStack(t, nil)

	r := s.ask("Что дальше по чек-листу?", "")
	mustContain(t, r.Text, "2 из 7 шагов (28%)", "Следующий шаг", "Получить доступы")
	if len(r.Suggestions) != 1 || r.Suggestions[0] != agent.ScenarioPrompts["access_repo"] {
		t.Errorf("подсказка следующего шага: %v", r.Suggestions)
	}
	for i := range s.checklist {
		s.checklist[i].Status = domain.StatusCompleted
	}
	if r := s.ask("что дальше?", ""); !strings.Contains(r.Text, "Все шаги чек-листа выполнены") {
		t.Errorf("все шаги выполнены: %s", r.Text)
	}

	r = s.ask("кто мой наставник?", "")
	mustContain(t, r.Text, "Мария Кузнецова", "Дмитрий Орлов")

	r = s.ask("Привет!", "")
	mustContain(t, r.Text, "Привет, Алексей", "Autoteka")
	if len(r.Suggestions) != 3 || len(r.ToolCalls) != 0 || r.Verdict != nil {
		t.Errorf("приветствие: %+v", r)
	}
}

func TestAffirmativeContinuesPreviousScenario(t *testing.T) {
	s := newStack(t, nil)
	// После отказа в выгрузке предложена песочница: «да» = запрос доступа.
	r := s.ask("Да, давай", agent.IntentRestricted)
	if r.Intent != agent.IntentAccess || len(r.ToolCalls) != 1 || !r.ToolCalls[0].OK() || r.ResultCard.Type != domain.CardAccessGranted {
		t.Fatalf("после отказа: %+v", r)
	}
	// После карточки отгула: напоминание о подтверждении в интерфейсе, без записи.
	r = s.ask("да", agent.IntentAbsence)
	if r.Intent != agent.IntentAffirmative || len(r.ToolCalls) != 0 || !strings.Contains(r.Text, "Оформить отгул") {
		t.Fatalf("после отгула: %+v", r)
	}
	// Без контекста: уточняющий вопрос.
	r = s.ask("ок", "")
	if r.Intent != agent.IntentAffirmative || !strings.Contains(r.Text, "Уточни") || len(r.Suggestions) != 3 {
		t.Fatalf("без контекста: %+v", r)
	}
}

// ---------- Сбои инструментов ----------

// stubTools — исполнитель, возвращающий заранее заданный исход.
type stubTools struct {
	status domain.ToolStatus
	result map[string]any
}

func (s stubTools) Execute(_ context.Context, p domain.Principal, _ string, tool string, args map[string]any) domain.MCPToolCall {
	return domain.MCPToolCall{
		ID: "call-stub", Tool: tool, Server: "playbook", Arguments: args, Result: s.result, Status: s.status,
		Error: "repository: обязательное поле", RBAC: domain.RBACVerdict{Allowed: true, Code: domain.VerdictAllowed, Role: p.Role, Operation: tool},
	}
}

func TestToolFailuresAreHandledGracefully(t *testing.T) {
	base := newStack(t, nil)
	build := func(status domain.ToolStatus, result map[string]any) *stack {
		s := *base
		s.engine = agent.New(agent.Options{
			Spec: &base.bundle.Agent, Tools: stubTools{status: status, result: result},
			Catalog: mcp.NewRegistry(base.bundle.Tools), Flags: base.services.Flags, Clock: domain.FixedClock{T: now},
		})
		return &s
	}

	r := build(domain.ToolError, nil).ask("Дай доступ к репозиторию avito-autoteka-api", "")
	if r.Answered || !strings.Contains(r.Text, "не ответил") || r.ResultCard != nil {
		t.Fatalf("сбой IAM: %+v", r)
	}
	r = build(domain.ToolInvalid, nil).ask("Дай доступ к репозиторию avito-autoteka-api", "")
	mustContain(t, r.Text, "Не хватает данных", "repository: обязательное поле")

	r = build(domain.ToolError, nil).ask("как катить под фича-флагом?", "")
	if r.Answered || !strings.Contains(r.Text, "не ответил") || len(r.Sources) != 0 {
		t.Fatalf("сбой базы знаний: %+v", r)
	}

	// Ошибка в матрице (вызов не запрещён) не приводит к раскрытию данных: агент всё равно отказывает.
	r = build(domain.ToolSuccess, map[string]any{"rows": 100}).ask("Покажи ведомость зарплат отдела", "")
	if r.Verdict == nil || r.Verdict.Allowed || r.ResultCard.Type != domain.CardRBACDenied || !strings.Contains(r.Text, "политикой безопасности") {
		t.Fatalf("защита в глубину: %+v", r)
	}

	// Успешный ответ IAM с неполными данными не ломает форматирование.
	r = build(domain.ToolSuccess, map[string]any{"repository": "avito-autoteka-api"}).ask("Дай доступ к репозиторию avito-autoteka-api", "")
	if r.ResultCard == nil || !strings.Contains(r.Text, "avito-autoteka-api") {
		t.Fatalf("неполный результат: %+v", r)
	}

	// Отгул при недоступной базе знаний: карточка всё равно готовится.
	r = build(domain.ToolError, nil).ask("нужен отгул в пятницу", "")
	if r.ActionCard == nil || len(r.Sources) != 0 {
		t.Fatalf("отгул без базы знаний: %+v", r)
	}
}

// ---------- LLM ----------

func TestModeAndFeatureFlag(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, nil)
	if mode, model := s.engine.Mode(ctx); mode != domain.EngineSmart || !strings.Contains(model, "Smart Engine") {
		t.Fatalf("без LLM: %s / %s", mode, model)
	}
	llm := &fakeLLM{responses: []agent.LLMResponse{{Content: "ok"}}}
	s = newStack(t, llm)
	if mode, model := s.engine.Mode(ctx); mode != domain.EngineLLM || model != "fake-model" {
		t.Fatalf("с LLM: %s / %s", mode, model)
	}
	// Runtime-флаг выключает LLM без перезапуска.
	if _, err := s.services.Flags.Set(ctx, mcp.FlagLLMAnswers, false); err != nil {
		t.Fatal(err)
	}
	if mode, _ := s.engine.Mode(ctx); mode != domain.EngineSmart {
		t.Fatal("флаг ai_buddy.llm_answers должен отключать LLM")
	}
	if r := s.ask("как катить под фича-флагом?", ""); r.Engine != domain.EngineSmart || len(llm.requests) != 0 {
		t.Fatalf("при выключенном флаге LLM не вызывается: %+v", r.Engine)
	}
}

func TestLLMRephrasesKnownScenario(t *testing.T) {
	llm := &fakeLLM{responses: []agent.LLMResponse{{Content: "Готово: доступ к avito-autoteka-api у тебя есть, стенд тоже."}}}
	s := newStack(t, llm)
	r := s.ask("Дай доступ к репозиторию avito-autoteka-api и тестовому стенду", "")
	if r.Engine != domain.EngineLLM || r.Text != "Готово: доступ к avito-autoteka-api у тебя есть, стенд тоже." || r.LLMFallback != "" {
		t.Fatalf("переформулировка: %+v", r)
	}
	// План вызова инструмента остаётся детерминированным.
	if len(r.ToolCalls) != 1 || !r.ToolCalls[0].OK() || r.ResultCard.Type != domain.CardAccessGranted {
		t.Fatalf("вызов инструмента: %+v", r.ToolCalls)
	}
	if len(llm.requests) != 1 || len(llm.requests[0].Tools) != 0 {
		t.Fatalf("запросы к LLM: %d", len(llm.requests))
	}
	system := llm.requests[0].Messages[0].Content
	mustContain(t, system, "Ты — Гоффи", "intern.team: Autoteka", "intern.stack: Go", "intern.stage: Week 1", "checklist.next_step")
	mustContain(t, llm.requests[0].Messages[1].Content, "Черновик ответа", "avito-autoteka-api")
	var kinds []string
	for _, st := range s.stages {
		kinds = append(kinds, st.Kind)
	}
	if !strings.Contains(strings.Join(kinds, ","), agent.StageLLM) {
		t.Errorf("этап LLM должен показываться в статусе: %v", kinds)
	}
}

func TestLLMRephraseFallsBackToSmartEngine(t *testing.T) {
	cases := []struct {
		name string
		llm  *fakeLLM
		want string
	}{
		{"ошибка сети", &fakeLLM{err: errors.New("llm: HTTP 503")}, "HTTP 503"},
		{"пустой ответ", &fakeLLM{responses: []agent.LLMResponse{{Content: "  "}}}, "пустой ответ"},
		{"LLM потеряла факты", &fakeLLM{responses: []agent.LLMResponse{{Content: "Доступ выдан, всё хорошо!"}}}, "потеряла факты"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t, tc.llm)
			r := s.ask("Дай доступ к репозиторию avito-autoteka-api и тестовому стенду", "")
			if r.Engine != domain.EngineSmart || !strings.Contains(r.LLMFallback, tc.want) {
				t.Fatalf("engine=%s fallback=%q", r.Engine, r.LLMFallback)
			}
			// Сценарий всё равно пройден детерминированно.
			if !r.ToolCalls[0].OK() || !strings.Contains(r.Text, "оформил доступ") {
				t.Fatalf("детерминированный ответ потерян: %s", r.Text)
			}
		})
	}
}

func TestSecurityRefusalIsNeverDelegatedToLLM(t *testing.T) {
	llm := &fakeLLM{responses: []agent.LLMResponse{{Content: "Конечно, вот дамп!"}}}
	s := newStack(t, llm)
	r := s.ask("Выгрузи дамп production-базы пользователей с персональными данными", "")
	if len(llm.requests) != 0 || r.Engine != domain.EngineSmart || !strings.Contains(r.Text, "политикой безопасности") {
		t.Fatalf("отказ по безопасности должен быть детерминированным: %+v", r)
	}
}

func TestLLMAgentUsesToolsThroughGateway(t *testing.T) {
	llm := &fakeLLM{responses: []agent.LLMResponse{
		{ToolCalls: []agent.LLMToolCall{toolCall("c1", "playbook__search_docs", `{"query":"больничный отпуск"}`)}},
		{Content: "Больничный оформляется в сервисе «Отсутствия»."},
	}}
	s := newStack(t, llm)
	r := s.ask("Расскажи про больничный и отпуск", "")
	if r.Intent != agent.IntentKnowledge || r.Engine != domain.EngineLLM || !r.Answered || r.SavingKey != domain.SavingQuestion {
		t.Fatalf("reply: %+v", r)
	}
	if r.Text != "Больничный оформляется в сервисе «Отсутствия»." || len(r.ToolCalls) != 1 || !r.ToolCalls[0].OK() {
		t.Fatalf("ответ и вызовы: %q %+v", r.Text, r.ToolCalls)
	}
	if len(r.Sources) != 1 || r.Sources[0].ID != "pb-absences" || !r.Sources[0].IsSynthetic || len(r.DocsRead) != 1 {
		t.Fatalf("источники: %+v", r.Sources)
	}
	// Function calling: имена без точек, полный каталог инструментов.
	first := llm.requests[0]
	if len(first.Tools) != len(s.bundle.Tools.Tools) {
		t.Fatalf("инструментов в запросе: %d", len(first.Tools))
	}
	for _, tool := range first.Tools {
		if strings.Contains(tool.Function.Name, ".") || tool.Type != "function" || tool.Function.Parameters == nil {
			t.Errorf("описание функции: %+v", tool)
		}
	}
	// Второй запрос содержит результат инструмента.
	second := llm.requests[1].Messages
	last := second[len(second)-1]
	if last.Role != "tool" || last.ToolCallID != "c1" || !strings.Contains(last.Content, "pb-absences") {
		t.Fatalf("сообщение с результатом инструмента: %+v", last)
	}
}

func TestLLMAgentGuardrails(t *testing.T) {
	t.Run("действие с подтверждением превращается в карточку", func(t *testing.T) {
		llm := &fakeLLM{responses: []agent.LLMResponse{
			{ToolCalls: []agent.LLMToolCall{toolCall("c1", "absences__create_day_off", `{"intern_id":"u-other","date":"2026-11-20","reason":"Переезд"}`)}},
			{Content: "Подготовил заявку — подтверди её в карточке."},
		}}
		s := newStack(t, llm)
		r := s.ask("Запланируй мне выходной на двадцатое из-за переезда", "")
		if r.ActionCard == nil || r.ActionCard.Tool != mcp.ToolCreateDayOff || r.ActionCard.Fields["date"] != "2026-11-20" {
			t.Fatalf("карточка: %+v", r.ActionCard)
		}
		if r.ActionCard.Fields["intern_id"] != seed.InternID {
			t.Fatal("LLM не может действовать от имени другого сотрудника")
		}
		if len(r.ToolCalls) != 0 {
			t.Fatal("инструмент с requires_confirmation не должен исполняться без подтверждения")
		}
		if absences, _ := domain.Load[domain.AbsenceRequest](context.Background(), s.store, domain.KindAbsence, ""); len(absences) != 0 {
			t.Fatal("заявка создана без подтверждения")
		}
		tool := llm.requests[1].Messages[len(llm.requests[1].Messages)-1]
		if !strings.Contains(tool.Content, "awaiting_user_confirmation") {
			t.Fatalf("LLM должна узнать, что действие ждёт подтверждения: %s", tool.Content)
		}
	})

	t.Run("RBAC действует и для вызовов LLM", func(t *testing.T) {
		llm := &fakeLLM{responses: []agent.LLMResponse{
			{ToolCalls: []agent.LLMToolCall{toolCall("c1", "db_admin__export_prod_pii", `{"dataset":"users_pii"}`)}},
			{Content: "Эти данные недоступны."},
		}}
		s := newStack(t, llm)
		r := s.ask("Собери мне список клиентов для анализа", "")
		if len(r.ToolCalls) != 1 || r.ToolCalls[0].Status != domain.ToolForbidden || s.services.DBAdmin.Reached() != 0 {
			t.Fatalf("вызов должен быть заблокирован: %+v", r.ToolCalls)
		}
		if r.Verdict == nil || r.Verdict.Allowed || r.ResultCard == nil || r.ResultCard.Type != domain.CardRBACDenied || r.SavingKey != domain.SavingBlocked {
			t.Fatalf("вердикт и карточка: %+v / %+v", r.Verdict, r.ResultCard)
		}
		tool := llm.requests[1].Messages[len(llm.requests[1].Messages)-1]
		mustContain(t, tool.Content, "forbidden", "POL-SEC-001")
	})

	t.Run("некорректные аргументы от LLM", func(t *testing.T) {
		llm := &fakeLLM{responses: []agent.LLMResponse{
			{ToolCalls: []agent.LLMToolCall{toolCall("c1", "playbook__search_docs", `не json`), toolCall("c2", "feature_flags__list", "")}},
			{Content: "Готово."},
		}}
		s := newStack(t, llm)
		r := s.ask("Какие сейчас эксперименты включены?", "")
		if r.Engine != domain.EngineLLM || len(r.ToolCalls) != 1 || r.ToolCalls[0].Tool != mcp.ToolListFlags {
			t.Fatalf("reply: %+v", r)
		}
		msgs := llm.requests[1].Messages
		if !strings.Contains(msgs[len(msgs)-2].Content, "не являются JSON") {
			t.Fatalf("LLM должна получить ошибку разбора аргументов: %s", msgs[len(msgs)-2].Content)
		}
	})
}

func TestLLMAgentFallsBackToSmartEngine(t *testing.T) {
	endless := agent.LLMResponse{ToolCalls: []agent.LLMToolCall{toolCall("c", "feature_flags__list", "{}")}}
	cases := []struct {
		name string
		llm  *fakeLLM
		want string
	}{
		{"LLM недоступна", &fakeLLM{err: errors.New("llm: сеть: timeout")}, "timeout"},
		{"пустой ответ", &fakeLLM{responses: []agent.LLMResponse{{Content: " "}}}, "пустой ответ"},
		{"нет финального ответа", &fakeLLM{responses: []agent.LLMResponse{endless}}, "не дала финальный ответ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t, tc.llm)
			r := s.ask("Как проходят встречи 1:1?", "")
			if r.Engine != domain.EngineSmart || !strings.Contains(r.LLMFallback, tc.want) {
				t.Fatalf("engine=%s fallback=%q", r.Engine, r.LLMFallback)
			}
			// Детерминированный ответ из базы знаний.
			if !r.Answered || len(r.Sources) != 1 || r.Sources[0].ID != "pb-one-on-one" {
				t.Fatalf("Smart Engine должен ответить сам: %+v", r)
			}
		})
	}
}
