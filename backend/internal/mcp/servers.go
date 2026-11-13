package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/rbac"
)

// Имена инструментов (должны совпадать со specs/02_mcp_tools.openspec.yaml).
const (
	ToolRequestRepoAccess = "iam_access.request_repo_access"
	ToolExportProdPII     = "db_admin.export_prod_pii"
	ToolSearchDocs        = "playbook.search_docs"
	ToolCreateDayOff      = "absences.create_day_off"
	ToolReviewDayOff      = "absences.review_day_off"
	ToolSendBrief         = "mentor.send_1on1_brief"
	ToolSendNotification  = "notifications.send"
	ToolTaskActivity      = "tracker.get_task_activity"
	ToolListFlags         = "feature_flags.list"
)

// Deps — зависимости имитаций корпоративных сервисов.
type Deps struct {
	Store domain.Store
	Cache domain.Cache
	Clock domain.Clock
	IDs   domain.IDGenerator
	Seed  domain.SeedData
}

// Services — имитации корпоративных сервисов, доступные и как MCP-инструменты, и напрямую.
type Services struct {
	IAM           *IAM
	DBAdmin       *DBAdmin
	Playbook      *Playbook
	Absences      *Absences
	Mentor        *Mentor
	Notifications *Notifications
	Tracker       *Tracker
	Flags         *Flags
}

// NewServices создаёт имитации сервисов.
func NewServices(d Deps) *Services {
	notifications := &Notifications{store: d.Store, ids: d.IDs}
	return &Services{
		IAM:           &IAM{store: d.Store, ids: d.IDs, repos: d.Seed.Repositories},
		DBAdmin:       &DBAdmin{},
		Playbook:      &Playbook{docs: d.Seed.Docs},
		Absences:      &Absences{store: d.Store, ids: d.IDs, notifications: notifications},
		Mentor:        &Mentor{store: d.Store, ids: d.IDs, notifications: notifications},
		Notifications: notifications,
		Tracker:       &Tracker{store: d.Store},
		Flags:         NewFlags(d.Cache, d.Seed.Flags),
	}
}

// BindAll привязывает обработчики всех сервисов к реестру.
func (s *Services) BindAll(r *Registry) error {
	bindings := []struct {
		name    string
		handler Handler
		attrs   AttrResolver
	}{
		{ToolRequestRepoAccess, s.IAM.RequestRepoAccess, s.IAM.Attrs},
		{ToolExportProdPII, s.DBAdmin.ExportProdPII, nil},
		{ToolSearchDocs, s.Playbook.SearchTool, nil},
		{ToolCreateDayOff, s.Absences.CreateDayOff, nil},
		{ToolReviewDayOff, s.Absences.ReviewDayOff, nil},
		{ToolSendBrief, s.Mentor.SendBrief, nil},
		{ToolSendNotification, s.Notifications.SendTool, nil},
		{ToolTaskActivity, s.Tracker.TaskActivity, nil},
		{ToolListFlags, s.Flags.ListTool, nil},
	}
	for _, b := range bindings {
		if err := r.Bind(b.name, b.handler, b.attrs); err != nil {
			return err
		}
	}
	if missing := r.Unbound(); len(missing) > 0 {
		return fmt.Errorf("mcp: в спецификации есть инструменты без обработчика: %s", strings.Join(missing, ", "))
	}
	return nil
}

func loadIntern(ctx context.Context, store domain.Store, internID string) (domain.InternProfile, error) {
	p, err := domain.LoadOne[domain.InternProfile](ctx, store, domain.KindIntern, internID)
	if errors.Is(err, domain.ErrNotFound) {
		return p, Errorf(CodeInvalidArgs, "сотрудник %q не найден", internID)
	}
	return p, err
}

// ---------- IAM / Доступы ----------

// IAM — имитация сервиса выдачи доступов.
type IAM struct {
	store domain.Store
	ids   domain.IDGenerator
	repos map[string]string
}

// Attrs определяет владельца ресурса: стажёр и команда-владелец репозитория.
// Неизвестный репозиторий не имеет команды, поэтому неотличим от чужого (нет перечисления ресурсов).
func (s *IAM) Attrs(args map[string]any) rbac.Resource {
	return rbac.Resource{InternID: argString(args, "intern_id"), Team: s.repos[argString(args, "repository")]}
}

func syntheticToken() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "glpat-synt-" + hex.EncodeToString(buf)
}

