// Package domain содержит модели и чистые бизнес-правила Гоффи.
// Пакет не зависит от инфраструктуры: хранилища и кэш подключаются через порты (ports.go).
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SyntheticDisclaimer — дисклеймер по умолчанию для синтетических инструкций.
const SyntheticDisclaimer = "Тестовая инструкция (Синтетические данные)"

// Role — роль пользователя в ролевой модели.
type Role string

// Роли из specs/03_rbac_matrix.openspec.yaml.
const (
	RoleInternBackend Role = "intern_backend"
	RoleMentor        Role = "mentor"
	RoleTeamLead      Role = "team_lead"
	RoleSystemCron    Role = "system_cron"
)

// Principal — аутентифицированный субъект запроса.
type Principal struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role Role   `json:"role"`
	Team string `json:"team"`
}

// Employee — запись имитации корпоративного справочника сотрудников.
type Employee struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        Role   `json:"role"`
	RoleTitle   string `json:"role_title"`
	Team        string `json:"team"`
	Token       string `json:"-"`
	IsSynthetic bool   `json:"is_synthetic"`
}

// Principal возвращает субъект для проверки прав.
func (e Employee) Principal() Principal {
	return Principal{ID: e.ID, Name: e.Name, Role: e.Role, Team: e.Team}
}

// InternProfile — профиль стажёра и контекст для агента.
type InternProfile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         Role   `json:"role"`
	Team         string `json:"team"`
	Stack        string `json:"stack"`
	Stage        string `json:"stage"`
	Program      string `json:"program"`
	StartDate    string `json:"start_date"`
	Repository   string `json:"repository"`
	MentorID     string `json:"mentor_id"`
	MentorName   string `json:"mentor_name"`
	TeamLeadID   string `json:"team_lead_id"`
	TeamLeadName string `json:"team_lead_name"`
	IsSynthetic  bool   `json:"is_synthetic"`
}

// FirstName возвращает имя для обращения.
func (p InternProfile) FirstName() string {
	if parts := strings.Fields(p.Name); len(parts) > 0 {
		return parts[0]
	}
	return "коллега"
}

// ContextLine — краткая строка контекста стажёра для ответа и промпта.
func (p InternProfile) ContextLine() string {
	return fmt.Sprintf("команда %s · стек %s · этап %s", p.Team, p.Stack, p.Stage)
}

// ChecklistStatus — статус шага чек-листа адаптации.
type ChecklistStatus string

// Статусы шага чек-листа.
const (
	StatusPending    ChecklistStatus = "pending"
	StatusInProgress ChecklistStatus = "in_progress"
	StatusCompleted  ChecklistStatus = "completed"
)

// Valid сообщает, известен ли статус.
func (s ChecklistStatus) Valid() bool {
	return s == StatusPending || s == StatusInProgress || s == StatusCompleted
}

func (s ChecklistStatus) rank() int {
	switch s {
	case StatusInProgress:
		return 1
	case StatusCompleted:
		return 2
	default:
		return 0
	}
}

// ChecklistItem — шаг чек-листа адаптации стажёра.
type ChecklistItem struct {
	ID          string          `json:"id"`
	Position    int             `json:"position"`
	Week        int             `json:"week"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      ChecklistStatus `json:"status"`
	Scenario    string          `json:"scenario,omitempty"`
	Evidence    string          `json:"evidence,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// ErrInvalidTransition — попытка недопустимого перехода статуса.
var ErrInvalidTransition = errors.New("недопустимый переход статуса чек-листа")

// Advance переводит шаг в новый статус. Статус может только расти
// (pending → in_progress → completed); повтор текущего статуса — no-op.
func (c ChecklistItem) Advance(to ChecklistStatus, evidence string, now time.Time) (ChecklistItem, bool, error) {
	if !to.Valid() {
		return c, false, fmt.Errorf("%w: неизвестный статус %q", ErrInvalidTransition, to)
	}
	if to.rank() < c.Status.rank() {
		return c, false, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, c.Status, to)
	}
	if to == c.Status {
		return c, false, nil
	}
	c.Status = to
	c.UpdatedAt = now
	if evidence != "" {
		c.Evidence = evidence
	}
	if to == StatusCompleted {
		t := now
		c.CompletedAt = &t
	}
	return c, true, nil
}

