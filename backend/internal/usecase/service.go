// Package usecase — прикладные сценарии Гоффи: workspace, чат, подтверждение действий,
// ИИ-крон, сброс демо. Связывает агента, MCP-шлюз, RBAC и хранилище.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
)

// Операции REST API в матрице RBAC.
const (
	OpWorkspaceRead   = "api.workspace.read"
	OpChatSend        = "api.chat.send"
	OpChatHistoryRead = "api.chat_history.read"
	OpBriefsRead      = "api.briefs.read"
	OpCronTrigger     = "api.cron.trigger"
	OpReset           = "api.reset"
	OpTelemetryRead   = "api.telemetry.read"
	OpFlagsWrite      = "api.feature_flags.write"
	OpAbsencesTeam    = "api.absences.read_team"
	OpAbsencesReview  = "api.absences.review"
	OpIncidentsRead   = "api.incidents.read"
)

// ErrUnauthorized — запрос без действительного токена.
var ErrUnauthorized = errors.New("требуется авторизация: передайте демо-токен в заголовке Authorization: Bearer <token>")

// ForbiddenError — операция запрещена RBAC.
type ForbiddenError struct {
	Verdict domain.RBACVerdict
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("FORBIDDEN: %s (политика %s)", e.Verdict.Reason, e.Verdict.PolicyID)
}

// Deps — зависимости сервиса.
type Deps struct {
	Spec      *spec.Bundle
	Store     domain.Store
	Cache     domain.Cache
	Directory *domain.Directory
	RBAC      *rbac.Engine
	Gateway   *mcp.Gateway
	Services  *mcp.Services
	Agent     *agent.Engine
	Events    *mcp.Telemetry
	Clock     domain.Clock
	IDs       domain.IDGenerator
	Seed      domain.SeedData
	CronID    string
}

// Service — прикладной сервис Гоффи.
type Service struct {
	Deps
	internID string
}

// New создаёт сервис.
func New(d Deps) *Service {
	return &Service{Deps: d, internID: d.Seed.Intern.ID}
}

const (
	sessionTTL     = 30 * time.Minute
	maxMessageLen  = 2000
	welcomeMessage = "msg-welcome"
)

type session struct {
	LastIntent string `json:"last_intent"`
	Turns      int    `json:"turns"`
}

func (s *Service) resolveInternID(p domain.Principal, explicit ...string) string {
	if len(explicit) > 0 && strings.TrimSpace(explicit[0]) != "" {
		return strings.TrimSpace(explicit[0])
	}
	if p.Role == domain.RoleInternBackend && p.ID != "" {
		return p.ID
	}
	return s.internID
}

func (s *Service) sessionKey(internID string) string {
	if internID == "" {
		internID = s.internID
	}
	return "session:" + internID
}

func (s *Service) loadSession(ctx context.Context, internID string) session {
	var sess session
	if raw, ok, err := s.Cache.Get(ctx, s.sessionKey(internID)); err == nil && ok {
		_ = json.Unmarshal([]byte(raw), &sess)
	}
	return sess
}

func (s *Service) saveSession(ctx context.Context, internID string, sess session) {
	if raw, err := json.Marshal(sess); err == nil {
		_ = s.Cache.Set(ctx, s.sessionKey(internID), string(raw), sessionTTL)
	}
}

// Authenticate находит сотрудника по демо-токену (имитация корпоративной авторизации).
func (s *Service) Authenticate(token string) (domain.Principal, error) {
	e, ok := s.Directory.ByToken(strings.TrimSpace(token))
	if !ok || token == "" {
		return domain.Principal{}, ErrUnauthorized
	}
	return e.Principal(), nil
}

// authorize проверяет операцию API по матрице RBAC и журналирует отказ.
func (s *Service) authorize(ctx context.Context, p domain.Principal, op string) error {
	v := s.RBAC.Check(p, op, rbac.Resource{})
	if v.Allowed {
		return nil
	}
	s.Events.Track(ctx, s.internID, p, domain.EventKindRBAC, "rbac.decision", domain.SeverityWarn, map[string]any{
		"operation": v.Operation, "role": string(v.Role), "allowed": false, "code": v.Code,
		"policy_id": v.PolicyID, "reason": v.Reason,
	})
	return &ForbiddenError{Verdict: v}
}