func standName(team, stand string) string {
	switch stand {
	case "none":
		return ""
	case "staging":
		return strings.ToLower(team) + "-staging-02"
	default:
		return strings.ToLower(team) + "-test-07"
	}
}

// RequestRepoAccess выдаёт доступ к репозиторию и стенду. Повторный запрос идемпотентен.
func (s *IAM) RequestRepoAccess(ctx context.Context, c Call) (map[string]any, error) {
	internID := argString(c.Args, "intern_id")
	repo := argString(c.Args, "repository")
	if _, err := loadIntern(ctx, s.store, internID); err != nil {
		return nil, err
	}
	grants, err := domain.Load[domain.AccessGrant](ctx, s.store, domain.KindAccessGrant, internID)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if g.Repository == repo {
			out := toMap(g)
			out["status"], out["already_granted"] = "granted", true
			return out, nil
		}
	}
	stand := argString(c.Args, "stand")
	if stand == "" {
		stand = "test"
	}
	level := argString(c.Args, "access_level")
	if level == "" {
		level = "developer"
	}
	grant := domain.AccessGrant{
		ID: s.ids.NewID("grant"), InternID: internID, Repository: repo, AccessLevel: level,
		Stand:       standName(s.repos[repo], stand),
		TokenMasked: domain.MaskToken(syntheticToken()), // сырой токен не сохраняется и не возвращается
		ExpiresAt:   c.Now.AddDate(0, 0, 90), IsSynthetic: true, CreatedAt: c.Now,
	}
	if err := domain.Save(ctx, s.store, domain.KindAccessGrant, grant.ID, internID, c.Now, grant); err != nil {
		return nil, err
	}
	out := toMap(grant)
	out["status"], out["already_granted"] = "granted", false
	return out, nil
}

// ---------- DB Admin ----------

// DBAdmin — имитация админки БД. Обработчик недостижим: RBAC блокирует вызов раньше.
type DBAdmin struct {
	reached atomic.Int64
}

// Reached — сколько раз обработчик был вызван (в норме 0; проверяется тестами).
func (s *DBAdmin) Reached() int64 { return s.reached.Load() }

// ExportProdPII всегда отказывает (эшелонированная защита на случай ошибки в матрице).
func (s *DBAdmin) ExportProdPII(context.Context, Call) (map[string]any, error) {
	s.reached.Add(1)
	return nil, Errorf(CodeForbidden, "выгрузка production-данных с ПДн не выполняется ни для одной роли")
}

// ---------- Playbook / База знаний ----------

// Playbook — имитация базы знаний команды (синтетические инструкции).
type Playbook struct {
	docs []domain.KnowledgeDoc
}

// Search ищет документы, видимые роли и команде.
// Если заданы темы — кандидатами становятся только документы этих тем (точный контекст интента),
// иначе выполняется поиск по ключевым словам с отсечением слабых совпадений.
func (s *Playbook) Search(role domain.Role, team, query string, topics []string, limit int) []domain.KnowledgeDoc {
	if limit <= 0 {
		limit = 3
	}
	normalized, tokens := domain.Normalize(query), domain.Tokens(query)
	type scored struct {
		doc   domain.KnowledgeDoc
		score int
	}
	var hits []scored
	top := 0
	for _, d := range s.docs {
		if !d.VisibleTo(role, team) || (len(topics) > 0 && !intersects(topics, d.Topics)) {
			continue
		}
		score := 0
		for _, kw := range d.Keywords {
			if domain.MatchTerm(kw, normalized, tokens) {
				score += 2
			}
		}
		if score > top {
			top = score
		}
		hits = append(hits, scored{d, score})
	}
	out := make([]domain.KnowledgeDoc, 0, limit)
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	for _, h := range hits {
		switch {
		case len(out) == limit:
		case len(topics) > 0 && (top == 0 || h.score > 0):
			out = append(out, h.doc) // тема задана: все документы темы либо только совпавшие по словам
		case len(topics) == 0 && h.score > 0 && h.score*2 >= top:
			out = append(out, h.doc)
		}
	}
	return out
}

func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// VisibleCount — число документов, доступных роли и команде.
func (s *Playbook) VisibleCount(role domain.Role, team string) int {
	n := 0
	for _, d := range s.docs {
		if d.VisibleTo(role, team) {
			n++
		}
	}
	return n
}