// Progress — агрегированный прогресс чек-листа.
type Progress struct {
	Completed  int `json:"completed"`
	InProgress int `json:"in_progress"`
	Total      int `json:"total"`
	Percent    int `json:"percent"`
}

// ComputeProgress считает прогресс по шагам чек-листа.
func ComputeProgress(items []ChecklistItem) Progress {
	p := Progress{Total: len(items)}
	for _, it := range items {
		switch it.Status {
		case StatusCompleted:
			p.Completed++
		case StatusInProgress:
			p.InProgress++
		}
	}
	if p.Total > 0 {
		p.Percent = p.Completed * 100 / p.Total
	}
	return p
}

// NextStep возвращает первый невыполненный шаг (по позиции) или nil.
func NextStep(items []ChecklistItem) *ChecklistItem {
	var next *ChecklistItem
	for i := range items {
		if items[i].Status == StatusCompleted {
			continue
		}
		if next == nil || items[i].Position < next.Position {
			next = &items[i]
		}
	}
	return next
}

// MessageRole — автор сообщения чата.
type MessageRole string

// Роли сообщений.
const (
	MessageUser      MessageRole = "user"
	MessageAssistant MessageRole = "assistant"
)

// DocRef — ссылка на документ базы знаний в ответе агента.
type DocRef struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Steps       []string `json:"steps"`
	Source      string   `json:"source"`
	IsSynthetic bool     `json:"is_synthetic"`
	Disclaimer  string   `json:"disclaimer"`
}

// KnowledgeDoc — синтетическая инструкция базы знаний.
type KnowledgeDoc struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Steps       []string `json:"steps"`
	Topics      []string `json:"topics"`
	Keywords    []string `json:"keywords"`
	Team        string   `json:"team"`
	Audience    []Role   `json:"audience"`
	Source      string   `json:"source"`
	IsSynthetic bool     `json:"is_synthetic"`
	Disclaimer  string   `json:"disclaimer"`
}

// VisibleTo сообщает, доступен ли документ роли и команде.
func (d KnowledgeDoc) VisibleTo(role Role, team string) bool {
	if d.Team != "" && team != "" && !strings.EqualFold(d.Team, team) {
		return false
	}
	if len(d.Audience) == 0 {
		return true
	}
	for _, r := range d.Audience {
		if r == role {
			return true
		}
	}
	return false
}

// Ref превращает документ в ссылку для ответа.
func (d KnowledgeDoc) Ref() DocRef {
	return DocRef{
		ID: d.ID, Title: d.Title, Summary: d.Summary, Steps: append([]string{}, d.Steps...),
		Source: d.Source, IsSynthetic: d.IsSynthetic, Disclaimer: d.Disclaimer,
	}
}

// Типы карточек.
const (
	ActionAbsenceForm    = "absence_form"
	CardAccessGranted    = "access_granted"
	CardRBACDenied       = "rbac_denied"
	CardAbsenceCreated   = "absence_created"
	CardBriefSent        = "brief_sent"
	ActionStatusPending  = "pending"
	ActionStatusDone     = "submitted"
	CardStatusSuccess    = "success"
	CardStatusDenied     = "denied"
	CardStatusInfo       = "info"
	EngineSmart          = "smart"
	EngineLLM            = "llm"
	EngineCron           = "cron"
	SeverityInfo         = "info"
	SeverityWarn         = "warn"
	SeveritySecurity     = "security"
	AbsenceStatusPending  = "pending_approval"
	AbsenceStatusApproved = "approved"
	AbsenceStatusRejected = "rejected"
)

// ActionCard — интерактивная карточка, требующая подтверждения пользователя.
type ActionCard struct {
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Fields      map[string]any `json:"fields"`
	SubmitLabel string         `json:"submit_label"`
	Status      string         `json:"status"`
}

// CardLine — строка «метка: значение» карточки результата.
type CardLine struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ResultCard — карточка результата действия.
type ResultCard struct {
	Type     string     `json:"type"`
	Status   string     `json:"status"`
	Title    string     `json:"title"`
	Lines    []CardLine `json:"lines"`
	Footnote string     `json:"footnote,omitempty"`
}