func (s *Service) can(p domain.Principal, op string) bool {
	return s.RBAC.Check(p, op, rbac.Resource{}).Allowed
}

// seedRecords — исходные записи: seed + приветственное сообщение агента.
func (s *Service) seedRecords() ([]domain.Record, error) {
	now := s.Clock.Now()
	recs, err := s.Seed.Records(now)
	if err != nil {
		return nil, err
	}
	interns := s.Seed.Interns
	if len(interns) == 0 {
		interns = []domain.InternProfile{s.Seed.Intern}
	}
	for _, in := range interns {
		greeting, _ := s.Spec.Agent.IntentByID(agent.IntentGreeting)
		msgID := welcomeMessage
		if in.ID != s.internID {
			msgID = welcomeMessage + "-" + in.ID
		}
		suggestions := append([]string{}, greeting.Suggestions...)
		if in.ID == seed.Intern2ID {
			suggestions = []string{
				"Как устроены ML-пайплайны в команде: DVC, Airflow и версионирование данных?",
				"Как в нашей команде катить ML-модель под фича-флагом и запустить A/B-тест?",
				"Что дальше по чек-листу?",
			}
		}
		welcome := domain.ChatMessage{
			ID: msgID, InternID: in.ID, Role: domain.MessageAssistant,
			Text:   s.Spec.Agent.Response("greeting", map[string]string{"name": in.FirstName(), "team": in.Team}),
			Intent: agent.IntentGreeting, Engine: domain.EngineSmart,
			Sources: []domain.DocRef{}, Suggestions: suggestions,
			ToolCallIDs: []string{}, CreatedAt: now,
		}
		rec, err := domain.NewRecord(domain.KindMessage, welcome.ID, in.ID, now, welcome)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// EnsureSeed загружает синтетические данные при первом старте (пустое хранилище).
func (s *Service) EnsureSeed(ctx context.Context) error {
	_, err := s.Store.Get(ctx, domain.KindIntern, s.internID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return s.resetData(ctx)
}

func (s *Service) resetData(ctx context.Context) error {
	recs, err := s.seedRecords()
	if err != nil {
		return err
	}
	if err := s.Store.Reset(ctx, recs); err != nil {
		return err
	}
	_ = s.Services.Flags.Reset(ctx)
	_ = s.Cache.Del(ctx, s.sessionKey(s.internID))
	for _, in := range s.Seed.Interns {
		_ = s.Cache.Del(ctx, s.sessionKey(in.ID))
	}
	return nil
}

// ViewerInfo — кто смотрит workspace.
type ViewerInfo struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Role      domain.Role `json:"role"`
	RoleTitle string      `json:"role_title"`
	Team      string      `json:"team"`
}

// DocProgress — прогресс изучения документации.
type DocProgress struct {
	Read  []string `json:"read"`
	Total int      `json:"total"`
}

// EngineInfo — режим работы агента.
type EngineInfo struct {
	Mode  string `json:"mode"`
	Model string `json:"model"`
}

// StorageInfo — активные реализации хранилища и кэша.
type StorageInfo struct {
	Store string `json:"store"`
	Cache string `json:"cache"`
}

// WorkspaceView — состояние рабочего пространства для роли смотрящего.
type WorkspaceView struct {
	Viewer        ViewerInfo              `json:"viewer"`
	Intern        domain.InternProfile    `json:"intern"`
	Checklist     []domain.ChecklistItem  `json:"checklist"`
	Progress      domain.Progress         `json:"progress"`
	NextStep      *domain.ChecklistItem   `json:"next_step"`
	SprintTask    domain.SprintTask       `json:"sprint_task"`
	Skills        []domain.AgentSkill     `json:"skills"`
	FeatureFlags  []domain.FeatureFlag    `json:"feature_flags"`
	Metrics       domain.MetricsSummary   `json:"metrics"`
	Messages      []domain.ChatMessage    `json:"messages"`
	Notifications []domain.Notification   `json:"notifications"`
	Briefs        []domain.MentorBrief    `json:"briefs"`
	Absences      []domain.AbsenceRequest `json:"absences"`
	AccessGrants  []domain.AccessGrant    `json:"access_grants"`
	Incidents     []domain.TelemetryEvent `json:"incidents"`
	DocProgress   DocProgress             `json:"doc_progress"`
	Engine        EngineInfo              `json:"engine"`
	Storage       StorageInfo             `json:"storage"`
	Permissions   map[string]bool         `json:"permissions"`
	Disclaimer    string                  `json:"disclaimer"`
	Planner       []domain.PlannerItem    `json:"planner"`
	Reminders     []domain.Reminder       `json:"reminders"`
}

type docRead struct {
	DocID  string    `json:"doc_id"`
	ReadAt time.Time `json:"read_at"`
}

type openQuestion struct {
	Text    string    `json:"text"`
	AskedAt time.Time `json:"asked_at"`
}

func (s *Service) docsRead(ctx context.Context) ([]string, error) {
	return s.docsReadFor(ctx, s.internID)
}

func (s *Service) docsReadFor(ctx context.Context, internID string) ([]string, error) {
	if internID == "" {
		internID = s.internID
	}
	reads, err := domain.Load[docRead](ctx, s.Store, domain.KindDocRead, internID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(reads))
	for _, r := range reads {
		ids = append(ids, r.DocID)
	}
	return ids, nil
}

func (s *Service) openQuestions(ctx context.Context) ([]string, error) {
	qs, err := domain.Load[openQuestion](ctx, s.Store, domain.KindOpenQuestion, s.internID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.Text)
	}
	return out, nil
}

func (s *Service) metrics(ctx context.Context, progress domain.Progress) (domain.MetricsSummary, error) {
	events, err := domain.Load[domain.TelemetryEvent](ctx, s.Store, domain.KindEvent, s.internID)
	if err != nil {
		return domain.MetricsSummary{}, err
	}
	read, err := s.docsRead(ctx)
	if err != nil {
		return domain.MetricsSummary{}, err
	}
	open, err := s.openQuestions(ctx)
	if err != nil {
		return domain.MetricsSummary{}, err
	}
	grants, err := domain.Load[domain.AccessGrant](ctx, s.Store, domain.KindAccessGrant, s.internID)
	if err != nil {
		return domain.MetricsSummary{}, err
	}
	planner, err := s.planner(ctx, s.internID)
	if err != nil {
		return domain.MetricsSummary{}, err
	}
	tasksDone := 0
	for _, it := range planner {
		if it.Status == domain.PlannerDone {
			tasksDone++
		}
	}
	m := s.Spec.Agent.Metrics
	return domain.ComputeMetrics(domain.MetricsInput{
		AccessGrants: len(grants), TasksDone: tasksDone, TasksTotal: len(planner),
		Events: events, MinutesSaved: m.MinutesSaved,
		BaselineHoursMin: m.MentorHoursBaselineMin, BaselineHoursMax: m.MentorHoursBaselineMax,
		Progress: progress, DocsRead: len(read),
		DocsTotal:     s.Services.Playbook.VisibleCount(s.Seed.Intern.Role, s.Seed.Intern.Team),
		OpenQuestions: len(open),
	}), nil
}

// planner возвращает события календаря и задачи стажёра по возрастанию времени.
func (s *Service) planner(ctx context.Context, internID string) ([]domain.PlannerItem, error) {
	items, err := domain.Load[domain.PlannerItem](ctx, s.Store, domain.KindPlanner, internID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].StartsAt.Before(items[j].StartsAt) })
	return items, nil
}