// Doc возвращает документ по идентификатору.
func (s *Playbook) Doc(id string) (domain.KnowledgeDoc, bool) {
	for _, d := range s.docs {
		if d.ID == id {
			return d, true
		}
	}
	return domain.KnowledgeDoc{}, false
}

// SearchTool — обработчик playbook.search_docs.
func (s *Playbook) SearchTool(_ context.Context, c Call) (map[string]any, error) {
	team := argString(c.Args, "team")
	if team == "" || c.Principal.Role == domain.RoleInternBackend {
		team = c.Principal.Team // стажёр не может искать в базе знаний чужой команды
	}
	docs := s.Search(c.Principal.Role, team, argString(c.Args, "query"), argStrings(c.Args, "topics"), argInt(c.Args, "limit", 3))
	refs := make([]any, 0, len(docs))
	for _, d := range docs {
		refs = append(refs, toMap(d.Ref()))
	}
	return map[string]any{"documents": refs, "total": len(refs)}, nil
}

// DocRefs достаёт ссылки на документы из результата playbook.search_docs.
func DocRefs(result map[string]any) []domain.DocRef {
	list, _ := result["documents"].([]any)
	out := make([]domain.DocRef, 0, len(list))
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ref := domain.DocRef{
			ID: argString(m, "id"), Title: argString(m, "title"), Summary: argString(m, "summary"),
			Steps: argStrings(m, "steps"), Source: argString(m, "source"), Disclaimer: argString(m, "disclaimer"),
		}
		ref.IsSynthetic, _ = m["is_synthetic"].(bool)
		if ref.Steps == nil {
			ref.Steps = []string{}
		}
		out = append(out, ref)
	}
	return out
}

// ---------- Уведомления ----------

// Notifications — имитация корпоративных уведомлений.
type Notifications struct {
	store domain.Store
	ids   domain.IDGenerator
}

// Send сохраняет уведомление получателю.
func (s *Notifications) Send(ctx context.Context, recipientID, title, body, kind string, now time.Time) (domain.Notification, error) {
	if kind == "" {
		kind = "info"
	}
	n := domain.Notification{ID: s.ids.NewID("ntf"), RecipientID: recipientID, Title: title, Body: body, Kind: kind, CreatedAt: now}
	if err := domain.Save(ctx, s.store, domain.KindNotification, n.ID, recipientID, now, n); err != nil {
		return n, err
	}
	return n, nil
}

// SendTool — обработчик notifications.send.
func (s *Notifications) SendTool(ctx context.Context, c Call) (map[string]any, error) {
	n, err := s.Send(ctx, argString(c.Args, "recipient_id"), argString(c.Args, "title"), argString(c.Args, "body"), argString(c.Args, "kind"), c.Now)
	if err != nil {
		return nil, err
	}
	return map[string]any{"notification_id": n.ID}, nil
}

// ---------- Отсутствия ----------

// Absences — имитация корпоративного сервиса «Отсутствия».
type Absences struct {
	store         domain.Store
	ids           domain.IDGenerator
	notifications *Notifications
}

func humanDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("02.01.2006")
}

