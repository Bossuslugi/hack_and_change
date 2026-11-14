package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/spec"
)

// Execute выполняет MCP-инструмент по прямому подтверждению из интерфейса
// (например, отправка формы отгула). Права проверяет RBAC-шлюз по роли вызывающего.
func (s *Service) Execute(ctx context.Context, p domain.Principal, tool string, args map[string]any, messageID string, targetInternID ...string) (ChatResult, error) {
	started := time.Now()
	var res ChatResult
	if strings.TrimSpace(tool) == "" {
		return res, fmt.Errorf("%w: не указан инструмент", domain.ErrValidation)
	}
	internID := s.resolveInternID(p, targetInternID...)
	s.Events.Track(ctx, internID, p, domain.EventKindClickstream, "ui.action_confirmed", domain.SeverityInfo,
		map[string]any{"tool": tool, "message_id": messageID})

	call := s.Gateway.Execute(ctx, p, internID, tool, args)
	verdict := call.RBAC
	res = ChatResult{ToolCalls: []domain.MCPToolCall{call}, RBAC: &verdict, ChecklistUpdates: []domain.ChecklistItem{}}

	var updates []domain.ChecklistItem
	if call.OK() {
		var err error
		if updates, err = s.applyEffects(ctx, p, res.ToolCalls, nil, internID); err != nil {
			return res, err
		}
		res.ChecklistUpdates = nonNil(updates)
	}
	checklist, err := domain.LoadChecklist(ctx, s.Store, internID)
	if err != nil {
		return res, err
	}
	res.Checklist, res.Progress, res.NextStep = checklist, domain.ComputeProgress(checklist), domain.NextStep(checklist)

	if call.OK() && p.ID == internID {
		if messageID != "" {
			s.markCardSubmitted(ctx, messageID, internID)
		}
		text, card, saving := s.describeAction(call)
		if text == "" {
			// «Тихое» действие (например, личная задача в календаре): чат не засоряем.
			res.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
			return res, nil
		}
		msg := domain.ChatMessage{
			ID: s.IDs.NewID("msg"), InternID: internID, Role: domain.MessageAssistant,
			Text: text + checklistNote(updates, checklist), Intent: "action_confirmed", Engine: domain.EngineSmart,
			Sources: []domain.DocRef{}, ResultCard: card, Suggestions: []string{"Что дальше по чек-листу?"},
			ToolCallIDs: []string{call.ID}, CreatedAt: s.Clock.Now(),
		}
		if err := s.saveMessage(ctx, msg); err != nil {
			return res, err
		}
		res.Message = &msg
		payload := map[string]any{"tool": tool, "engine": domain.EngineSmart, "answered": true}
		if saving != "" {
			payload["saving_key"] = saving
		}
		s.Events.Track(ctx, internID, p, domain.EventKindAgent, "agent.action", domain.SeverityInfo, payload)
	}
	res.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	return res, nil
}