// PrometheusMetrics возвращает агрегированные метрики для экспорта в Prometheus / Grafana.
func (s *Service) PrometheusMetrics(ctx context.Context) (domain.MetricsSummary, float64, error) {
	checklist, err := domain.LoadChecklist(ctx, s.Store, s.internID)
	if err != nil {
		return domain.MetricsSummary{}, 0, err
	}
	progress := domain.ComputeProgress(checklist)
	m, err := s.metrics(ctx, progress)
	if err != nil {
		return domain.MetricsSummary{}, 0, err
	}
	ratio := 0.0
	if progress.Total > 0 {
		ratio = float64(progress.Completed) / float64(progress.Total)
	}
	return m, ratio, nil
}

// Workspace возвращает состояние с учётом прав роли: переписку видит только стажёр,
// брифы — наставник и руководитель, инциденты безопасности — руководитель.
func (s *Service) Workspace(ctx context.Context, p domain.Principal, targetInternID ...string) (WorkspaceView, error) {
	var v WorkspaceView
	if err := s.authorize(ctx, p, OpWorkspaceRead); err != nil {
		return v, err
	}
	internID := s.resolveInternID(p, targetInternID...)
	intern, err := domain.LoadOne[domain.InternProfile](ctx, s.Store, domain.KindIntern, internID)
	if err != nil {
		return v, err
	}
	checklist, err := domain.LoadChecklist(ctx, s.Store, internID)
	if err != nil {
		return v, err
	}
	task, err := domain.LoadOne[domain.SprintTask](ctx, s.Store, domain.KindSprintTask, internID)
	if err != nil {
		return v, err
	}
	progress := domain.ComputeProgress(checklist)
	metrics, err := s.metrics(ctx, progress)
	if err != nil {
		return v, err
	}
	read, err := s.docsReadFor(ctx, internID)
	if err != nil {
		return v, err
	}
	notifications, err := domain.Load[domain.Notification](ctx, s.Store, domain.KindNotification, p.ID)
	if err != nil {
		return v, err
	}

	skills := make([]domain.AgentSkill, 0, len(s.Spec.Agent.Skills))
	for _, sk := range s.Spec.Agent.Skills {
		skills = append(skills, domain.AgentSkill{
			ID: sk.ID, Name: sk.Name, Description: sk.Description, Active: sk.Active,
		})
	}

	mode, model := s.Agent.Mode(ctx)
	v = WorkspaceView{
		Viewer:    ViewerInfo{ID: p.ID, Name: p.Name, Role: p.Role, RoleTitle: s.RBAC.RoleTitle(p.Role), Team: p.Team},
		Intern:    intern,
		Checklist: checklist, Progress: progress, NextStep: domain.NextStep(checklist),
		SprintTask: task, Skills: skills, FeatureFlags: s.Services.Flags.List(ctx), Metrics: metrics,
		Messages: []domain.ChatMessage{}, Notifications: reverse(notifications),
		Briefs: []domain.MentorBrief{}, Absences: []domain.AbsenceRequest{},
		AccessGrants: []domain.AccessGrant{}, Incidents: []domain.TelemetryEvent{},
		DocProgress: DocProgress{Read: read, Total: metrics.DocsTotal},
		Engine:      EngineInfo{Mode: mode, Model: model},
		Storage:     StorageInfo{Store: s.Store.Name(), Cache: s.Cache.Name()},
		Permissions: map[string]bool{
			"chat": s.can(p, OpChatSend), "cron": s.can(p, OpCronTrigger), "reset": s.can(p, OpReset),
			"telemetry": s.can(p, OpTelemetryRead), "flags_write": s.can(p, OpFlagsWrite),
			"briefs": s.can(p, OpBriefsRead), "incidents": s.can(p, OpIncidentsRead),
			"planner_add": s.RBAC.Can(p.Role, mcp.ToolAddTask), "planner_complete": s.RBAC.Can(p.Role, mcp.ToolCompleteItem),
			"absence_approve": s.RBAC.Can(p.Role, mcp.ToolReviewDayOff),
		},
		Disclaimer: s.Spec.Agent.Agent.SyntheticData.Disclaimer,
	}

	isOwner := p.ID == internID
	if v.Planner, err = s.planner(ctx, internID); err != nil {
		return v, err
	}
	v.Reminders = []domain.Reminder{}
	if isOwner {
		if r := domain.BuildReminders(v.Planner, notifications, s.Clock.Now()); r != nil {
			v.Reminders = r
		}
	}
	if s.can(p, OpChatHistoryRead) && isOwner {
		if v.Messages, err = domain.Load[domain.ChatMessage](ctx, s.Store, domain.KindMessage, internID); err != nil {
			return v, err
		}
	}
	if isOwner || s.can(p, OpAbsencesTeam) {
		absences, err := domain.Load[domain.AbsenceRequest](ctx, s.Store, domain.KindAbsence, internID)
		if err != nil {
			return v, err
		}
		grants, err := domain.Load[domain.AccessGrant](ctx, s.Store, domain.KindAccessGrant, internID)
		if err != nil {
			return v, err
		}
		v.Absences, v.AccessGrants = reverse(absences), grants
	}
	if s.can(p, OpBriefsRead) {
		briefs, err := domain.Load[domain.MentorBrief](ctx, s.Store, domain.KindBrief, internID)
		if err != nil {
			return v, err
		}
		v.Briefs = reverse(briefs)
	}
	if s.can(p, OpIncidentsRead) {
		events, err := domain.Load[domain.TelemetryEvent](ctx, s.Store, domain.KindEvent, internID)
		if err != nil {
			return v, err
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Name == "rbac.incident" {
				v.Incidents = append(v.Incidents, events[i])
			}
		}
	}
	return v, nil
}