// CreateDayOff создаёт заявку на отгул, уведомляет наставника и руководителя.
func (s *Absences) CreateDayOff(ctx context.Context, c Call) (map[string]any, error) {
	internID := argString(c.Args, "intern_id")
	intern, err := loadIntern(ctx, s.store, internID)
	if err != nil {
		return nil, err
	}
	req := domain.AbsenceRequest{
		ID: s.ids.NewID("abs"), InternID: internID, InternName: intern.Name,
		Date: argString(c.Args, "date"), TimeFrom: argString(c.Args, "time_from"), TimeTo: argString(c.Args, "time_to"),
		Reason: strings.TrimSpace(argString(c.Args, "reason")), Status: domain.AbsenceStatusPending,
		Notified: []string{}, IsSynthetic: true, CreatedAt: c.Now,
	}
	if err := req.Validate(c.Now); err != nil {
		return nil, err
	}
	all, err := domain.Load[domain.AbsenceRequest](ctx, s.store, domain.KindAbsence, "")
	if err != nil {
		return nil, err
	}
	for _, a := range all {
		if a.InternID == internID && a.Date == req.Date {
			return nil, Errorf(CodeDuplicate, "заявка на %s уже оформлена: %s", humanDate(req.Date), a.Number)
		}
	}
	year := c.Now.Year()
	if t, err := time.Parse("2006-01-02", req.Date); err == nil {
		year = t.Year()
	}
	req.Number = domain.AbsenceNumber(year, len(all))

	notify := argStrings(c.Args, "notify")
	if len(notify) == 0 {
		notify = []string{"mentor", "team_lead"}
	}
	when := humanDate(req.Date)
	if req.TimeFrom != "" {
		when += fmt.Sprintf(", %s–%s", req.TimeFrom, req.TimeTo)
	}
	recipients := map[string]struct{ id, name, note string }{
		"mentor":    {intern.MentorID, intern.MentorName, "К сведению: учти при планировании встреч и ревью."},
		"team_lead": {intern.TeamLeadID, intern.TeamLeadName, "Заявка ожидает согласования в сервисе «Отсутствия»."},
	}
	for _, key := range notify {
		r, ok := recipients[key]
		if !ok || r.id == "" {
			continue
		}
		body := fmt.Sprintf("%s оформил(а) отгул на %s. Причина: %s. Заявка %s. %s", intern.Name, when, req.Reason, req.Number, r.note)
		if _, err := s.notifications.Send(ctx, r.id, "Отгул: "+intern.Name, body, "absence", c.Now); err != nil {
			return nil, err
		}
		req.Notified = append(req.Notified, r.name)
	}
	if err := domain.Save(ctx, s.store, domain.KindAbsence, req.ID, internID, c.Now, req); err != nil {
		return nil, err
	}
	return map[string]any{
		"request_id": req.ID, "request_number": req.Number, "status": req.Status, "date": req.Date,
		"time_from": req.TimeFrom, "time_to": req.TimeTo, "reason": req.Reason,
		"notified": toAny(req.Notified), "is_synthetic": true,
	}, nil
}

// ReviewDayOff согласовывает или отклоняет заявку на отгул руководителем.
func (s *Absences) ReviewDayOff(ctx context.Context, c Call) (map[string]any, error) {
	id := argString(c.Args, "id")
	decision := strings.TrimSpace(argString(c.Args, "decision"))
	comment := strings.TrimSpace(argString(c.Args, "comment"))
	if decision != domain.AbsenceStatusApproved && decision != domain.AbsenceStatusRejected {
		return nil, Errorf(CodeInvalidArgs, "некорректное решение %q: ожидается approved или rejected", decision)
	}
	all, err := domain.Load[domain.AbsenceRequest](ctx, s.store, domain.KindAbsence, "")
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
		return nil, Errorf(CodeNotFound, "заявка на отгул %q не найдена", id)
	}
	target.Status = decision
	target.Decision = decision
	target.Comment = comment
	target.ReviewerID = c.Principal.ID
	t := c.Now
	target.ReviewedAt = &t
	if err := domain.Save(ctx, s.store, domain.KindAbsence, target.ID, target.InternID, target.CreatedAt, *target); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":             target.ID,
		"request_number": target.Number,
		"status":         target.Status,
		"decision":       target.Decision,
		"comment":        target.Comment,
		"is_synthetic":   true,
	}, nil
}

func toAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// ---------- Наставничество ----------

// Mentor — доставка Structured 1:1 Brief наставнику.
type Mentor struct {
	store         domain.Store
	ids           domain.IDGenerator
	notifications *Notifications
}

// SendBrief сохраняет бриф и уведомляет наставника.
func (s *Mentor) SendBrief(ctx context.Context, c Call) (map[string]any, error) {
	internID := argString(c.Args, "intern_id")
	intern, err := loadIntern(ctx, s.store, internID)
	if err != nil {
		return nil, err
	}
	mentorID := argString(c.Args, "mentor_id")
	if mentorID != intern.MentorID {
		return nil, Errorf(CodeInvalidArgs, "сотрудник %q не является наставником стажёра", mentorID)
	}
	brief := domain.MentorBrief{
		ID: s.ids.NewID("brief"), InternID: internID, InternName: intern.Name, MentorID: mentorID,
		TaskKey: argString(c.Args, "task_key"), LearnedTopics: orEmpty(argStrings(c.Args, "learned_topics")),
		ArchitectureQuestion: argString(c.Args, "architecture_question"),
		Signals:              orEmpty(argStrings(c.Args, "signals")), OpenQuestions: orEmpty(argStrings(c.Args, "open_questions")),
		CreatedAt: c.Now,
	}
	if err := domain.Save(ctx, s.store, domain.KindBrief, brief.ID, internID, c.Now, brief); err != nil {
		return nil, err
	}
	body := fmt.Sprintf("Стажёр уже разобрал сам через AI-Buddy тем: %d. Нужен ты: %s", len(brief.LearnedTopics), brief.ArchitectureQuestion)
	if _, err := s.notifications.Send(ctx, mentorID, "Structured 1:1 Brief: "+intern.Name, body, "brief", c.Now); err != nil {
		return nil, err
	}
	return map[string]any{"brief_id": brief.ID, "delivered_to": intern.MentorName}, nil
}