// markCardSubmitted помечает карточку исходного сообщения как отправленную (best-effort).
func (s *Service) markCardSubmitted(ctx context.Context, messageID, internID string) {
	msg, err := domain.LoadOne[domain.ChatMessage](ctx, s.Store, domain.KindMessage, messageID)
	if err != nil || msg.InternID != internID || msg.ActionCard == nil {
		return
	}
	msg.ActionCard.Status = domain.ActionStatusDone
	_ = s.saveMessage(ctx, msg)
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// describeAction формирует подтверждение и карточку результата по итогам инструмента.
func (s *Service) describeAction(call domain.MCPToolCall) (string, *domain.ResultCard, string) {
	r := call.Result
	switch call.Tool {
	case mcp.ToolCreateDayOff:
		when := agent.HumanDay(str(r, "date"))
		if from := str(r, "time_from"); from != "" {
			when += fmt.Sprintf(", %s–%s", from, str(r, "time_to"))
		}
		var notified []string
		if list, ok := r["notified"].([]any); ok {
			for _, n := range list {
				if name, ok := n.(string); ok {
					notified = append(notified, name)
				}
			}
		}
		who := strings.Join(notified, ", ")
		if who == "" {
			who = "—"
		}
		number := str(r, "request_number")
		text := fmt.Sprintf("Готово! Заявка **%s** создана в сервисе «Отсутствия»: %s. Уведомления отправлены: %s. "+
			"Статус — ожидает согласования, результат придёт в уведомлениях.", number, when, who)
		return text, &domain.ResultCard{
			Type: domain.CardAbsenceCreated, Status: domain.CardStatusSuccess, Title: "Заявка на отгул создана",
			Lines: []domain.CardLine{
				{Label: "Номер заявки", Value: number},
				{Label: "Когда", Value: when},
				{Label: "Причина", Value: str(r, "reason")},
				{Label: "Статус", Value: "Ожидает согласования"},
				{Label: "Уведомлены", Value: who},
			},
			Footnote: "Имитация сервиса «Отсутствия» · синтетические данные",
		}, domain.SavingAbsence
	case mcp.ToolRequestRepoAccess:
		text := fmt.Sprintf("Готово! Доступ к репозиторию **%s** в Stash оформлен заявкой на доступ.", str(r, "repository"))
		return text, &domain.ResultCard{
			Type: domain.CardAccessGranted, Status: domain.CardStatusSuccess, Title: "Доступ выдан",
			Lines: []domain.CardLine{
				{Label: "Проект / ресурс", Value: "Stash (Git)"},
				{Label: "Репозиторий", Value: str(r, "repository")},
				{Label: "Согласующий", Value: "Владелец проекта"},
				{Label: "Тестовый стенд", Value: str(r, "stand")},
				{Label: "Токен", Value: str(r, "token_masked")},
			},
			Footnote: "Имитация заявки на доступ в Stash · синтетические данные",
		}, domain.SavingAccess
	case mcp.ToolAddTask:
		return "", nil, ""
	case mcp.ToolCompleteItem:
		report := str(r, "report")
		if report == "" {
			return "", nil, ""
		}
		text := fmt.Sprintf("Принял отчёт по «%s» — спасибо! Главное передам наставнику в брифе к 1:1.", str(r, "title"))
		return text, &domain.ResultCard{
			Type: "report_sent", Status: domain.CardStatusSuccess, Title: "Отчёт отправлен Гоффи",
			Lines: []domain.CardLine{{Label: "Событие", Value: str(r, "title")}, {Label: "Отчёт", Value: report}},
		}, ""
	case mcp.ToolReviewDayOff:
		number := str(r, "request_number")
		decision := str(r, "decision")
		text := fmt.Sprintf("Заявка %s рассмотрена: статус %s.", number, decision)
		return text, &domain.ResultCard{
			Type: "absence_reviewed", Status: domain.CardStatusSuccess, Title: "Заявка рассмотрена",
			Lines: []domain.CardLine{
				{Label: "Номер заявки", Value: number},
				{Label: "Решение", Value: decision},
			},
			Footnote: "Имитация сервиса «Отсутствия» · синтетические данные",
		}, ""
	}
	return fmt.Sprintf("Инструмент %s выполнен.", call.Tool), &domain.ResultCard{
		Type: "tool_result", Status: domain.CardStatusInfo, Title: "Действие выполнено",
		Lines: []domain.CardLine{{Label: "Инструмент", Value: call.Tool}},
	}, ""
}

// CronResult — итог запуска ИИ-крона.
type CronResult struct {
	Triggered        bool                   `json:"triggered"`
	Deduplicated     bool                   `json:"deduplicated"`
	Reason           string                 `json:"reason"`
	Task             *domain.SprintTask     `json:"task"`
	ProactiveMessage *domain.ChatMessage    `json:"proactive_message"`
	Brief            *domain.MentorBrief    `json:"brief"`
	ToolCalls        []domain.MCPToolCall   `json:"tool_calls"`
	Checklist        []domain.ChecklistItem `json:"checklist"`
	Progress         domain.Progress        `json:"progress"`
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// learnedTopics собирает то, что стажёр уже сделал и изучил сам через Гоффи.
func (s *Service) learnedTopics(ctx context.Context) ([]string, error) {
	var topics []string
	read, err := s.docsRead(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range read {
		if d, ok := s.Services.Playbook.Doc(id); ok {
			topics = append(topics, fmt.Sprintf("Изучил инструкцию «%s»", d.Title))
		}
	}
	grants, err := domain.Load[domain.AccessGrant](ctx, s.Store, domain.KindAccessGrant, s.internID)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		topics = append(topics, fmt.Sprintf("Сам получил доступ к репозиторию %s и тестовому стенду", g.Repository))
	}
	absences, err := domain.Load[domain.AbsenceRequest](ctx, s.Store, domain.KindAbsence, s.internID)
	if err != nil {
		return nil, err
	}
	for _, a := range absences {
		topics = append(topics, fmt.Sprintf("Разобрался с оформлением отсутствий: заявка %s", a.Number))
	}
	planner, err := s.planner(ctx, s.internID)
	if err != nil {
		return nil, err
	}
	for _, it := range planner {
		if it.Status == domain.PlannerDone && it.Report != "" {
			topics = append(topics, fmt.Sprintf("Отчитался по «%s»: %s", it.Title, it.Report))
		}
	}
	if len(topics) == 0 {
		topics = append(topics, "Пока не обращался к Гоффи по процессам — стоит начать 1:1 с короткой вводной")
	}
	return topics, nil
}

// CronTrigger имитирует запуск фонового ИИ-крона Авито: если первая задача стажёра висит
// в работе без коммитов, отправляет стажёру проактивное сообщение и готовит наставнику
// Structured 1:1 Brief. Джоб исполняется от сервисного аккаунта system_cron.
func (s *Service) CronTrigger(ctx context.Context, p domain.Principal) (CronResult, error) {
	res := CronResult{ToolCalls: []domain.MCPToolCall{}}
	if err := s.authorize(ctx, p, OpCronTrigger); err != nil {
		return res, err
	}
	cronEmployee, ok := s.Directory.ByID(s.CronID)
	if !ok {
		return res, fmt.Errorf("сервисный аккаунт крона %q не найден", s.CronID)
	}
	cron := cronEmployee.Principal()

	finish := func(reason string) (CronResult, error) {
		res.Reason = reason
		checklist, err := domain.LoadChecklist(ctx, s.Store, s.internID)
		if err != nil {
			return res, err
		}
		res.Checklist, res.Progress = checklist, domain.ComputeProgress(checklist)
		payload := map[string]any{
			"triggered": res.Triggered, "deduplicated": res.Deduplicated, "reason": reason, "requested_by": p.ID,
		}
		if res.Brief != nil && !res.Deduplicated {
			payload["brief_id"], payload["saving_key"] = res.Brief.ID, domain.SavingBrief
		}
		s.Events.Track(ctx, s.internID, cron, domain.EventKindCron, "cron.run", domain.SeverityInfo, payload)
		return res, nil
	}

	if !s.Services.Flags.Enabled(ctx, mcp.FlagProactiveCron) {
		return finish("Feature flag " + mcp.FlagProactiveCron + " выключен — крон пропускает запуск")
	}

	activity := s.Gateway.Execute(ctx, cron, s.internID, mcp.ToolTaskActivity, map[string]any{"intern_id": s.internID})
	res.ToolCalls = append(res.ToolCalls, activity)
	if !activity.OK() {
		return finish("Трекер задач недоступен: " + activity.Error)
	}
	task, err := domain.LoadOne[domain.SprintTask](ctx, s.Store, domain.KindSprintTask, s.internID)
	if err != nil {
		return res, err
	}
	res.Task = &task
	if !task.Stalled() {
		return finish(fmt.Sprintf("Задача %s не требует внимания: статус %s, коммитов %d", task.Key, task.Status, task.Commits))
	}

	intern, err := domain.LoadOne[domain.InternProfile](ctx, s.Store, domain.KindIntern, s.internID)
	if err != nil {
		return res, err
	}
	learned, err := s.learnedTopics(ctx)
	if err != nil {
		return res, err
	}
	open, err := s.openQuestions(ctx)
	if err != nil {
		return res, err
	}
	briefs, err := domain.Load[domain.MentorBrief](ctx, s.Store, domain.KindBrief, s.internID)
	if err != nil {
		return res, err
	}
	res.Triggered = true
	if n := len(briefs); n > 0 {
		last := briefs[n-1]
		if last.TaskKey == task.Key && sameStrings(last.LearnedTopics, learned) && sameStrings(last.OpenQuestions, open) {
			res.Deduplicated, res.Brief = true, &last
			return finish("Бриф по задаче уже отправлен наставнику, новых данных нет — повторно не беспокоим")
		}
	}

	checklist, err := domain.LoadChecklist(ctx, s.Store, s.internID)
	if err != nil {
		return res, err
	}
	progress := domain.ComputeProgress(checklist)
	signals := []string{
		fmt.Sprintf("Задача %s «%s» в статусе in_progress %d дн., коммитов: %d", task.Key, task.Title, task.DaysInProgress, task.Commits),
		fmt.Sprintf("Прогресс чек-листа адаптации: %d из %d шагов", progress.Completed, progress.Total),
	}
	briefCall := s.Gateway.Execute(ctx, cron, s.internID, mcp.ToolSendBrief, map[string]any{
		"intern_id": s.internID, "mentor_id": intern.MentorID, "task_key": task.Key,
		"learned_topics": toAny(learned), "architecture_question": task.ArchitectureQuestion,
		"signals": toAny(signals), "open_questions": toAny(open),
	})
	res.ToolCalls = append(res.ToolCalls, briefCall)
	if !briefCall.OK() {
		res.Triggered = false
		return finish("Не удалось отправить бриф наставнику: " + briefCall.Error)
	}
	brief, err := domain.LoadOne[domain.MentorBrief](ctx, s.Store, domain.KindBrief, str(briefCall.Result, "brief_id"))
	if err != nil {
		return res, err
	}
	res.Brief = &brief

	msg := domain.ChatMessage{
		ID: s.IDs.NewID("msg"), InternID: s.internID, Role: domain.MessageAssistant, Proactive: true,
		Intent: "proactive_checkin", Engine: domain.EngineCron,
		Text: fmt.Sprintf("%s, привет! Вижу, что задача **%s** («%s») в работе уже %d-й день, а коммитов пока нет. "+
			"На первой неделе это нормально — давай помогу сдвинуться:\n"+
			"- подскажу, с чего начать и как разбить задачу на шаги;\n"+
			"- напомню, как катить изменения под фича-флагом и проходить ревью.\n\n"+
			"Архитектурный вопрос по задаче я уже передал наставнику (%s) в брифе к 1:1 — он придёт на встречу подготовленным.",
			intern.FirstName(), task.Key, task.Summary, task.DaysInProgress, intern.MentorName),
		Sources: []domain.DocRef{},
		Suggestions: []string{
			agent.ScenarioPrompts["first_task"],
			agent.ScenarioPrompts["team_processes"],
		},
		ToolCallIDs: callIDs(res.ToolCalls), CreatedAt: s.Clock.Now(),
	}
	if err := s.saveMessage(ctx, msg); err != nil {
		return res, err
	}
	res.ProactiveMessage = &msg
	if _, err := s.applyEffects(ctx, cron, res.ToolCalls, nil); err != nil {
		return res, err
	}
	return finish(fmt.Sprintf("Задача %s без коммитов %d дн.: стажёру отправлено проактивное сообщение, наставнику — бриф к 1:1",
		task.Key, task.DaysInProgress))
}

func toAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

// Reset возвращает демо-данные в исходное состояние.
func (s *Service) Reset(ctx context.Context, p domain.Principal) (WorkspaceView, error) {
	if err := s.authorize(ctx, p, OpReset); err != nil {
		return WorkspaceView{}, err
	}
	if err := s.resetData(ctx); err != nil {
		return WorkspaceView{}, err
	}
	s.Events.Track(ctx, s.internID, p, domain.EventKindClickstream, "demo.reset", domain.SeverityInfo, nil)
	return s.Workspace(ctx, p)
}

// ToolInfo — контракт инструмента с признаком доступности для роли смотрящего.
type ToolInfo struct {
	spec.Tool
	AllowedForViewer bool `json:"allowed_for_viewer"`
}

// TelemetryView — данные инспектора телеметрии (MCP + RBAC + Clickstream).
type TelemetryView struct {
	Events    []domain.TelemetryEvent `json:"events"`
	ToolCalls []domain.MCPToolCall    `json:"tool_calls"`
	Tools     []ToolInfo              `json:"tools"`
	Servers   []spec.Server           `json:"servers"`
	Roles     []spec.Role             `json:"roles"`
	Matrix    []rbac.MatrixRow        `json:"matrix"`
}

const telemetryLimit = 200

func latest[T any](in []T, limit int) []T {
	out := reverse(in)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Telemetry возвращает журнал событий, вызовов MCP и матрицу RBAC для инспектора.
func (s *Service) Telemetry(ctx context.Context, p domain.Principal) (TelemetryView, error) {
	var v TelemetryView
	if err := s.authorize(ctx, p, OpTelemetryRead); err != nil {
		return v, err
	}
	events, err := domain.Load[domain.TelemetryEvent](ctx, s.Store, domain.KindEvent, s.internID)
	if err != nil {
		return v, err
	}
	calls, err := domain.Load[domain.MCPToolCall](ctx, s.Store, domain.KindToolCall, s.internID)
	if err != nil {
		return v, err
	}
	reg := s.Gateway.Registry()
	tools := make([]ToolInfo, 0, len(reg.Tools()))
	for _, t := range reg.Tools() {
		tools = append(tools, ToolInfo{Tool: t, AllowedForViewer: s.RBAC.Can(p.Role, t.Name)})
	}
	return TelemetryView{
		Events: latest(events, telemetryLimit), ToolCalls: latest(calls, telemetryLimit),
		Tools: tools, Servers: reg.Servers(), Roles: s.RBAC.Roles(), Matrix: s.RBAC.Matrix(),
	}, nil
}

// SetFlag переключает runtime feature flag (только руководитель).
func (s *Service) SetFlag(ctx context.Context, p domain.Principal, key string, enabled bool) (domain.FeatureFlag, error) {
	if err := s.authorize(ctx, p, OpFlagsWrite); err != nil {
		return domain.FeatureFlag{}, err
	}
	flag, err := s.Services.Flags.Set(ctx, key, enabled)
	if err != nil {
		return flag, err
	}
	s.Events.Track(ctx, s.internID, p, domain.EventKindClickstream, "feature_flag.changed", domain.SeverityInfo,
		map[string]any{"key": key, "enabled": enabled})
	return flag, nil
}

var uiEventRe = regexp.MustCompile(`^ui\.[a-z0-9_.]{1,48}$`)

// TrackUI записывает clickstream-событие интерфейса (имя вида ui.*; payload — плоский).
func (s *Service) TrackUI(ctx context.Context, p domain.Principal, name string, payload map[string]any) (domain.TelemetryEvent, error) {
	if !uiEventRe.MatchString(name) {
		return domain.TelemetryEvent{}, fmt.Errorf("%w: имя события должно иметь вид ui.<название>", domain.ErrValidation)
	}
	clean := map[string]any{}
	for k, v := range payload {
		if len(clean) >= 10 {
			break
		}
		switch t := v.(type) {
		case string:
			if len([]rune(t)) > 200 {
				t = string([]rune(t)[:200])
			}
			clean[k] = t
		case float64, bool, int:
			clean[k] = t
		}
	}
	return s.Events.Track(ctx, s.internID, p, domain.EventKindClickstream, name, domain.SeverityInfo, clean), nil
}

// HandleMCP обрабатывает JSON-RPC запрос протокола MCP от имени аутентифицированного субъекта.
func (s *Service) HandleMCP(ctx context.Context, p domain.Principal, raw []byte) *mcp.RPCResponse {
	return s.Gateway.HandleRPC(ctx, p, s.internID, raw)
}

// Health проверяет доступность хранилища и кэша.
func (s *Service) Health(ctx context.Context) (StorageInfo, error) {
	info := StorageInfo{Store: s.Store.Name(), Cache: s.Cache.Name()}
	return info, errors.Join(s.Store.Ping(ctx), s.Cache.Ping(ctx))
}
