package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/spec"
)

// ToolExecutor — RBAC-шлюз MCP (реализуется mcp.Gateway).
type ToolExecutor interface {
	Execute(ctx context.Context, p domain.Principal, internID, tool string, args map[string]any) domain.MCPToolCall
}

// ToolCatalog — каталог контрактов инструментов (реализуется mcp.Registry).
type ToolCatalog interface {
	Tools() []spec.Tool
	Tool(name string) (spec.Tool, bool)
}

// FlagReader — чтение feature flags.
type FlagReader interface {
	Enabled(ctx context.Context, key string) bool
}

// Stage — этап обработки запроса (для статуса ожидания в интерфейсе).
type Stage struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Tool  string `json:"tool,omitempty"`
}

// Виды этапов.
const (
	StageUnderstanding = "understanding"
	StageRBAC          = "rbac"
	StageTool          = "tool"
	StageLLM           = "llm"
	StageComposing     = "composing"
)

// Request — запрос к агенту.
type Request struct {
	Principal  domain.Principal
	Intern     domain.InternProfile
	Text       string
	Checklist  []domain.ChecklistItem
	Task       domain.SprintTask
	LastIntent string
	Emit       func(Stage)
}

// Reply — ответ агента (без побочных эффектов на чек-лист: их применяет usecase).
type Reply struct {
	Intent       string
	Scenario     string
	Engine       string
	Text         string
	Sources      []domain.DocRef
	ActionCard   *domain.ActionCard
	ResultCard   *domain.ResultCard
	Suggestions  []string
	ToolCalls    []domain.MCPToolCall
	Verdict      *domain.RBACVerdict
	Answered     bool
	SavingKey    string
	DocsRead     []string
	OpenQuestion string
	LLMFallback  string // причина отката на Smart Engine, если LLM была вызвана и не справилась

	facts []string // факты, которые LLM обязана сохранить при переформулировке
}

// Engine — оркестратор агента.
type Engine struct {
	spec    *spec.AgentSpec
	tools   ToolExecutor
	catalog ToolCatalog
	flags   FlagReader
	llm     LLM
	clock   domain.Clock
	delay   time.Duration
	sleep   func(ctx context.Context, d time.Duration)
}

// Options — параметры оркестратора.
type Options struct {
	Spec    *spec.AgentSpec
	Tools   ToolExecutor
	Catalog ToolCatalog
	Flags   FlagReader
	LLM     LLM // nil — только Smart Engine
	Clock   domain.Clock
	// Delay — суммарная имитация «печатает…» Smart Engine; <0 — взять из спецификации.
	Delay time.Duration
}