// ChatMessage — сообщение чата стажёра с Гоффи.
type ChatMessage struct {
	ID          string      `json:"id"`
	InternID    string      `json:"intern_id"`
	Role        MessageRole `json:"role"`
	Text        string      `json:"text"`
	Intent      string      `json:"intent,omitempty"`
	Engine      string      `json:"engine,omitempty"`
	Sources     []DocRef    `json:"sources"`
	ActionCard  *ActionCard `json:"action_card"`
	ResultCard  *ResultCard `json:"result_card"`
	Suggestions []string    `json:"suggestions"`
	Proactive   bool        `json:"proactive"`
	ToolCallIDs []string    `json:"tool_call_ids"`
	CreatedAt   time.Time   `json:"created_at"`
}

// RBACVerdict — решение RBAC-движка по операции.
type RBACVerdict struct {
	Allowed         bool   `json:"allowed"`
	Code            string `json:"code"`
	HTTPStatus      int    `json:"http_status"`
	Role            Role   `json:"role"`
	Operation       string `json:"operation"`
	PolicyID        string `json:"policy_id"`
	Reason          string `json:"reason"`
	Constraint      string `json:"constraint,omitempty"`
	SafeAlternative string `json:"safe_alternative,omitempty"`
	Incident        bool   `json:"incident"`
}

// Коды вердикта RBAC.
const (
	VerdictAllowed   = "ALLOWED"
	VerdictForbidden = "FORBIDDEN"
)

// ToolStatus — итог вызова MCP-инструмента.
type ToolStatus string

// Статусы вызова инструмента.
const (
	ToolSuccess   ToolStatus = "success"
	ToolForbidden ToolStatus = "forbidden"
	ToolInvalid   ToolStatus = "invalid"
	ToolError     ToolStatus = "error"
)

// MCPToolCall — запись о вызове MCP-инструмента (аудит).
type MCPToolCall struct {
	ID         string         `json:"id"`
	InternID   string         `json:"intern_id"`
	ActorID    string         `json:"actor_id"`
	ActorRole  Role           `json:"actor_role"`
	Server     string         `json:"server"`
	Tool       string         `json:"tool"`
	Arguments  map[string]any `json:"arguments"`
	Result     map[string]any `json:"result"`
	Status     ToolStatus     `json:"status"`
	ErrorCode  string         `json:"error_code,omitempty"`
	Error      string         `json:"error,omitempty"`
	RBAC       RBACVerdict    `json:"rbac"`
	DurationMS float64        `json:"duration_ms"`
	CreatedAt  time.Time      `json:"created_at"`
}

// OK сообщает, что вызов завершился успешно.
func (c MCPToolCall) OK() bool { return c.Status == ToolSuccess }