// ReviewAbsence согласовывает или отклоняет заявку на отгул руководителем.
func (s *Service) ReviewAbsence(ctx context.Context, p domain.Principal, id, decision, comment string) (*domain.AbsenceRequest, error) {
	if err := s.authorize(ctx, p, OpAbsencesReview); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	decision = strings.TrimSpace(decision)
	comment = strings.TrimSpace(comment)
	if id == "" {
		return nil, fmt.Errorf("%w: не указан id заявки", domain.ErrValidation)
	}
	if decision != domain.AbsenceStatusApproved && decision != domain.AbsenceStatusRejected {
		return nil, fmt.Errorf("%w: некорректное решение %q (ожидается approved или rejected)", domain.ErrValidation, decision)
	}

	all, err := domain.Load[domain.AbsenceRequest](ctx, s.Store, domain.KindAbsence, "")
	if err != nil {
		return nil, err
	}
	var target *domain.AbsenceRequest
	for i := range all {
		if all[i].ID == id || all[i].Number == id {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return nil, domain.ErrNotFound
	}

	now := s.Clock.Now()
	target.Status = decision
	target.Decision = decision
	target.Comment = comment
	target.ReviewerID = p.ID
	target.ReviewedAt = &now

	if err := domain.Save(ctx, s.Store, domain.KindAbsence, target.ID, target.InternID, target.CreatedAt, *target); err != nil {
		return nil, err
	}

	// 1. Clickstream-событие absence.reviewed
	s.Events.Track(ctx, target.InternID, p, domain.EventKindClickstream, "absence.reviewed", domain.SeverityInfo, map[string]any{
		"id":          target.ID,
		"number":      target.Number,
		"decision":    decision,
		"comment":     comment,
		"reviewer_id": p.ID,
	})

	// 2. Системное уведомление от Гоффи в чат стажёра
	var notifyText string
	if decision == domain.AbsenceStatusApproved {
		notifyText = fmt.Sprintf("✅ Руководитель согласовал твою заявку на отгул %s. Статус в системе «Отсутствия» обновлён.", target.Number)
	} else {
		notifyText = fmt.Sprintf("❌ Руководитель отклонил твою заявку на отгул %s. Статус в системе «Отсутствия» обновлён.", target.Number)
		if comment != "" {
			notifyText += fmt.Sprintf("\nКомментарий: %s", comment)
		}
	}

	msg := domain.ChatMessage{
		ID:        s.IDs.NewID("msg"),
		InternID:  target.InternID,
		Role:      domain.MessageAssistant,
		Text:      notifyText,
		Intent:    "absence_reviewed",
		Engine:    domain.EngineSmart,
		Sources:   []domain.DocRef{},
		CreatedAt: now,
	}
	_ = s.saveMessage(ctx, msg)

	// Уведомление во входящие
	kind, body := mcp.ReviewNotice(*target)
	_, _ = s.Services.Notifications.Send(ctx, target.InternID, "Решение по отгулу", body, kind, now)

	return target, nil
}

func reverse[T any](in []T) []T {
	out := make([]T, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}

// ChatResult — результат обработки сообщения или подтверждённого действия.
type ChatResult struct {
	UserMessage      *domain.ChatMessage    `json:"user_message"`
	Message          *domain.ChatMessage    `json:"message"`
	ToolCalls        []domain.MCPToolCall   `json:"tool_calls"`
	RBAC             *domain.RBACVerdict    `json:"rbac"`
	Checklist        []domain.ChecklistItem `json:"checklist"`
	ChecklistUpdates []domain.ChecklistItem `json:"checklist_updates"`
	Progress         domain.Progress        `json:"progress"`
	NextStep         *domain.ChecklistItem  `json:"next_step"`
	LatencyMS        float64                `json:"latency_ms"`
}

func (s *Service) saveMessage(ctx context.Context, m domain.ChatMessage) error {
	return domain.Save(ctx, s.Store, domain.KindMessage, m.ID, m.InternID, m.CreatedAt, m)
}

func callIDs(calls []domain.MCPToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, c := range calls {
		ids = append(ids, c.ID)
	}
	return ids
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// Chat обрабатывает сообщение стажёра: агент → MCP/RBAC → эффекты на чек-лист → ответ.
func (s *Service) Chat(ctx context.Context, p domain.Principal, text string, emit func(agent.Stage), targetInternID ...string) (ChatResult, error) {
	started := time.Now()
	var res ChatResult
	if err := s.authorize(ctx, p, OpChatSend); err != nil {
		return res, err
	}
	text = strings.TrimSpace(text)
	if n := len([]rune(text)); n == 0 || n > maxMessageLen {
		return res, fmt.Errorf("%w: сообщение должно быть от 1 до %d символов", domain.ErrValidation, maxMessageLen)
	}
	internID := s.resolveInternID(p, targetInternID...)
	intern, err := domain.LoadOne[domain.InternProfile](ctx, s.Store, domain.KindIntern, internID)
	if err != nil {
		return res, err
	}
	checklist, err := domain.LoadChecklist(ctx, s.Store, internID)
	if err != nil {
		return res, err
	}
	task, err := domain.LoadOne[domain.SprintTask](ctx, s.Store, domain.KindSprintTask, internID)
	if err != nil {
		return res, err
	}
	sess := s.loadSession(ctx, internID)

	user := domain.ChatMessage{
		ID: s.IDs.NewID("msg"), InternID: internID, Role: domain.MessageUser, Text: text,
		Sources: []domain.DocRef{}, Suggestions: []string{}, ToolCallIDs: []string{}, CreatedAt: s.Clock.Now(),
	}
	if err := s.saveMessage(ctx, user); err != nil {
		return res, err
	}
	s.Events.Track(ctx, internID, p, domain.EventKindClickstream, "chat.message_sent", domain.SeverityInfo,
		map[string]any{"length": len([]rune(text))})

	reply := s.Agent.Handle(ctx, agent.Request{
		Principal: p, Intern: intern, Text: text, Checklist: checklist, Task: task,
		LastIntent: sess.LastIntent, Emit: emit,
	})

	updates, err := s.applyEffects(ctx, p, reply.ToolCalls, reply.DocsRead, internID)
	if err != nil {
		return res, err
	}
	if reply.OpenQuestion != "" {
		q := openQuestion{Text: reply.OpenQuestion, AskedAt: s.Clock.Now()}
		if err := domain.Save(ctx, s.Store, domain.KindOpenQuestion, s.IDs.NewID("q"), internID, q.AskedAt, q); err != nil {
			return res, err
		}
	}
	checklist, err = domain.LoadChecklist(ctx, s.Store, internID)
	if err != nil {
		return res, err
	}

	assistant := domain.ChatMessage{
		ID: s.IDs.NewID("msg"), InternID: internID, Role: domain.MessageAssistant,
		Text:   reply.Text + checklistNote(updates, checklist),
		Intent: reply.Intent, Engine: reply.Engine, Sources: nonNil(reply.Sources),
		ActionCard: reply.ActionCard, ResultCard: reply.ResultCard, Suggestions: nonNil(reply.Suggestions),
		ToolCallIDs: callIDs(reply.ToolCalls), CreatedAt: s.Clock.Now(),
	}
	if err := s.saveMessage(ctx, assistant); err != nil {
		return res, err
	}
	s.saveSession(ctx, internID, session{LastIntent: reply.Intent, Turns: sess.Turns + 1})

	latency := float64(time.Since(started).Microseconds()) / 1000
	payload := map[string]any{
		"intent": reply.Intent, "scenario": reply.Scenario, "engine": reply.Engine,
		"answered": reply.Answered, "latency_ms": latency, "tool_calls": len(reply.ToolCalls),
	}
	if reply.SavingKey != "" {
		payload["saving_key"] = reply.SavingKey
	}
	s.Events.Track(ctx, internID, p, domain.EventKindAgent, "agent.answer", domain.SeverityInfo, payload)
	if reply.LLMFallback != "" {
		s.Events.Track(ctx, internID, p, domain.EventKindAgent, "llm.fallback", domain.SeverityWarn,
			map[string]any{"reason": reply.LLMFallback, "fallback": "smart_engine"})
	}

	return ChatResult{
		UserMessage: &user, Message: &assistant, ToolCalls: nonNil(reply.ToolCalls), RBAC: reply.Verdict,
		Checklist: checklist, ChecklistUpdates: nonNil(updates), Progress: domain.ComputeProgress(checklist),
		NextStep: domain.NextStep(checklist), LatencyMS: latency,
	}, nil
}

func checklistNote(updates, checklist []domain.ChecklistItem) string {
	if len(updates) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n")
	for _, u := range updates {
		state := "выполнен"
		if u.Status == domain.StatusInProgress {
			state = "в работе"
		}
		fmt.Fprintf(&b, "Шаг чек-листа «%s» — %s. ", u.Title, state)
	}
	if next := domain.NextStep(checklist); next != nil {
		fmt.Fprintf(&b, "Следующий шаг: «%s».", next.Title)
	} else {
		b.WriteString("Все шаги чек-листа выполнены!")
	}
	return strings.TrimRight(b.String(), " ")
}

func evidence(call domain.MCPToolCall) string {
	switch call.Tool {
	case mcp.ToolRequestRepoAccess:
		return fmt.Sprintf("Доступ к %v в Stash выдан по заявке (Гоффи)", call.Result["repository"])
	case mcp.ToolCreateDayOff:
		return fmt.Sprintf("Заявка %v оформлена через Гоффи", call.Result["request_number"])
	case mcp.ToolSendBrief:
		return "Бриф к 1:1 отправлен наставнику"
	}
	return "Выполнено через Гоффи"
}

// applyEffects применяет правила checklist_effects из спецификации агента.
func (s *Service) applyEffects(ctx context.Context, p domain.Principal, calls []domain.MCPToolCall, docs []string, targetInternID ...string) ([]domain.ChecklistItem, error) {
	internID := s.resolveInternID(p, targetInternID...)
	now := s.Clock.Now()
	for _, id := range docs {
		if err := domain.Save(ctx, s.Store, domain.KindDocRead, internID+":"+id, internID, now, docRead{DocID: id, ReadAt: now}); err != nil {
			return nil, err
		}
	}
	read := map[string]bool{}
	if len(docs) > 0 {
		ids, err := s.docsReadFor(ctx, internID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			read[id] = true
		}
	}

	var updates []domain.ChecklistItem
	for _, eff := range s.Spec.Agent.ChecklistEffects {
		status, proof := domain.ChecklistStatus(""), ""
		switch {
		case eff.Tool != "":
			for _, c := range calls {
				if c.Tool == eff.Tool && c.OK() {
					status, proof = domain.ChecklistStatus(eff.Status), evidence(c)
				}
			}
		case len(eff.DocsRead) > 0 && len(docs) > 0:
			done := 0
			for _, id := range eff.DocsRead {
				if read[id] {
					done++
				}
			}
			switch {
			case done == len(eff.DocsRead):
				status, proof = domain.ChecklistStatus(eff.Status), "Инструкции изучены через Гоффи"
			case done > 0:
				status, proof = domain.StatusInProgress, fmt.Sprintf("Изучено инструкций: %d из %d", done, len(eff.DocsRead))
			}
		}
		if status == "" {
			continue
		}
		item, err := domain.LoadOne[domain.ChecklistItem](ctx, s.Store, domain.KindChecklist, domain.ChecklistRecordID(internID, eff.Item))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		next, changed, err := item.Advance(status, proof, now)
		if err != nil || !changed {
			continue // шаг уже в этом или более позднем статусе
		}
		if err := domain.SaveChecklistItem(ctx, s.Store, internID, next); err != nil {
			return nil, err
		}
		updates = append(updates, next)
		s.Events.Track(ctx, internID, p, domain.EventKindClickstream, "checklist.updated", domain.SeverityInfo,
			map[string]any{"item": next.ID, "status": string(next.Status), "evidence": next.Evidence})
	}
	return updates, nil
}