// New создаёт оркестратор.
func New(o Options) *Engine {
	delay := o.Delay
	if delay < 0 {
		delay = time.Duration(o.Spec.Agent.Engine.SmartDelayMS) * time.Millisecond
	}
	// Интерфейс с nil-указателем внутри не равен nil — нормализуем.
	if c, ok := o.LLM.(*OpenAIClient); ok && c == nil {
		o.LLM = nil
	}
	return &Engine{
		spec: o.Spec, tools: o.Tools, catalog: o.Catalog, flags: o.Flags, llm: o.LLM, clock: o.Clock,
		delay: delay, sleep: sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Mode возвращает режим работы и модель для отображения в интерфейсе.
func (e *Engine) Mode(ctx context.Context) (mode, model string) {
	if e.llmEnabled(ctx) {
		return domain.EngineLLM, e.llm.Model()
	}
	return domain.EngineSmart, "Smart Engine (детерминированный)"
}

func (e *Engine) llmEnabled(ctx context.Context) bool {
	return e.llm != nil && e.flags.Enabled(ctx, mcp.FlagLLMAnswers)
}

func (e *Engine) pause(ctx context.Context) { e.sleep(ctx, e.delay/3) }

func emit(req Request, kind, label, tool string) {
	if req.Emit != nil {
		req.Emit(Stage{Kind: kind, Label: label, Tool: tool})
	}
}

// Handle обрабатывает сообщение стажёра.
func (e *Engine) Handle(ctx context.Context, req Request) Reply {
	emit(req, StageUnderstanding, "Разбираюсь в запросе…", "")
	e.pause(ctx)

	intent := Classify(e.spec, req.Text)
	var r Reply
	switch intent.ID {
	case IntentAffirmative:
		intent, r = e.affirmative(ctx, req, intent)
	case IntentRestricted:
		r = e.restricted(ctx, req, intent)
	case IntentAccess:
		r = e.access(ctx, req, intent)
	case IntentAbsence:
		r = e.absence(ctx, req, intent)
	case IntentProcess, IntentTaskHelp:
		r = e.knowledge(ctx, req, intent)
	case IntentNextStep:
		r = e.nextStep(req)
	case IntentMentor:
		r = e.mentorContact(req)
	case IntentGreeting:
		r = e.greeting(req)
	default:
		var ok bool
		if e.llmEnabled(ctx) {
			r, ok = e.llmAgent(ctx, req)
		}
		if !ok {
			reason := r.LLMFallback
			r = e.knowledge(ctx, req, intent)
			if reason != "" {
				// LLM уже не справилась в этом ходе: отвечает Smart Engine, повторно её не дёргаем.
				r.LLMFallback, r.Engine = reason, domain.EngineSmart
			}
		}
	}

	r.Intent, r.Scenario = intent.ID, intent.Scenario
	if len(r.Suggestions) == 0 {
		r.Suggestions = append([]string{}, intent.Suggestions...)
	}
	if r.Verdict == nil {
		r.Verdict = pickVerdict(r.ToolCalls)
	}
	if r.Engine == "" {
		r.Engine = domain.EngineSmart
		if e.llmEnabled(ctx) && rephrasable(intent.ID) && r.Answered {
			e.rephrase(ctx, req, &r)
		}
	}
	emit(req, StageComposing, "Формулирую ответ…", "")
	if r.Engine == domain.EngineSmart {
		e.pause(ctx)
	}
	return r
}

// pickVerdict выбирает итоговый вердикт RBAC: первый запрет, иначе вердикт последнего вызова.
func pickVerdict(calls []domain.MCPToolCall) *domain.RBACVerdict {
	if len(calls) == 0 {
		return nil
	}
	for i := range calls {
		if !calls[i].RBAC.Allowed {
			v := calls[i].RBAC
			return &v
		}
	}
	v := calls[len(calls)-1].RBAC
	return &v
}

// rephrasable — интенты, ответы на которые можно отдать LLM на переформулировку.
// Отказ по безопасности (restricted_data_export) всегда детерминирован.
func rephrasable(intent string) bool {
	switch intent {
	case IntentProcess, IntentTaskHelp, IntentKnowledge, IntentAbsence, IntentAccess, IntentNextStep, IntentMentor:
		return true
	}
	return false
}

func (e *Engine) call(ctx context.Context, req Request, tool string, args map[string]any, label string) domain.MCPToolCall {
	emit(req, StageRBAC, "Проверяю, можно ли это сделать…", tool)
	emit(req, StageTool, label, tool)
	e.pause(ctx)
	return e.tools.Execute(ctx, req.Principal, req.Intern.ID, tool, args)
}

// friendlyReason объясняет отказ человеческим языком. Идентификаторы политик и коды RBAC
// стажёру не показываются — они доступны экспертам в «Инспекторе жюри».
func friendlyReason(v domain.RBACVerdict) string {
	switch {
	case v.Constraint == "team_scope":
		return "Этот ресурс принадлежит другой команде"
	case v.Constraint == "self_only":
		return "Такое действие можно выполнять только для себя"
	case v.Incident:
		return "Данные с персональной и финансовой информацией закрыты для всех ролей"
	}
	return "У твоей роли нет прав на это действие"
}

func deniedCard(v domain.RBACVerdict) *domain.ResultCard {
	card := &domain.ResultCard{
		Type: domain.CardRBACDenied, Status: domain.CardStatusDenied,
		Title:    "Действие недоступно",
		Lines:    []domain.CardLine{{Label: "Почему", Value: friendlyReason(v)}},
		Footnote: "Решение принимает политика безопасности, а не Гоффи",
	}
	if v.Incident {
		card.Lines = append(card.Lines, domain.CardLine{Label: "Журнал безопасности", Value: "запрос отмечен автоматически — это штатная процедура"})
	}
	if v.SafeAlternative != "" {
		card.Lines = append(card.Lines, domain.CardLine{Label: "Что можно вместо этого", Value: v.SafeAlternative})
	}
	return card
}

func (e *Engine) toolFailure(req Request, call domain.MCPToolCall) string {
	if call.Status == domain.ToolInvalid {
		return e.spec.Response("invalid_arguments", map[string]string{"details": call.Error})
	}
	return e.spec.Response("tool_error", map[string]string{"server": call.Server, "mentor": req.Intern.MentorName})
}

// ---------- Сценарий 1Б: запрос закрытых данных ----------

func (e *Engine) restricted(ctx context.Context, req Request, intent spec.Intent) Reply {
	dataset := "users_pii"
	if domain.MatchTrigger([]string{"зарплат|ведомост|оклад|salary|payroll"}, req.Text) {
		dataset = "payroll"
	}
	call := e.call(ctx, req, intent.Tool, map[string]any{
		"dataset": dataset, "reason": truncate(req.Text, 500), "requested_by": req.Principal.ID,
	}, "Сверяюсь с политикой безопасности…")

	r := Reply{ToolCalls: []domain.MCPToolCall{call}, Answered: true, SavingKey: domain.SavingBlocked}
	v := call.RBAC
	if call.Status != domain.ToolForbidden {
		// Недостижимо при корректной матрице; на случай ошибки в спецификации отказываем всё равно.
		v = domain.RBACVerdict{
			Allowed: false, Code: domain.VerdictForbidden, HTTPStatus: 403, Role: req.Principal.Role,
			Operation: intent.Tool, PolicyID: "POL-SEC-001", Incident: true,
			Reason: "Выгрузка production-данных с ПДн и финансовых сведений запрещена",
		}
	}
	r.Verdict = &v
	r.Text = e.spec.Response("rbac_forbidden", map[string]string{
		"operation": v.Operation, "policy": v.PolicyID, "role": string(v.Role),
	}) + "\n\n" + e.spec.Response("rbac_safe_alternative", nil)
	r.ResultCard = deniedCard(v)
	return r
}

// ---------- Сценарий 1А: доступ к репозиторию и стенду ----------

func (e *Engine) access(ctx context.Context, req Request, intent spec.Intent) Reply {
	repo := ExtractRepository(req.Text)
	if repo == "" {
		repo = req.Intern.Repository
	}
	stand := "test"
	if domain.MatchTrigger([]string{"staging|стейдж|стейджинг"}, req.Text) {
		stand = "staging"
	}
	call := e.call(ctx, req, intent.Tool, map[string]any{
		"intern_id": req.Intern.ID, "repository": repo, "stand": stand, "access_level": "developer",
		"justification": "Онбординг стажёра: " + req.Intern.ContextLine(),
	}, "Оформляю доступ…")
	r := Reply{ToolCalls: []domain.MCPToolCall{call}, Answered: true}

	switch call.Status {
	case domain.ToolSuccess:
		res := call.Result
		standName, _ := res["stand"].(string)
		token, _ := res["token_masked"].(string)
		level, _ := res["access_level"].(string)
		expires := ""
		if s, _ := res["expires_at"].(string); len(s) >= 10 {
			if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
				expires = t.Format("02.01.2006")
			}
		}
		already, _ := res["already_granted"].(bool)
		var b strings.Builder
		if already {
			fmt.Fprintf(&b, "Доступ к **%s** у тебя уже есть — повторно выдавать не нужно.\n\n", repo)
		} else {
			b.WriteString("Готово! Я проверил твои права и оформил доступ — можно работать.\n\n")
		}
		fmt.Fprintf(&b, "- Репозиторий в Stash: **%s**, уровень прав: %s\n", repo, level)
		if standName != "" {
			fmt.Fprintf(&b, "- Тестовый стенд: **%s** (только синтетические данные)\n", standName)
		}
		fmt.Fprintf(&b, "- Токен: %s — синтетический, действует до %s", token, expires)
		r.Text = b.String()
		if !already {
			r.SavingKey = domain.SavingAccess // повторный запрос не экономит время наставника
		}
		r.facts = []string{repo}
		// Состав карточки повторяет заявку на доступ: ресурс, уровень прав, обоснование, согласующий.
		lines := []domain.CardLine{
			{Label: "Проект / ресурс", Value: "Stash (Git)"},
			{Label: "Репозиторий", Value: repo},
			{Label: "Уровень прав", Value: level},
			{Label: "Обоснование", Value: "Шаг чек-листа адаптации: доступы к репозиторию команды"},
			{Label: "Согласующий", Value: "Владелец проекта"},
		}
		if standName != "" {
			lines = append(lines, domain.CardLine{Label: "Тестовый стенд", Value: standName})
		}
		lines = append(lines,
			domain.CardLine{Label: "Токен", Value: token},
			domain.CardLine{Label: "Действует до", Value: expires},
		)
		r.ResultCard = &domain.ResultCard{
			Type: domain.CardAccessGranted, Status: domain.CardStatusSuccess, Title: "Доступ выдан",
			Lines: lines, Footnote: "Имитация заявки на доступ в Stash · синтетические данные",
		}
	case domain.ToolForbidden:
		r.Text = fmt.Sprintf(
			"Не могу выдать доступ к **%s**: %s. "+
				"Доступ к репозиториям других команд запрашивает руководитель — %s.\n\n"+
				"Могу оформить доступ к репозиторию твоей команды: **%s**.",
			repo, lowerFirst(friendlyReason(call.RBAC)), req.Intern.TeamLeadName, req.Intern.Repository)
		r.ResultCard = deniedCard(call.RBAC)
		r.Suggestions = []string{fmt.Sprintf("Дай доступ к репозиторию %s и тестовому стенду", req.Intern.Repository)}
	default:
		r.Text = e.toolFailure(req, call)
		r.Answered = false
	}
	return r
}

// ---------- Сценарий 2: вопросы о процессах и база знаний ----------

func (e *Engine) search(ctx context.Context, req Request, topics []string) (domain.MCPToolCall, []domain.DocRef) {
	args := map[string]any{
		"query": truncate(req.Text, 500), "team": req.Intern.Team, "stack": req.Intern.Stack,
		"stage": req.Intern.Stage, "limit": 3,
	}
	if len(topics) > 0 {
		list := make([]any, 0, len(topics))
		for _, t := range topics {
			list = append(list, t)
		}
		args["topics"] = list
	}
	call := e.call(ctx, req, mcp.ToolSearchDocs, args, "Ищу в базе знаний команды…")
	if !call.OK() {
		return call, nil
	}
	return call, mcp.DocRefs(call.Result)
}

func writeDoc(b *strings.Builder, d domain.DocRef) {
	fmt.Fprintf(b, "**%s**\n", d.Title)
	if d.Summary != "" {
		b.WriteString(d.Summary + "\n")
	}
	for i, step := range d.Steps {
		fmt.Fprintf(b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\n")
}

func docIDs(docs []domain.DocRef) []string {
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	return ids
}

func (e *Engine) knowledge(ctx context.Context, req Request, intent spec.Intent) Reply {
	call, docs := e.search(ctx, req, intent.Topics)
	r := Reply{ToolCalls: []domain.MCPToolCall{call}}
	if !call.OK() {
		r.Text = e.toolFailure(req, call)
		return r
	}
	if len(docs) == 0 {
		r.Text = e.spec.Response("not_found", map[string]string{"mentor": req.Intern.MentorName})
		r.OpenQuestion = truncate(req.Text, 300)
		return r
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Учитываю твой контекст: %s.\n\n", req.Intern.ContextLine())
	if intent.ID == IntentTaskHelp && req.Task.Key != "" {
		fmt.Fprintf(&b, "Твоя задача **%s**: %s.\n\n", req.Task.Key, req.Task.Summary)
	}
	for _, d := range docs {
		writeDoc(&b, d)
	}
	if strings.HasPrefix(req.Intern.Stage, "Week 1") {
		switch intent.ID {
		case IntentProcess:
			fmt.Fprintf(&b, "Для первой недели: первую выкладку и включение флага делай в паре с наставником (%s) — так принято в команде %s.\n\n",
				req.Intern.MentorName, req.Intern.Team)
		case IntentTaskHelp:
			fmt.Fprintf(&b, "Если после этого останется архитектурный вопрос — я добавлю его в бриф к 1:1 с наставником (%s).\n\n", req.Intern.MentorName)
		}
	}
	b.WriteString("Источники — тестовые инструкции (синтетические данные), а не регламенты Авито.")

	r.Text = b.String()
	r.Sources = docs
	r.DocsRead = docIDs(docs)
	r.Answered = true
	r.SavingKey = domain.SavingQuestion
	return r
}

// ---------- Сценарий 3: отгул ----------

func (e *Engine) absence(ctx context.Context, req Request, intent spec.Intent) Reply {
	call, docs := e.search(ctx, req, intent.Topics)
	r := Reply{ToolCalls: []domain.MCPToolCall{call}, Sources: docs, DocsRead: docIDs(docs), Answered: true, SavingKey: domain.SavingQuestion}

	var b strings.Builder
	fmt.Fprintf(&b, "Коротко: отдельно писать руководителю не нужно — заявка в сервисе «Отсутствия» сама уведомит наставника (%s) и руководителя (%s). "+
		"Если из-за отгула пропускаешь встречу — предупреди наставника в командном чате.\n\n", req.Intern.MentorName, req.Intern.TeamLeadName)
	if len(docs) > 0 {
		fmt.Fprintf(&b, "**Порядок в команде %s:**\n", req.Intern.Team)
		for i, step := range docs[0].Steps {
			fmt.Fprintf(&b, "%d. %s\n", i+1, step)
		}
		b.WriteString("\n")
	}

	if !e.flags.Enabled(ctx, mcp.FlagAbsenceCard) {
		b.WriteString("Интерактивная карточка оформления сейчас выключена feature-флагом — оформи заявку в сервисе «Отсутствия» по шагам выше.")
		r.Text = b.String()
		return r
	}

	draft := ParseAbsence(req.Text, e.clock.Now())
	if draft.DateFound {
		fmt.Fprintf(&b, "Я подготовил заявку: **%s, %s–%s**, причина — «%s». ", HumanDay(draft.Date), draft.TimeFrom, draft.TimeTo, draft.Reason)
		r.facts = []string{draft.TimeFrom}
	} else {
		b.WriteString("Я подготовил заявку, но не понял дату — укажи её в карточке. ")
	}
	b.WriteString("Проверь данные и нажми «Оформить отгул»: без твоего подтверждения я ничего не отправляю.")
	r.Text = b.String()
	r.ActionCard = &domain.ActionCard{
		Type: domain.ActionAbsenceForm, Title: "Заявка на отгул",
		Description: "Сервис «Отсутствия» (имитация) · после подтверждения уведомлю наставника и руководителя",
		Tool:        intent.ConfirmTool,
		Fields: map[string]any{
			"intern_id": req.Intern.ID, "date": draft.Date, "time_from": draft.TimeFrom, "time_to": draft.TimeTo,
			"reason": draft.Reason, "notify": []any{"mentor", "team_lead"},
		},
		SubmitLabel: "Оформить отгул", Status: domain.ActionStatusPending,
	}
	return r
}

// ---------- Служебные интенты ----------

// ScenarioPrompts — подсказки-запросы по шагам чек-листа.
var ScenarioPrompts = map[string]string{
	"access_repo":      "Дай доступ к репозиторию avito-autoteka-api и тестовому стенду",
	"team_processes":   "Как в нашей команде катить задачу под фича-флагом и проходить ревью?",
	"first_task":       "Я застрял на первой задаче, с чего начать?",
	"absence_rules":    "Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?",
	"first_one_on_one": "Как проходят встречи 1:1?",
}

func (e *Engine) nextStep(req Request) Reply {
	p := domain.ComputeProgress(req.Checklist)
	var b strings.Builder
	fmt.Fprintf(&b, "Прогресс адаптации: %d из %d шагов (%d%%).\n\n", p.Completed, p.Total, p.Percent)
	marks := map[domain.ChecklistStatus]string{
		domain.StatusCompleted: "выполнено", domain.StatusInProgress: "в работе", domain.StatusPending: "впереди",
	}
	for _, it := range req.Checklist {
		fmt.Fprintf(&b, "- %s — %s\n", it.Title, marks[it.Status])
	}
	r := Reply{Answered: true, SavingKey: domain.SavingQuestion}
	if next := domain.NextStep(req.Checklist); next != nil {
		fmt.Fprintf(&b, "\n**Следующий шаг:** %s. %s", next.Title, next.Description)
		if prompt, ok := ScenarioPrompts[next.ID]; ok {
			r.Suggestions = []string{prompt}
		}
	} else {
		b.WriteString("\nВсе шаги чек-листа выполнены — отличная работа!")
	}
	r.Text = b.String()
	return r
}

func (e *Engine) mentorContact(req Request) Reply {
	in := req.Intern
	text := fmt.Sprintf(
		"Твой наставник — **%s**: вопросы по задачам, коду, ревью и всему, где нужен опыт.\n"+
			"Руководитель команды — **%s**: цели стажировки, ожидания, согласование отсутствий.\n\n"+
			"Рутину (доступы, процессы команды, оформление отгула) закрываю я. "+
			"Если вопрос требует сеньора — добавлю его в бриф к встрече 1:1, чтобы наставник пришёл подготовленным.",
		in.MentorName, in.TeamLeadName)
	return Reply{Text: text, Answered: true, SavingKey: domain.SavingQuestion}
}

func (e *Engine) greeting(req Request) Reply {
	return Reply{
		Text:     e.spec.Response("greeting", map[string]string{"name": req.Intern.FirstName(), "team": req.Intern.Team}),
		Answered: true,
	}
}

// affirmative продолжает предыдущий сценарий по контексту сессии.
func (e *Engine) affirmative(ctx context.Context, req Request, intent spec.Intent) (spec.Intent, Reply) {
	switch req.LastIntent {
	case IntentRestricted:
		// После отказа предложена песочница: «да» = запросить доступ к репозиторию и стенду.
		if access, ok := e.spec.IntentByID(IntentAccess); ok {
			req.Text = fmt.Sprintf("Дай доступ к репозиторию %s и тестовому стенду", req.Intern.Repository)
			return access, e.access(ctx, req, access)
		}
	case IntentAbsence:
		return intent, Reply{
			Text:     "Чтобы отправить заявку, нажми «Оформить отгул» в карточке выше: действия в корпоративных сервисах я выполняю только после явного подтверждения в интерфейсе.",
			Answered: true,
		}
	}
	greeting, _ := e.spec.IntentByID(IntentGreeting)
	return intent, Reply{
		Text:        "Уточни, пожалуйста, с чем помочь — например, с доступами, процессами команды или отгулом.",
		Answered:    true,
		Suggestions: append([]string{}, greeting.Suggestions...),
	}
}

// ---------- LLM ----------

func (e *Engine) systemPrompt(req Request) string {
	in := req.Intern
	next := "все шаги выполнены"
	if n := domain.NextStep(req.Checklist); n != nil {
		next = n.Title
	}
	return strings.TrimSpace(e.spec.Agent.SystemPrompt) + fmt.Sprintf(
		"\n\nКонтекст стажёра (синтетические данные):\n- intern.name: %s\n- intern.role: %s\n- intern.team: %s\n- intern.stack: %s\n"+
			"- intern.stage: %s\n- intern.mentor: %s\n- intern.team_lead: %s\n- checklist.next_step: %s\n- session.last_intent: %s\n- today: %s",
		in.Name, in.Role, in.Team, in.Stack, in.Stage, in.MentorName, in.TeamLeadName, next, req.LastIntent,
		e.clock.Now().Format("2006-01-02 (Monday)"))
}

// rephrase просит LLM переформулировать детерминированный черновик, не меняя фактов.
// При любой ошибке или потере фактов остаётся черновик Smart Engine.
func (e *Engine) rephrase(ctx context.Context, req Request, r *Reply) {
	emit(req, StageLLM, "Формулирую ответ с помощью LLM…", "")
	resp, err := e.llm.Complete(ctx, LLMRequest{Messages: []LLMMessage{
		{Role: "system", Content: e.systemPrompt(req)},
		{Role: "user", Content: fmt.Sprintf(
			"Запрос стажёра: «%s»\n\nЧерновик ответа, собранный из результатов MCP-инструментов и базы знаний:\n---\n%s\n---\n"+
				"Перепиши черновик естественным языком. Сохрани ВСЕ факты, шаги, имена, номера, идентификаторы политик и названия "+
				"репозиториев дословно. Ничего не добавляй от себя. Допустимая разметка: **жирный**, нумерованные и маркированные списки. "+
				"Верни только текст ответа.", truncate(req.Text, 500), r.Text)},
	}})
	switch {
	case err != nil:
		r.LLMFallback = err.Error()
	case strings.TrimSpace(resp.Content) == "":
		r.LLMFallback = "LLM вернула пустой ответ"
	case !keepsFacts(resp.Content, r.facts):
		r.LLMFallback = "LLM потеряла факты из результата инструмента"
	default:
		r.Text, r.Engine = strings.TrimSpace(resp.Content), domain.EngineLLM
	}
}

func keepsFacts(text string, facts []string) bool {
	for _, f := range facts {
		if f != "" && !strings.Contains(text, f) {
			return false
		}
	}
	return true
}

const maxLLMRounds = 3

func llmToolName(name string) string { return strings.ReplaceAll(name, ".", "__") }
func mcpToolName(name string) string { return strings.ReplaceAll(name, "__", ".") }
func jsonText(v any) string          { raw, _ := json.Marshal(v); return string(raw) }
func hasProperty(t spec.Tool, key string) bool {
	props, _ := t.InputSchema["properties"].(map[string]any)
	_, ok := props[key]
	return ok
}

// llmAgent — свободный режим для запросов вне известных сценариев: LLM сама выбирает
// MCP-инструменты (function calling). Каждый вызов проходит тот же RBAC-шлюз;
// инструменты с requires_confirmation не исполняются, а превращаются в карточку подтверждения.
func (e *Engine) llmAgent(ctx context.Context, req Request) (Reply, bool) {
	emit(req, StageLLM, "Думаю вместе с LLM…", "")
	var tools []LLMTool
	for _, t := range e.catalog.Tools() {
		tools = append(tools, LLMTool{Type: "function", Function: LLMFunction{
			Name: llmToolName(t.Name), Description: t.Description, Parameters: t.InputSchema,
		}})
	}
	messages := []LLMMessage{
		{Role: "system", Content: e.systemPrompt(req)},
		{Role: "user", Content: truncate(req.Text, 2000)},
	}
	r := Reply{Engine: domain.EngineLLM}
	for round := 0; round < maxLLMRounds; round++ {
		resp, err := e.llm.Complete(ctx, LLMRequest{Messages: messages, Tools: tools})
		if err != nil {
			return Reply{LLMFallback: err.Error()}, false
		}
		if len(resp.ToolCalls) == 0 {
			if strings.TrimSpace(resp.Content) == "" {
				return Reply{LLMFallback: "LLM вернула пустой ответ"}, false
			}
			r.Text = resp.Content
			break
		}
		messages = append(messages, LLMMessage{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			messages = append(messages, LLMMessage{Role: "tool", ToolCallID: tc.ID, Content: e.runLLMTool(ctx, req, tc, &r)})
		}
	}
	if r.Text == "" {
		return Reply{LLMFallback: "LLM не дала финальный ответ за отведённое число шагов"}, false
	}
	r.Answered = true
	r.SavingKey = domain.SavingQuestion
	if v := pickVerdict(r.ToolCalls); v != nil && !v.Allowed {
		r.ResultCard = deniedCard(*v)
		if v.Incident {
			r.SavingKey = domain.SavingBlocked
		}
	}
	return r, true
}

func (e *Engine) runLLMTool(ctx context.Context, req Request, tc LLMToolCall, r *Reply) string {
	name := mcpToolName(tc.Function.Name)
	args := map[string]any{}
	if raw := strings.TrimSpace(tc.Function.Arguments); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return jsonText(map[string]any{"status": "invalid", "error": "аргументы не являются JSON-объектом"})
		}
	}
	toolSpec, known := e.catalog.Tool(name)
	if known && hasProperty(toolSpec, "intern_id") {
		args["intern_id"] = req.Intern.ID // агент действует только от имени текущего стажёра
	}
	if known && toolSpec.RequiresConfirmation {
		if _, ok := args["notify"]; !ok {
			args["notify"] = []any{"mentor", "team_lead"}
		}
		r.ActionCard = &domain.ActionCard{
			Type: domain.ActionAbsenceForm, Title: "Заявка на отгул",
			Description: "Сервис «Отсутствия» (имитация) · после подтверждения уведомлю наставника и руководителя",
			Tool:        name, Fields: args, SubmitLabel: "Оформить отгул", Status: domain.ActionStatusPending,
		}
		return jsonText(map[string]any{
			"status": "awaiting_user_confirmation",
			"note":   "Пользователю показана карточка подтверждения. Действие будет выполнено только после его подтверждения в интерфейсе.",
		})
	}
	call := e.call(ctx, req, name, args, "Обращаюсь к корпоративному сервису…")
	r.ToolCalls = append(r.ToolCalls, call)
	if call.OK() && name == mcp.ToolSearchDocs {
		docs := mcp.DocRefs(call.Result)
		r.Sources = append(r.Sources, docs...)
		r.DocsRead = append(r.DocsRead, docIDs(docs)...)
	}
	return jsonText(map[string]any{
		"status": call.Status, "result": call.Result, "error_code": call.ErrorCode, "error": call.Error,
		"rbac": map[string]any{"code": call.RBAC.Code, "policy_id": call.RBAC.PolicyID, "reason": call.RBAC.Reason},
	})
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[0])) + string(r[1:])
}