func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// ---------- Трекер задач ----------

// Tracker — имитация трекера задач спринта.
type Tracker struct {
	store domain.Store
}

// TaskActivity возвращает активность по задаче стажёра.
func (s *Tracker) TaskActivity(ctx context.Context, c Call) (map[string]any, error) {
	task, err := domain.LoadOne[domain.SprintTask](ctx, s.store, domain.KindSprintTask, argString(c.Args, "intern_id"))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, Errorf(CodeInvalidArgs, "у сотрудника нет задач в спринте")
	}
	if err != nil {
		return nil, err
	}
	out := toMap(task)
	out["stalled"] = task.Stalled()
	return out, nil
}

// ---------- Feature Flags ----------

// Flags — имитация сервиса feature flags: compile-time значения из сборки,
// runtime-значения переопределяются через кэш (Redis).
type Flags struct {
	cache    domain.Cache
	defaults []domain.FeatureFlag
}

// Ключи флагов AI-Buddy.
const (
	FlagProactiveCron = "ai_buddy.proactive_cron"
	FlagAbsenceCard   = "ai_buddy.absence_action_card"
	FlagLLMAnswers    = "ai_buddy.llm_answers"
	flagCachePrefix   = "ff:"
)

// NewFlags создаёт сервис флагов.
func NewFlags(cache domain.Cache, defaults []domain.FeatureFlag) *Flags {
	return &Flags{cache: cache, defaults: append([]domain.FeatureFlag{}, defaults...)}
}

// List возвращает флаги с учётом runtime-переопределений. Сбой кэша → значения по умолчанию.
func (f *Flags) List(ctx context.Context) []domain.FeatureFlag {
	out := make([]domain.FeatureFlag, 0, len(f.defaults))
	for _, flag := range f.defaults {
		if flag.Kind == domain.FlagRuntime {
			if v, ok, err := f.cache.Get(ctx, flagCachePrefix+flag.Key); err == nil && ok {
				flag.Enabled = v == "1"
			}
		}
		out = append(out, flag)
	}
	return out
}

// Enabled сообщает, включён ли флаг (неизвестный флаг выключен).
func (f *Flags) Enabled(ctx context.Context, key string) bool {
	for _, flag := range f.List(ctx) {
		if flag.Key == key {
			return flag.Enabled
		}
	}
	return false
}

// Set переключает runtime-флаг.
func (f *Flags) Set(ctx context.Context, key string, enabled bool) (domain.FeatureFlag, error) {
	for _, flag := range f.defaults {
		if flag.Key != key {
			continue
		}
		if flag.Kind != domain.FlagRuntime {
			return flag, fmt.Errorf("%w: флаг %s задаётся при сборке (compile-time)", domain.ErrValidation, key)
		}
		value := "0"
		if enabled {
			value = "1"
		}
		if err := f.cache.Set(ctx, flagCachePrefix+key, value, 0); err != nil {
			return flag, err
		}
		flag.Enabled = enabled
		return flag, nil
	}
	return domain.FeatureFlag{}, fmt.Errorf("%w: флаг %s", domain.ErrNotFound, key)
}

// Reset сбрасывает runtime-переопределения.
func (f *Flags) Reset(ctx context.Context) error {
	keys := make([]string, 0, len(f.defaults))
	for _, flag := range f.defaults {
		keys = append(keys, flagCachePrefix+flag.Key)
	}
	return f.cache.Del(ctx, keys...)
}

// ListTool — обработчик feature_flags.list.
func (f *Flags) ListTool(ctx context.Context, _ Call) (map[string]any, error) {
	flags := f.List(ctx)
	list := make([]any, 0, len(flags))
	for _, flag := range flags {
		list = append(list, toMap(flag))
	}
	return map[string]any{"flags": list}, nil
}