// AbsenceRequest — заявка на отгул в имитации сервиса «Отсутствия».
type AbsenceRequest struct {
	ID          string     `json:"id"`
	Number      string     `json:"number"`
	InternID    string     `json:"intern_id"`
	InternName  string     `json:"intern_name"`
	Date        string     `json:"date"`
	TimeFrom    string     `json:"time_from"`
	TimeTo      string     `json:"time_to"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	Notified    []string   `json:"notified"`
	IsSynthetic bool       `json:"is_synthetic"`
	Decision    string     `json:"decision,omitempty"`
	ReviewerID  string     `json:"reviewer_id,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	Comment     string     `json:"comment,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

var (
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// ErrValidation — ошибка валидации доменной сущности.
var ErrValidation = errors.New("ошибка валидации")

// Validate проверяет заявку: формат даты и времени, дата не в прошлом, интервал корректен.
func (a AbsenceRequest) Validate(now time.Time) error {
	if strings.TrimSpace(a.InternID) == "" {
		return fmt.Errorf("%w: не указан сотрудник", ErrValidation)
	}
	if !dateRe.MatchString(a.Date) {
		return fmt.Errorf("%w: дата должна быть в формате ГГГГ-ММ-ДД", ErrValidation)
	}
	day, err := time.ParseInLocation("2006-01-02", a.Date, now.Location())
	if err != nil {
		return fmt.Errorf("%w: некорректная дата %q", ErrValidation, a.Date)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if day.Before(today) {
		return fmt.Errorf("%w: дата отгула уже прошла", ErrValidation)
	}
	if (a.TimeFrom == "") != (a.TimeTo == "") {
		return fmt.Errorf("%w: укажите и начало, и окончание отгула", ErrValidation)
	}
	if a.TimeFrom != "" {
		if !timeRe.MatchString(a.TimeFrom) || !timeRe.MatchString(a.TimeTo) {
			return fmt.Errorf("%w: время должно быть в формате ЧЧ:ММ", ErrValidation)
		}
		if a.TimeTo <= a.TimeFrom {
			return fmt.Errorf("%w: окончание должно быть позже начала", ErrValidation)
		}
	}
	if n := len([]rune(strings.TrimSpace(a.Reason))); n < 3 || n > 300 {
		return fmt.Errorf("%w: причина должна быть от 3 до 300 символов", ErrValidation)
	}
	return nil
}

// AbsenceNumberBase — номер первой заявки в демо-данных (#ABS-2026-84).
const AbsenceNumberBase = 84

// AbsenceNumber формирует номер заявки: #ABS-<год>-<порядковый номер>.
func AbsenceNumber(year, existing int) string {
	return fmt.Sprintf("#ABS-%d-%d", year, AbsenceNumberBase+existing)
}

// AccessGrant — выданный доступ к репозиторию и стенду (имитация IAM).
type AccessGrant struct {
	ID          string    `json:"id"`
	InternID    string    `json:"intern_id"`
	Repository  string    `json:"repository"`
	AccessLevel string    `json:"access_level"`
	Stand       string    `json:"stand"`
	TokenMasked string    `json:"token_masked"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsSynthetic bool      `json:"is_synthetic"`
	CreatedAt   time.Time `json:"created_at"`
}

// MaskToken маскирует токен, оставляя префикс и 4 последних символа.
func MaskToken(token string) string {
	const keep = 4
	r := []rune(token)
	if len(r) <= keep {
		return strings.Repeat("•", len(r))
	}
	prefix := ""
	if i := strings.LastIndex(token, "-"); i > 0 && i < len(token)-keep {
		prefix = token[:i+1]
	}
	return prefix + "••••" + string(r[len(r)-keep:])
}

// Notification — уведомление сотруднику.
type Notification struct {
	ID          string    `json:"id"`
	RecipientID string    `json:"recipient_id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Kind        string    `json:"kind"`
	CreatedAt   time.Time `json:"created_at"`
}

// MentorBrief — Structured 1:1 Brief для наставника.
type MentorBrief struct {
	ID                   string    `json:"id"`
	InternID             string    `json:"intern_id"`
	InternName           string    `json:"intern_name"`
	MentorID             string    `json:"mentor_id"`
	TaskKey              string    `json:"task_key"`
	LearnedTopics        []string  `json:"learned_topics"`
	ArchitectureQuestion string    `json:"architecture_question"`
	Signals              []string  `json:"signals"`
	OpenQuestions        []string  `json:"open_questions"`
	CreatedAt            time.Time `json:"created_at"`
}

// SprintTask — задача стажёра в спринте (имитация трекера).
type SprintTask struct {
	Key                  string `json:"key"`
	InternID             string `json:"intern_id"`
	Title                string `json:"title"`
	Summary              string `json:"summary"`
	Status               string `json:"status"`
	Commits              int    `json:"commits"`
	DaysInProgress       int    `json:"days_in_progress"`
	Repository           string `json:"repository"`
	ArchitectureQuestion string `json:"architecture_question"`
}

// StalledAfterDays — порог (в днях) для срабатывания ИИ-крона.
const StalledAfterDays = 2

// Stalled сообщает, что задача висит в работе без коммитов.
func (t SprintTask) Stalled() bool {
	return t.Status == string(StatusInProgress) && t.Commits == 0 && t.DaysInProgress >= StalledAfterDays
}

// TelemetryEvent — событие телеметрии (имитация Clickstream + аудит MCP/RBAC).
type TelemetryEvent struct {
	ID        string         `json:"id"`
	InternID  string         `json:"intern_id"`
	ActorID   string         `json:"actor_id"`
	ActorRole Role           `json:"actor_role"`
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Severity  string         `json:"severity"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// Виды событий телеметрии.
const (
	EventKindMCP         = "mcp"
	EventKindRBAC        = "rbac"
	EventKindClickstream = "clickstream"
	EventKindAgent       = "agent"
	EventKindCron        = "cron"
)

// AgentSkill — подключённый модульный скилл агента.
type AgentSkill struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Active      bool   `json:"active" yaml:"active"`
}

// FeatureFlag — флаг функциональности.
type FeatureFlag struct {
	Key         string `json:"key"`
	Enabled     bool   `json:"enabled"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// Виды флагов.
const (
	FlagRuntime     = "runtime"
	FlagCompileTime = "compile_time"
)

// Ключи экономии времени (specs/01_agent_behavior: metrics.minutes_saved).
const (
	SavingQuestion = "question_answered"
	SavingAccess   = "access_granted"
	SavingAbsence  = "absence_created"
	SavingBrief    = "brief_prepared"
	SavingBlocked  = "rbac_blocked"
)

// MetricsSummary — метрики пользы Гоффи.
type MetricsSummary struct {
	QuestionsAnswered      int     `json:"questions_answered"`
	ActionsAutomated       int     `json:"actions_automated"`
	RBACBlocks             int     `json:"rbac_blocks"`
	BriefsPrepared         int     `json:"briefs_prepared"`
	UnansweredQuestions    int     `json:"unanswered_questions"`
	ToolCalls              int     `json:"tool_calls"`
	DocsRead               int     `json:"docs_read"`
	DocsTotal              int     `json:"docs_total"`
	ChecklistPercent       int     `json:"checklist_percent"`
	MentorMinutesSaved     int     `json:"mentor_minutes_saved"`
	MentorHoursBaselineMin int     `json:"mentor_hours_baseline_min"`
	MentorHoursBaselineMax int     `json:"mentor_hours_baseline_max"`
	SavingPercent          float64 `json:"saving_percent"`
	AvgResponseMS          float64 `json:"avg_response_ms"`
	// DeflectionRate — «показатель самостоятельности»: % вопросов, закрытых без наставника.
	DeflectionRate int `json:"deflection_rate"`
	AccessGrants   int `json:"access_grants"`
	TasksDone      int `json:"tasks_done"`
	TasksTotal     int `json:"tasks_total"`
}

// MetricsInput — исходные данные для расчёта метрик.
type MetricsInput struct {
	Events           []TelemetryEvent
	MinutesSaved     map[string]int
	BaselineHoursMin int
	BaselineHoursMax int
	Progress         Progress
	DocsRead         int
	DocsTotal        int
	OpenQuestions    int
	AccessGrants     int
	TasksDone        int
	TasksTotal       int
}

// ComputeMetrics считает метрики по событиям телеметрии.
// Событие учитывается в экономии, если в payload есть saving_key из спецификации.
func ComputeMetrics(in MetricsInput) MetricsSummary {
	m := MetricsSummary{
		DocsRead: in.DocsRead, DocsTotal: in.DocsTotal,
		ChecklistPercent:       in.Progress.Percent,
		MentorHoursBaselineMin: in.BaselineHoursMin,
		MentorHoursBaselineMax: in.BaselineHoursMax,
		UnansweredQuestions:    in.OpenQuestions,
		AccessGrants:           in.AccessGrants,
		TasksDone:              in.TasksDone,
		TasksTotal:             in.TasksTotal,
	}
	var latencySum float64
	var latencyN int
	for _, e := range in.Events {
		if e.Kind == EventKindMCP {
			m.ToolCalls++
		}
		if v, ok := e.Payload["latency_ms"].(float64); ok && e.Kind == EventKindAgent {
			latencySum += v
			latencyN++
		}
		key, _ := e.Payload["saving_key"].(string)
		if key == "" {
			continue
		}
		m.MentorMinutesSaved += in.MinutesSaved[key]
		switch key {
		case SavingQuestion:
			m.QuestionsAnswered++
		case SavingAccess, SavingAbsence:
			m.ActionsAutomated++
		case SavingBrief:
			m.BriefsPrepared++
		case SavingBlocked:
			m.RBACBlocks++
		}
	}
	if asked := m.QuestionsAnswered + m.UnansweredQuestions; asked > 0 {
		m.DeflectionRate = m.QuestionsAnswered * 100 / asked
	}
	if latencyN > 0 {
		m.AvgResponseMS = round1(latencySum / float64(latencyN))
	}
	if avg := float64(in.BaselineHoursMin+in.BaselineHoursMax) / 2 * 60; avg > 0 {
		m.SavingPercent = round1(float64(m.MentorMinutesSaved) / avg * 100)
	}
	return m
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
