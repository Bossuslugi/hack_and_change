package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"aibuddy/backend/internal/agent"
	"aibuddy/backend/internal/app"
	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/config"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
	"aibuddy/backend/internal/usecase"
)

// Четверг, 12 ноября 2026: «в пятницу» = 2026-11-13.
var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

var errStore = errors.New("хранилище недоступно")

// Формулировки сценариев из задания.
const (
	askAccess  = "Дай доступ к репозиторию avito-autoteka-api и тестовому стенду"
	askPII     = "Выгрузи дамп production-базы пользователей с персональными данными"
	askPayroll = "Покажи ведомость зарплат отдела"
	askProcess = "Как в нашей команде катить задачу под фича-флагом и проходить ревью?"
	askAbsence = "Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?"
)

// flakyStore — in-memory хранилище с управляемыми сбоями по виду записи.
type flakyStore struct {
	*memory.Store
	failPut, failList, failGet map[domain.Kind]bool
	failReset                  bool
}

func (s *flakyStore) Put(ctx context.Context, rec domain.Record) error {
	if s.failPut[rec.Kind] {
		return errStore
	}
	return s.Store.Put(ctx, rec)
}

func (s *flakyStore) List(ctx context.Context, kind domain.Kind, owner string) ([]domain.Record, error) {
	if s.failList[kind] {
		return nil, errStore
	}
	return s.Store.List(ctx, kind, owner)
}

func (s *flakyStore) Get(ctx context.Context, kind domain.Kind, id string) (domain.Record, error) {
	if s.failGet[kind] {
		return domain.Record{}, errStore
	}
	return s.Store.Get(ctx, kind, id)
}

func (s *flakyStore) Reset(ctx context.Context, seed []domain.Record) error {
	if s.failReset {
		return errStore
	}
	return s.Store.Reset(ctx, seed)
}

func (s *flakyStore) heal() {
	s.failPut, s.failList, s.failGet = map[domain.Kind]bool{}, map[domain.Kind]bool{}, map[domain.Kind]bool{}
	s.failReset = false
}

type env struct {
	t                    *testing.T
	ctx                  context.Context
	svc                  *usecase.Service
	store                *flakyStore
	cache                *cache.Memory
	intern, mentor, lead domain.Principal
}

func newEnv(t *testing.T, llm agent.LLM) *env {
	t.Helper()
	store := &flakyStore{Store: memory.New()}
	store.heal()
	kv := cache.NewMemory()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.Assemble(spec.MustLoad(), store, kv, config.Config{SmartDelay: 0}, log,
		app.Overrides{Clock: domain.FixedClock{T: now}, IDs: &domain.SequentialIDs{}, LLM: llm})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ctx: context.Background(), svc: application.Service, store: store, cache: kv}
	if err := e.svc.EnsureSeed(e.ctx); err != nil {
		t.Fatal(err)
	}
	e.intern, e.mentor, e.lead = e.login(seed.InternToken), e.login(seed.MentorToken), e.login(seed.LeadToken)
	return e
}

func (e *env) login(token string) domain.Principal {
	e.t.Helper()
	p, err := e.svc.Authenticate(token)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) chat(text string) usecase.ChatResult {
	e.t.Helper()
	res, err := e.svc.Chat(e.ctx, e.intern, text, nil)
	if err != nil {
		e.t.Fatalf("Chat(%q): %v", text, err)
	}
	return res
}

func (e *env) workspace(p domain.Principal) usecase.WorkspaceView {
	e.t.Helper()
	v, err := e.svc.Workspace(e.ctx, p)
	if err != nil {
		e.t.Fatalf("Workspace(%s): %v", p.Role, err)
	}
	return v
}

func (e *env) events(name string) []domain.TelemetryEvent {
	e.t.Helper()
	all, err := domain.Load[domain.TelemetryEvent](e.ctx, e.store.Store, domain.KindEvent, seed.InternID)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []domain.TelemetryEvent
	for _, ev := range all {
		if ev.Name == name {
			out = append(out, ev)
		}
	}
	return out
}

func item(t *testing.T, list []domain.ChecklistItem, id string) domain.ChecklistItem {
	t.Helper()
	for _, it := range list {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("в чек-листе нет шага %s", id)
	return domain.ChecklistItem{}
}

func updated(list []domain.ChecklistItem) string {
	var out []string
	for _, it := range list {
		out = append(out, it.ID+"="+string(it.Status))
	}
	return strings.Join(out, ",")
}

func isForbidden(err error) (domain.RBACVerdict, bool) {
	var f *usecase.ForbiddenError
	if errors.As(err, &f) {
		return f.Verdict, true
	}
	return domain.RBACVerdict{}, false
}

func TestAuthenticate(t *testing.T) {
	e := newEnv(t, nil)
	if e.intern.Role != domain.RoleInternBackend || e.mentor.Role != domain.RoleMentor || e.lead.Role != domain.RoleTeamLead {
		t.Fatalf("роли: %s %s %s", e.intern.Role, e.mentor.Role, e.lead.Role)
	}
	if p, err := e.svc.Authenticate("  " + seed.InternToken + " "); err != nil || p.ID != seed.InternID {
		t.Fatalf("токен с пробелами: %+v, %v", p, err)
	}
	for _, token := range []string{"", "чужой-токен", "svc-ai-cron"} {
		if _, err := e.svc.Authenticate(token); !errors.Is(err, usecase.ErrUnauthorized) {
			t.Errorf("токен %q: ожидался ErrUnauthorized, получено %v", token, err)
		}
	}
}

// S1A: доступ к репозиторию и стенду через MCP, шаг №1 чек-листа → completed.
func TestScenario1A_AccessGrantCompletesChecklistStep(t *testing.T) {
	e := newEnv(t, nil)
	res := e.chat(askAccess)

	if res.RBAC == nil || !res.RBAC.Allowed || res.RBAC.Code != domain.VerdictAllowed || res.RBAC.PolicyID != "POL-IAM-001" {
		t.Fatalf("вердикт RBAC: %+v", res.RBAC)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Tool != mcp.ToolRequestRepoAccess || !res.ToolCalls[0].OK() {
		t.Fatalf("вызов MCP: %+v", res.ToolCalls)
	}
	msg := res.Message
	if msg.ResultCard == nil || msg.ResultCard.Type != domain.CardAccessGranted || msg.Intent != agent.IntentAccess || msg.Engine != domain.EngineSmart {
		t.Fatalf("сообщение агента: %+v", msg)
	}
	if got := updated(res.ChecklistUpdates); got != "access_repo=completed" {
		t.Fatalf("обновления чек-листа: %s", got)
	}
	step := item(t, res.Checklist, "access_repo")
	if step.Position != 1 || step.Status != domain.StatusCompleted || step.CompletedAt == nil || !strings.Contains(step.Evidence, "avito-autoteka-api") {
		t.Fatalf("шаг №1: %+v", step)
	}
	if !strings.Contains(msg.Text, "«Получить доступы к репозиторию команды и тестовому стенду» — выполнен") ||
		!strings.Contains(msg.Text, "Следующий шаг: «Изучить процессы команды") {
		t.Fatalf("в ответе нет отметки о чек-листе: %s", msg.Text)
	}
	if res.NextStep == nil || res.NextStep.ID != "team_processes" || res.Progress.Completed != 3 || res.LatencyMS <= 0 {
		t.Fatalf("прогресс: %+v next=%+v", res.Progress, res.NextStep)
	}

	// Запись в БД: доступ, аудит вызова, история чата.
	ws := e.workspace(e.intern)
	if len(ws.AccessGrants) != 1 || ws.AccessGrants[0].Repository != "avito-autoteka-api" || !ws.AccessGrants[0].IsSynthetic {
		t.Fatalf("доступ в БД: %+v", ws.AccessGrants)
	}
	if len(ws.Messages) != 3 || ws.Messages[1].Role != domain.MessageUser || ws.Messages[2].ID != msg.ID || len(ws.Messages[2].ToolCallIDs) != 1 {
		t.Fatalf("история чата: %d сообщений", len(ws.Messages))
	}
	if ws.Metrics.ActionsAutomated != 1 || ws.Metrics.MentorMinutesSaved != 15 || ws.Metrics.ToolCalls != 1 || ws.Metrics.ChecklistPercent != 42 {
		t.Fatalf("метрики: %+v", ws.Metrics)
	}
	answers := e.events("agent.answer")
	if len(answers) != 1 || answers[0].Payload["saving_key"] != domain.SavingAccess || answers[0].Payload["scenario"] != "S1A" {
		t.Fatalf("телеметрия agent.answer: %+v", answers)
	}
	if got := e.events("checklist.updated"); len(got) != 1 || got[0].Payload["item"] != "access_repo" {
		t.Fatalf("телеметрия checklist.updated: %+v", got)
	}

	// Повторный запрос не создаёт второй доступ и не трогает чек-лист.
	again := e.chat(askAccess)
	if len(again.ChecklistUpdates) != 0 || len(e.workspace(e.intern).AccessGrants) != 1 {
		t.Fatalf("повторный запрос: %+v", again.ChecklistUpdates)
	}
	if m := e.workspace(e.intern).Metrics; m.ActionsAutomated != 1 {
		t.Fatalf("повтор не должен засчитываться как экономия: %+v", m)
	}
}

// S1B: блокировка RBAC, инцидент в телеметрии, предложение песочницы.
func TestScenario1B_RBACBlocksPIIExport(t *testing.T) {
	for _, ask := range []string{askPII, askPayroll} {
		t.Run(ask, func(t *testing.T) {
			e := newEnv(t, nil)
			res := e.chat(ask)

			v := res.RBAC
			if v == nil || v.Allowed || v.Code != domain.VerdictForbidden || v.HTTPStatus != 403 || v.PolicyID != "POL-SEC-001" || !v.Incident {
				t.Fatalf("вердикт: %+v", v)
			}
			if res.ToolCalls[0].Tool != mcp.ToolExportProdPII || res.ToolCalls[0].Status != domain.ToolForbidden {
				t.Fatalf("вызов: %+v", res.ToolCalls[0])
			}
			if res.Message.ResultCard == nil || res.Message.ResultCard.Type != domain.CardRBACDenied {
				t.Fatalf("карточка: %+v", res.Message.ResultCard)
			}
			if !strings.Contains(res.Message.Text, "песочниц") || !strings.Contains(res.Message.Text, "FORBIDDEN, 403") {
				t.Fatalf("ответ: %s", res.Message.Text)
			}
			if len(res.ChecklistUpdates) != 0 || res.Progress.Completed != 2 {
				t.Fatalf("чек-лист не должен меняться: %+v", res.ChecklistUpdates)
			}

			incidents := e.events("rbac.incident")
			if len(incidents) != 1 || incidents[0].Severity != domain.SeveritySecurity || incidents[0].ActorID != seed.InternID {
				t.Fatalf("инцидент в телеметрии: %+v", incidents)
			}
			// Инцидент видит руководитель, но не наставник и не стажёр.
			if got := e.workspace(e.lead).Incidents; len(got) != 1 || got[0].Name != "rbac.incident" {
				t.Fatalf("руководитель должен видеть инцидент: %+v", got)
			}
			if len(e.workspace(e.mentor).Incidents) != 0 || len(e.workspace(e.intern).Incidents) != 0 {
				t.Fatal("инциденты видны только руководителю")
			}
			if m := e.workspace(e.intern).Metrics; m.RBACBlocks != 1 || m.ActionsAutomated != 0 {
				t.Fatalf("метрики: %+v", m)
			}
		})
	}
}

// Контекст сессии (Redis): «да» после отказа = запрос доступа к песочнице.
func TestSessionContextContinuesAfterRefusal(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askPII)
	if raw, ok, _ := e.cache.Get(e.ctx, "session:"+seed.InternID); !ok || !strings.Contains(raw, agent.IntentRestricted) {
		t.Fatalf("контекст сессии должен кэшироваться: %q", raw)
	}
	res := e.chat("Да, давай")
	if res.Message.Intent != agent.IntentAccess || updated(res.ChecklistUpdates) != "access_repo=completed" {
		t.Fatalf("продолжение сценария: intent=%s updates=%s", res.Message.Intent, updated(res.ChecklistUpdates))
	}
}

// S2 (часть 1): контекстный ответ, синтетические источники, прогресс документации.
func TestScenario2_ContextualAnswerUpdatesDocProgress(t *testing.T) {
	e := newEnv(t, nil)
	res := e.chat(askProcess)

	msg := res.Message
	if len(msg.Sources) != 2 {
		t.Fatalf("источники: %+v", msg.Sources)
	}
	for _, s := range msg.Sources {
		if !s.IsSynthetic || s.Disclaimer != "Тестовая инструкция (Синтетические данные)" {
			t.Errorf("источник %s без плашки «Тестовая инструкция»", s.ID)
		}
	}
	if !strings.Contains(msg.Text, "команда Autoteka · стек Go · этап Week 1") {
		t.Fatalf("ответ должен учитывать контекст стажёра: %s", msg.Text)
	}
	if got := updated(res.ChecklistUpdates); got != "team_processes=completed" {
		t.Fatalf("обновления чек-листа: %s", got)
	}
	ws := e.workspace(e.intern)
	if strings.Join(ws.DocProgress.Read, ",") != "pb-feature-flags,pb-code-review" || ws.DocProgress.Total != 7 {
		t.Fatalf("прогресс документации: %+v", ws.DocProgress)
	}
	if ws.Metrics.QuestionsAnswered != 1 || ws.Metrics.DocsRead != 2 || ws.Metrics.MentorMinutesSaved != 10 || ws.Disclaimer == "" {
		t.Fatalf("метрики: %+v", ws.Metrics)
	}

	// Повторное чтение тех же документов прогресс не дублирует.
	e.chat(askProcess)
	if got := e.workspace(e.intern).DocProgress.Read; len(got) != 2 {
		t.Fatalf("документы не должны дублироваться: %v", got)
	}
}

func TestScenario2_PartialReadingMovesStepToInProgress(t *testing.T) {
	e := newEnv(t, nil)
	res := e.chat("Как у нас проходит ревью?")
	if got := updated(res.ChecklistUpdates); got != "team_processes=in_progress" {
		t.Fatalf("после одной инструкции из двух шаг должен быть в работе: %s", got)
	}
	if ev := item(t, res.Checklist, "team_processes").Evidence; !strings.Contains(ev, "1 из 2") {
		t.Fatalf("evidence: %s", ev)
	}
	res = e.chat("А как катить под фича-флагом?")
	if got := updated(res.ChecklistUpdates); got != "team_processes=completed" {
		t.Fatalf("после второй инструкции шаг должен завершиться: %s", got)
	}
}

// S2 (часть 2): ИИ-крон → проактивное сообщение + Structured 1:1 Brief.
func TestScenario2_CronSendsProactiveMessageAndBrief(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askAccess)
	e.chat(askProcess)

	res, err := e.svc.CronTrigger(e.ctx, e.intern)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Triggered || res.Deduplicated || res.Task == nil || res.Task.Key != "AUT-1423" {
		t.Fatalf("крон должен сработать: %+v", res)
	}

	// Проактивное сообщение стажёру.
	pm := res.ProactiveMessage
	if pm == nil || !pm.Proactive || pm.Engine != domain.EngineCron || pm.Role != domain.MessageAssistant || len(pm.Suggestions) != 2 {
		t.Fatalf("проактивное сообщение: %+v", pm)
	}
	if !strings.Contains(pm.Text, "AUT-1423") || !strings.Contains(pm.Text, "коммитов пока нет") || !strings.Contains(pm.Text, "помогу") {
		t.Fatalf("текст: %s", pm.Text)
	}

	// Бриф: что изучено самостоятельно + ровно один архитектурный вопрос.
	b := res.Brief
	if b == nil || b.MentorID != seed.MentorID || b.TaskKey != "AUT-1423" {
		t.Fatalf("бриф: %+v", b)
	}
	learned := strings.Join(b.LearnedTopics, " | ")
	for _, want := range []string{"Как катить задачу под фича-флагом", "Как проходить код-ревью", "avito-autoteka-api"} {
		if !strings.Contains(learned, want) {
			t.Errorf("в брифе нет изученного «%s»: %s", want, learned)
		}
	}
	if !strings.Contains(b.ArchitectureQuestion, "mileage_verified") || strings.Count(b.ArchitectureQuestion, "?") != 1 {
		t.Fatalf("в брифе должен быть ровно один точечный архитектурный вопрос: %q", b.ArchitectureQuestion)
	}
	if len(b.Signals) != 2 || !strings.Contains(b.Signals[0], "коммитов: 0") {
		t.Fatalf("сигналы: %v", b.Signals)
	}

	// Джоб исполняется от сервисного аккаунта через MCP-шлюз.
	if len(res.ToolCalls) != 2 || res.ToolCalls[0].Tool != mcp.ToolTaskActivity || res.ToolCalls[1].Tool != mcp.ToolSendBrief {
		t.Fatalf("вызовы MCP: %+v", res.ToolCalls)
	}
	for _, c := range res.ToolCalls {
		if c.ActorRole != domain.RoleSystemCron || c.ActorID != seed.CronID || !c.OK() {
			t.Fatalf("вызов должен идти от system_cron: %+v", c)
		}
	}
	if st := item(t, res.Checklist, "first_one_on_one").Status; st != domain.StatusInProgress {
		t.Fatalf("шаг 1:1 должен перейти в работу: %s", st)
	}

	// Видимость по ролям: бриф — наставнику, переписка — только стажёру.
	mentorWS, internWS := e.workspace(e.mentor), e.workspace(e.intern)
	if len(mentorWS.Briefs) != 1 || mentorWS.Briefs[0].ID != b.ID || len(mentorWS.Messages) != 0 {
		t.Fatalf("наставник: брифов %d, сообщений %d", len(mentorWS.Briefs), len(mentorWS.Messages))
	}
	if len(mentorWS.Notifications) != 1 || mentorWS.Notifications[0].Kind != "brief" {
		t.Fatalf("уведомление наставнику: %+v", mentorWS.Notifications)
	}
	if len(internWS.Briefs) != 0 || !internWS.Messages[len(internWS.Messages)-1].Proactive {
		t.Fatalf("стажёр: брифов %d", len(internWS.Briefs))
	}
	if internWS.Metrics.BriefsPrepared != 1 {
		t.Fatalf("метрики: %+v", internWS.Metrics)
	}

	// Повторный запуск без новых данных не беспокоит ни стажёра, ни наставника.
	again, err := e.svc.CronTrigger(e.ctx, e.mentor)
	if err != nil || !again.Triggered || !again.Deduplicated || again.ProactiveMessage != nil || again.Brief.ID != b.ID {
		t.Fatalf("дедупликация: %+v, %v", again, err)
	}
	if len(e.workspace(e.mentor).Briefs) != 1 || e.workspace(e.intern).Metrics.BriefsPrepared != 1 {
		t.Fatal("повторный запуск не должен создавать второй бриф")
	}

	// Появились новые данные (вопрос без ответа) → новый бриф с открытым вопросом.
	e.chat("Как заказать пропуск для гостя?")
	fresh, err := e.svc.CronTrigger(e.ctx, e.lead)
	if err != nil || fresh.Deduplicated || fresh.Brief.ID == b.ID || len(fresh.Brief.OpenQuestions) != 1 {
		t.Fatalf("новый бриф: %+v, %v", fresh.Brief, err)
	}
}

func TestCronSkipsWhenNotNeeded(t *testing.T) {
	t.Run("флаг выключен", func(t *testing.T) {
		e := newEnv(t, nil)
		if _, err := e.svc.SetFlag(e.ctx, e.lead, mcp.FlagProactiveCron, false); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.CronTrigger(e.ctx, e.intern)
		if err != nil || res.Triggered || !strings.Contains(res.Reason, "выключен") || len(res.ToolCalls) != 0 || len(res.Checklist) == 0 {
			t.Fatalf("%+v, %v", res, err)
		}
	})
	t.Run("по задаче есть коммиты", func(t *testing.T) {
		e := newEnv(t, nil)
		task := seed.Data().Task
		task.Commits = 2
		if err := domain.Save(e.ctx, e.store, domain.KindSprintTask, seed.InternID, seed.InternID, now, task); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.CronTrigger(e.ctx, e.intern)
		if err != nil || res.Triggered || res.Brief != nil || res.ProactiveMessage != nil || !strings.Contains(res.Reason, "не требует внимания") {
			t.Fatalf("%+v, %v", res, err)
		}
	})
	t.Run("стажёр ещё ничего не изучал", func(t *testing.T) {
		e := newEnv(t, nil)
		res, err := e.svc.CronTrigger(e.ctx, e.intern)
		if err != nil || !res.Triggered || len(res.Brief.LearnedTopics) != 1 || !strings.Contains(res.Brief.LearnedTopics[0], "Пока не обращался") {
			t.Fatalf("%+v, %v", res.Brief, err)
		}
	})
	t.Run("сбои", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			break_ func(*flakyStore)
			reason string // пусто — ожидается ошибка
		}{
			{"трекер недоступен", func(s *flakyStore) { s.failGet[domain.KindSprintTask] = true }, "Трекер задач недоступен"},
			{"бриф не сохраняется", func(s *flakyStore) { s.failPut[domain.KindBrief] = true }, "Не удалось отправить бриф"},
			{"нет профиля", func(s *flakyStore) { s.failGet[domain.KindIntern] = true }, ""},
			{"нет прогресса документации", func(s *flakyStore) { s.failList[domain.KindDocRead] = true }, ""},
			{"нет доступов", func(s *flakyStore) { s.failList[domain.KindAccessGrant] = true }, ""},
			{"нет отсутствий", func(s *flakyStore) { s.failList[domain.KindAbsence] = true }, ""},
			{"нет открытых вопросов", func(s *flakyStore) { s.failList[domain.KindOpenQuestion] = true }, ""},
			{"нет брифов", func(s *flakyStore) { s.failList[domain.KindBrief] = true }, ""},
			{"нет чек-листа", func(s *flakyStore) { s.failList[domain.KindChecklist] = true }, ""},
			{"сообщение не сохраняется", func(s *flakyStore) { s.failPut[domain.KindMessage] = true }, ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				e := newEnv(t, nil)
				tc.break_(e.store)
				res, err := e.svc.CronTrigger(e.ctx, e.intern)
				if tc.reason == "" {
					if !errors.Is(err, errStore) {
						t.Fatalf("ожидалась ошибка хранилища: %+v, %v", res, err)
					}
					return
				}
				if err != nil || res.Triggered || !strings.Contains(res.Reason, tc.reason) {
					t.Fatalf("%+v, %v", res, err)
				}
			})
		}
	})
}

// S3: объяснение порядка → карточка → подтверждение → заявка #ABS-2026-84.
func TestScenario3_DayOffThroughAbsencesService(t *testing.T) {
	e := newEnv(t, nil)
	res := e.chat(askAbsence)

	card := res.Message.ActionCard
	if card == nil || card.Type != domain.ActionAbsenceForm || card.Tool != mcp.ToolCreateDayOff || card.Status != domain.ActionStatusPending {
		t.Fatalf("карточка действия: %+v", card)
	}
	if card.Fields["date"] != "2026-11-13" || card.Fields["time_from"] != "14:00" {
		t.Fatalf("поля: %v", card.Fields)
	}
	if !strings.Contains(res.Message.Text, "писать руководителю не нужно") {
		t.Fatalf("ответ должен объяснять порядок: %s", res.Message.Text)
	}
	if len(e.workspace(e.intern).Absences) != 0 || item(t, res.Checklist, "absence_rules").Status != domain.StatusPending {
		t.Fatal("до подтверждения заявка не создаётся и шаг не закрывается")
	}

	// Подтверждение кнопкой в интерфейсе → POST /api/v1/mcp/execute.
	done, err := e.svc.Execute(e.ctx, e.intern, card.Tool, card.Fields, res.Message.ID)
	if err != nil {
		t.Fatal(err)
	}
	call := done.ToolCalls[0]
	if !call.OK() || call.Result["request_number"] != "#ABS-2026-84" || done.RBAC.PolicyID != "POL-ABS-001" {
		t.Fatalf("вызов absences.create_day_off: %+v", call)
	}
	msg := done.Message
	if msg == nil || msg.ResultCard == nil || msg.ResultCard.Type != domain.CardAbsenceCreated || msg.ResultCard.Lines[0].Value != "#ABS-2026-84" {
		t.Fatalf("сообщение с результатом: %+v", msg)
	}
	for _, want := range []string{"#ABS-2026-84", "пт, 13.11, 14:00–18:00", "Мария Кузнецова, Дмитрий Орлов", "«Разобраться с оформлением отсутствий» — выполнен"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("в подтверждении нет %q: %s", want, msg.Text)
		}
	}
	if got := updated(done.ChecklistUpdates); got != "absence_rules=completed" {
		t.Fatalf("обновления чек-листа: %s", got)
	}

	// Заявка в БД, уведомления наставнику и руководителю, карточка помечена отправленной.
	ws := e.workspace(e.intern)
	if len(ws.Absences) != 1 || ws.Absences[0].Number != "#ABS-2026-84" || ws.Absences[0].Status != domain.AbsenceStatusPending {
		t.Fatalf("заявка: %+v", ws.Absences)
	}
	for _, m := range ws.Messages {
		if m.ID == res.Message.ID && m.ActionCard.Status != domain.ActionStatusDone {
			t.Fatalf("карточка должна быть помечена отправленной: %+v", m.ActionCard)
		}
	}
	for name, p := range map[string]domain.Principal{"наставник": e.mentor, "руководитель": e.lead} {
		v := e.workspace(p)
		if len(v.Notifications) != 1 || v.Notifications[0].Kind != "absence" || !strings.Contains(v.Notifications[0].Body, "#ABS-2026-84") {
			t.Errorf("%s: уведомление: %+v", name, v.Notifications)
		}
		if len(v.Absences) != 1 {
			t.Errorf("%s должен видеть заявку команды", name)
		}
	}
	if ws.Metrics.ActionsAutomated != 1 || ws.Metrics.QuestionsAnswered != 1 || ws.Metrics.MentorMinutesSaved != 20 {
		t.Fatalf("метрики: %+v", ws.Metrics)
	}
	if got := e.events("ui.action_confirmed"); len(got) != 1 || got[0].Payload["tool"] != mcp.ToolCreateDayOff {
		t.Fatalf("clickstream: %+v", got)
	}

	// Повторная отправка: дубликат, сообщение в чат не добавляется.
	dup, err := e.svc.Execute(e.ctx, e.intern, card.Tool, card.Fields, res.Message.ID)
	if err != nil || dup.ToolCalls[0].ErrorCode != mcp.CodeDuplicate || dup.Message != nil || len(dup.Checklist) == 0 {
		t.Fatalf("дубликат: %+v, %v", dup.ToolCalls[0], err)
	}
}

func TestExecuteEnforcesRBACAndValidation(t *testing.T) {
	e := newEnv(t, nil)
	fields := map[string]any{"intern_id": seed.InternID, "date": "2026-11-13", "reason": "Зачёт в университете"}

	// Наставник и руководитель не могут оформить отгул за стажёра.
	for _, p := range []domain.Principal{e.mentor, e.lead} {
		res, err := e.svc.Execute(e.ctx, p, mcp.ToolCreateDayOff, fields, "")
		if err != nil || res.ToolCalls[0].Status != domain.ToolForbidden || res.RBAC.Allowed || res.Message != nil {
			t.Fatalf("%s: %+v, %v", p.Role, res.ToolCalls[0], err)
		}
	}
	// Стажёр не может оформить отгул за другого и выгрузить ПДн напрямую.
	other := map[string]any{"intern_id": "u-intern-99", "date": "2026-11-13", "reason": "Зачёт в университете"}
	if res, _ := e.svc.Execute(e.ctx, e.intern, mcp.ToolCreateDayOff, other, ""); res.RBAC.Constraint != "self_only" {
		t.Fatalf("self_only: %+v", res.RBAC)
	}
	if res, _ := e.svc.Execute(e.ctx, e.intern, mcp.ToolExportProdPII, map[string]any{"dataset": "payroll"}, ""); res.ToolCalls[0].Status != domain.ToolForbidden {
		t.Fatalf("прямой вызов выгрузки: %+v", res.ToolCalls[0])
	}
	if len(e.workspace(e.intern).Absences) != 0 {
		t.Fatal("запрещённые вызовы не должны создавать заявок")
	}

	// Невалидные аргументы и пустое имя инструмента.
	bad := map[string]any{"intern_id": seed.InternID, "date": "завтра", "reason": "Зачёт"}
	if res, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolCreateDayOff, bad, ""); err != nil || res.ToolCalls[0].Status != domain.ToolInvalid {
		t.Fatalf("невалидные аргументы: %+v, %v", res.ToolCalls[0], err)
	}
	if _, err := e.svc.Execute(e.ctx, e.intern, "  ", nil, ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("пустой инструмент: %v", err)
	}
	if res, err := e.svc.Execute(e.ctx, e.intern, "ghost.tool", nil, ""); err != nil || res.ToolCalls[0].ErrorCode != mcp.CodeUnknownTool {
		t.Fatalf("неизвестный инструмент: %+v, %v", res.ToolCalls[0], err)
	}
}

func TestExecuteOtherTools(t *testing.T) {
	e := newEnv(t, nil)
	// Прямой вызов IAM из интерфейса тоже закрывает шаг чек-листа.
	res, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolRequestRepoAccess, map[string]any{"intern_id": seed.InternID, "repository": seed.TeamRepo}, "нет-такого-сообщения")
	if err != nil || res.Message == nil || res.Message.ResultCard.Type != domain.CardAccessGranted || updated(res.ChecklistUpdates) != "access_repo=completed" {
		t.Fatalf("IAM: %+v, %v", res.Message, err)
	}
	// Инструмент без специальной карточки.
	res, err = e.svc.Execute(e.ctx, e.intern, mcp.ToolListFlags, map[string]any{}, "")
	if err != nil || res.Message == nil || res.Message.ResultCard.Type != "tool_result" || !strings.Contains(res.Message.Text, mcp.ToolListFlags) {
		t.Fatalf("generic: %+v, %v", res.Message, err)
	}
	// Наставник запрашивает доступ стажёру: действие выполняется, но в чат стажёра не пишет.
	e2 := newEnv(t, nil)
	res, err = e2.svc.Execute(e2.ctx, e2.mentor, mcp.ToolRequestRepoAccess, map[string]any{"intern_id": seed.InternID, "repository": seed.TeamRepo}, "")
	if err != nil || !res.ToolCalls[0].OK() || res.Message != nil || updated(res.ChecklistUpdates) != "access_repo=completed" {
		t.Fatalf("наставник: %+v, %v", res, err)
	}
	if n := len(e2.workspace(e2.intern).Messages); n != 1 {
		t.Fatalf("в чате стажёра должно остаться только приветствие: %d", n)
	}
}

func TestChatValidationAndAuthorization(t *testing.T) {
	e := newEnv(t, nil)
	for _, text := range []string{"", "   ", strings.Repeat("я", 2001)} {
		if _, err := e.svc.Chat(e.ctx, e.intern, text, nil); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("сообщение длиной %d: ожидался ErrValidation, получено %v", len([]rune(text)), err)
		}
	}
	if _, err := e.svc.Chat(e.ctx, e.intern, strings.Repeat("я", 2000), nil); err != nil {
		t.Errorf("сообщение на границе лимита: %v", err)
	}
	// Чат — инструмент стажёра: наставник и руководитель получают FORBIDDEN.
	for _, p := range []domain.Principal{e.mentor, e.lead} {
		_, err := e.svc.Chat(e.ctx, p, "привет", nil)
		v, ok := isForbidden(err)
		if !ok || v.Operation != usecase.OpChatSend || v.HTTPStatus != 403 || !strings.Contains(err.Error(), "FORBIDDEN") {
			t.Fatalf("%s: %v", p.Role, err)
		}
	}
	denied := 0
	for _, ev := range e.events("rbac.decision") {
		if ev.Payload["allowed"] == false && ev.Payload["operation"] == usecase.OpChatSend {
			denied++
		}
	}
	if denied != 2 {
		t.Fatalf("отказы API должны журналироваться: %d", denied)
	}
	// Неизвестная роль не имеет доступа ни к чему.
	ghost := domain.Principal{ID: "x", Role: "contractor"}
	if _, err := e.svc.Workspace(e.ctx, ghost); err == nil {
		t.Fatal("workspace для неизвестной роли")
	}
}

func TestStagesAreEmittedForWaitingStatus(t *testing.T) {
	e := newEnv(t, nil)
	var kinds []string
	if _, err := e.svc.Chat(e.ctx, e.intern, askAccess, func(s agent.Stage) { kinds = append(kinds, s.Kind) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(kinds, ","); got != "understanding,rbac,tool,composing" {
		t.Fatalf("этапы: %s", got)
	}
}

func TestUnansweredQuestionGoesToMentorAgenda(t *testing.T) {
	e := newEnv(t, nil)
	res := e.chat("Как заказать пропуск для гостя?")
	if !strings.Contains(res.Message.Text, "не нашёл") || len(res.Message.Sources) != 0 {
		t.Fatalf("ответ: %s", res.Message.Text)
	}
	m := e.workspace(e.intern).Metrics
	if m.UnansweredQuestions != 1 || m.QuestionsAnswered != 0 || m.MentorMinutesSaved != 0 {
		t.Fatalf("метрики: %+v", m)
	}
}

type failingLLM struct{}

func (failingLLM) Model() string { return "broken-model" }
func (failingLLM) Complete(context.Context, agent.LLMRequest) (agent.LLMResponse, error) {
	return agent.LLMResponse{}, errors.New("llm: HTTP 503")
}

// Недоступная LLM не мешает пройти сценарий: Smart Engine + событие llm.fallback.
func TestLLMOutageFallsBackSeamlessly(t *testing.T) {
	e := newEnv(t, failingLLM{})
	if ws := e.workspace(e.intern); ws.Engine.Mode != domain.EngineLLM || ws.Engine.Model != "broken-model" {
		t.Fatalf("режим: %+v", ws.Engine)
	}
	res := e.chat(askAccess)
	if res.Message.Engine != domain.EngineSmart || updated(res.ChecklistUpdates) != "access_repo=completed" {
		t.Fatalf("сценарий должен пройти на Smart Engine: %+v", res.Message)
	}
	fb := e.events("llm.fallback")
	if len(fb) != 1 || fb[0].Severity != domain.SeverityWarn || !strings.Contains(fb[0].Payload["reason"].(string), "503") {
		t.Fatalf("событие llm.fallback: %+v", fb)
	}
}

func TestWorkspaceByRole(t *testing.T) {
	e := newEnv(t, nil)
	cases := []struct {
		p           domain.Principal
		title       string
		messages    int
		permissions map[string]bool
	}{
		{e.intern, "Стажёр (Backend)", 1, map[string]bool{"chat": true, "cron": true, "reset": true, "telemetry": true, "flags_write": false, "briefs": false, "incidents": false}},
		{e.mentor, "Наставник (buddy)", 0, map[string]bool{"chat": false, "cron": true, "reset": true, "telemetry": true, "flags_write": false, "briefs": true, "incidents": false}},
		{e.lead, "Руководитель команды", 0, map[string]bool{"chat": false, "cron": true, "reset": true, "telemetry": true, "flags_write": true, "briefs": true, "incidents": true}},
	}
	for _, tc := range cases {
		t.Run(string(tc.p.Role), func(t *testing.T) {
			ws := e.workspace(tc.p)
			if ws.Viewer.ID != tc.p.ID || ws.Viewer.RoleTitle != tc.title || ws.Viewer.Team != "Autoteka" {
				t.Fatalf("viewer: %+v", ws.Viewer)
			}
			if len(ws.Messages) != tc.messages {
				t.Fatalf("сообщений %d, want %d (переписку видит только стажёр)", len(ws.Messages), tc.messages)
			}
			for key, want := range tc.permissions {
				if ws.Permissions[key] != want {
					t.Errorf("permissions[%s] = %v, want %v", key, ws.Permissions[key], want)
				}
			}
			if ws.Intern.ID != seed.InternID || len(ws.Checklist) != 7 || ws.Progress.Completed != 2 || ws.NextStep.ID != "access_repo" {
				t.Fatalf("состояние стажёра: %+v", ws.Progress)
			}
			if ws.SprintTask.Key != "AUT-1423" || len(ws.FeatureFlags) != 10 || len(ws.Skills) != 5 || ws.Storage.Store != "memory" || ws.Storage.Cache != "memory" {
				t.Fatalf("задача/флаги/хранилище: %+v", ws.Storage)
			}
			if ws.Engine.Mode != domain.EngineSmart || ws.Metrics.MentorHoursBaselineMin != 40 || ws.Metrics.MentorHoursBaselineMax != 48 {
				t.Fatalf("engine/metrics: %+v", ws.Engine)
			}
			// Пустые коллекции сериализуются как [], а не null (контракт для Zod-схем фронтенда).
			if ws.Briefs == nil || ws.Absences == nil || ws.AccessGrants == nil || ws.Incidents == nil || ws.Notifications == nil || ws.DocProgress.Read == nil {
				t.Fatal("коллекции не должны быть nil")
			}
		})
	}
	// Приветственное сообщение с подсказками трёх сценариев.
	welcome := e.workspace(e.intern).Messages[0]
	if welcome.Role != domain.MessageAssistant || len(welcome.Suggestions) != 3 || !strings.Contains(welcome.Text, "Привет, Алексей") {
		t.Fatalf("приветствие: %+v", welcome)
	}
}

func TestResetRestoresDemoState(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askAccess)
	e.chat(askPII)
	if _, err := e.svc.SetFlag(e.ctx, e.lead, mcp.FlagProactiveCron, false); err != nil {
		t.Fatal(err)
	}

	ws, err := e.svc.Reset(e.ctx, e.intern)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Progress.Completed != 2 || len(ws.Messages) != 1 || len(ws.AccessGrants) != 0 || ws.Metrics.RBACBlocks != 0 || ws.Metrics.MentorMinutesSaved != 0 {
		t.Fatalf("состояние после сброса: %+v / %+v", ws.Progress, ws.Metrics)
	}
	for _, f := range ws.FeatureFlags {
		if f.Key == mcp.FlagProactiveCron && !f.Enabled {
			t.Fatal("сброс должен вернуть флаги к значениям по умолчанию")
		}
	}
	if _, ok, _ := e.cache.Get(e.ctx, "session:"+seed.InternID); ok {
		t.Fatal("контекст сессии должен очищаться")
	}
	if len(e.workspace(e.lead).Incidents) != 0 {
		t.Fatal("инциденты должны очищаться")
	}
	if got := e.events("demo.reset"); len(got) != 1 {
		t.Fatalf("событие demo.reset: %d", len(got))
	}
	// Сценарий воспроизводим после сброса: номер заявки снова #ABS-2026-84.
	card := e.chat(askAbsence).Message.ActionCard
	done, err := e.svc.Execute(e.ctx, e.intern, card.Tool, card.Fields, "")
	if err != nil || done.ToolCalls[0].Result["request_number"] != "#ABS-2026-84" {
		t.Fatalf("повторный прогон после сброса: %+v, %v", done.ToolCalls[0], err)
	}

	e.store.failReset = true
	if _, err := e.svc.Reset(e.ctx, e.intern); !errors.Is(err, errStore) {
		t.Fatalf("сбой сброса: %v", err)
	}
	if _, err := e.svc.Reset(e.ctx, domain.Principal{ID: "x", Role: "contractor"}); err == nil {
		t.Fatal("сброс для неизвестной роли")
	}
}

func TestEnsureSeedIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askAccess)
	if err := e.svc.EnsureSeed(e.ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.workspace(e.intern).AccessGrants) != 1 {
		t.Fatal("повторный EnsureSeed не должен затирать данные (перезапуск бэкенда с PostgreSQL)")
	}
	e.store.failGet[domain.KindIntern] = true
	if err := e.svc.EnsureSeed(e.ctx); !errors.Is(err, errStore) {
		t.Fatalf("сбой чтения: %v", err)
	}
}

func TestTelemetryView(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askAccess)
	e.chat(askPII)

	v, err := e.svc.Telemetry(e.ctx, e.intern)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.ToolCalls) != 2 || v.ToolCalls[0].Tool != mcp.ToolExportProdPII {
		t.Fatalf("вызовы должны идти от новых к старым: %+v", v.ToolCalls)
	}
	if len(v.Events) < 8 || v.Events[0].Name != "agent.answer" {
		t.Fatalf("события: %d, первое %s", len(v.Events), v.Events[0].Name)
	}
	kinds := map[string]bool{}
	for _, ev := range v.Events {
		kinds[ev.Kind] = true
	}
	for _, k := range []string{domain.EventKindMCP, domain.EventKindRBAC, domain.EventKindClickstream, domain.EventKindAgent} {
		if !kinds[k] {
			t.Errorf("в инспекторе нет событий вида %s", k)
		}
	}
	allowed := map[string]bool{}
	for _, tool := range v.Tools {
		allowed[tool.Name] = tool.AllowedForViewer
	}
	if len(v.Tools) != 9 || !allowed[mcp.ToolCreateDayOff] || allowed[mcp.ToolExportProdPII] || allowed[mcp.ToolSendBrief] || allowed[mcp.ToolReviewDayOff] {
		t.Fatalf("доступность инструментов для стажёра: %v", allowed)
	}
	if len(v.Matrix) == 0 || len(v.Roles) != 4 || len(v.Servers) != 8 {
		t.Fatalf("матрица/роли/серверы: %d/%d/%d", len(v.Matrix), len(v.Roles), len(v.Servers))
	}
	if _, err := e.svc.Telemetry(e.ctx, domain.Principal{ID: "x", Role: domain.RoleSystemCron}); err == nil {
		t.Fatal("сервисному аккаунту инспектор недоступен")
	}
	for _, kind := range []domain.Kind{domain.KindEvent, domain.KindToolCall} {
		e.store.heal()
		e.store.failList[kind] = true
		if _, err := e.svc.Telemetry(e.ctx, e.intern); !errors.Is(err, errStore) {
			t.Fatalf("сбой %s: %v", kind, err)
		}
	}
}

func TestSetFlag(t *testing.T) {
	e := newEnv(t, nil)
	flag, err := e.svc.SetFlag(e.ctx, e.lead, mcp.FlagAbsenceCard, false)
	if err != nil || flag.Enabled {
		t.Fatalf("%+v, %v", flag, err)
	}
	if got := e.events("feature_flag.changed"); len(got) != 1 || got[0].Payload["enabled"] != false {
		t.Fatalf("clickstream: %+v", got)
	}
	// Флаг реально меняет поведение: карточка отгула не показывается.
	if res := e.chat(askAbsence); res.Message.ActionCard != nil {
		t.Fatal("при выключенном флаге карточки быть не должно")
	}
	for _, p := range []domain.Principal{e.intern, e.mentor} {
		if _, err := e.svc.SetFlag(e.ctx, p, mcp.FlagAbsenceCard, true); err == nil {
			t.Errorf("%s не должен переключать флаги", p.Role)
		} else if _, ok := isForbidden(err); !ok {
			t.Errorf("%s: %v", p.Role, err)
		}
	}
	if _, err := e.svc.SetFlag(e.ctx, e.lead, "ai_buddy.unknown", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("неизвестный флаг: %v", err)
	}
	if _, err := e.svc.SetFlag(e.ctx, e.lead, "ai_buddy.mcp_jsonrpc", false); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("compile-time флаг: %v", err)
	}
}

func TestTrackUI(t *testing.T) {
	e := newEnv(t, nil)
	payload := map[string]any{"role": "mentor", "long": strings.Repeat("я", 300), "n": 3.0, "ok": true, "nested": map[string]any{"x": 1}}
	ev, err := e.svc.TrackUI(e.ctx, e.mentor, "ui.role_switched", payload)
	if err != nil || ev.Kind != domain.EventKindClickstream || ev.ActorRole != domain.RoleMentor {
		t.Fatalf("%+v, %v", ev, err)
	}
	if _, has := ev.Payload["nested"]; has || len([]rune(ev.Payload["long"].(string))) != 200 || ev.Payload["n"] != 3.0 || ev.Payload["ok"] != true {
		t.Fatalf("payload должен очищаться: %v", ev.Payload)
	}
	big := map[string]any{}
	for i := 0; i < 30; i++ {
		big[strings.Repeat("k", i+1)] = "v"
	}
	if ev, _ := e.svc.TrackUI(e.ctx, e.intern, "ui.big", big); len(ev.Payload) != 10 {
		t.Fatalf("payload ограничен 10 полями: %d", len(ev.Payload))
	}
	for _, name := range []string{"", "chat.sent", "ui.", "ui.Bad Name", "rbac.incident"} {
		if _, err := e.svc.TrackUI(e.ctx, e.intern, name, nil); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("имя %q должно отклоняться: %v", name, err)
		}
	}
}

func TestHandleMCPAndHealth(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.svc.HandleMCP(e.ctx, e.intern, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/list: %+v", resp)
	}
	info, err := e.svc.Health(e.ctx)
	if err != nil || info.Store != "memory" || info.Cache != "memory" {
		t.Fatalf("health: %+v, %v", info, err)
	}
}

func TestStoreFailuresSurfaceAsErrors(t *testing.T) {
	type breaker func(*flakyStore)
	get := func(k domain.Kind) breaker { return func(s *flakyStore) { s.failGet[k] = true } }
	list := func(k domain.Kind) breaker { return func(s *flakyStore) { s.failList[k] = true } }
	put := func(k domain.Kind) breaker { return func(s *flakyStore) { s.failPut[k] = true } }

	t.Run("chat", func(t *testing.T) {
		for name, br := range map[string]breaker{
			"профиль": get(domain.KindIntern), "чек-лист": list(domain.KindChecklist), "задача": get(domain.KindSprintTask),
			"сообщение": put(domain.KindMessage), "прогресс документации": put(domain.KindDocRead),
			"список прочитанного": list(domain.KindDocRead), "шаг чек-листа": put(domain.KindChecklist),
		} {
			t.Run(name, func(t *testing.T) {
				e := newEnv(t, nil)
				br(e.store)
				if _, err := e.svc.Chat(e.ctx, e.intern, askProcess, nil); !errors.Is(err, errStore) {
					t.Fatalf("ожидалась ошибка хранилища: %v", err)
				}
			})
		}
		e := newEnv(t, nil)
		e.store.failPut[domain.KindOpenQuestion] = true
		if _, err := e.svc.Chat(e.ctx, e.intern, "Как заказать пропуск для гостя?", nil); !errors.Is(err, errStore) {
			t.Fatalf("сохранение открытого вопроса: %v", err)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		e := newEnv(t, nil)
		for name, br := range map[string]breaker{
			"профиль": get(domain.KindIntern), "чек-лист": list(domain.KindChecklist), "задача": get(domain.KindSprintTask),
			"события": list(domain.KindEvent), "прочитанное": list(domain.KindDocRead), "вопросы": list(domain.KindOpenQuestion),
			"уведомления": list(domain.KindNotification), "сообщения": list(domain.KindMessage),
			"отсутствия": list(domain.KindAbsence), "доступы": list(domain.KindAccessGrant),
		} {
			e.store.heal()
			br(e.store)
			if _, err := e.svc.Workspace(e.ctx, e.intern); !errors.Is(err, errStore) {
				t.Errorf("%s: ожидалась ошибка хранилища: %v", name, err)
			}
		}
		e.store.heal()
		e.store.failList[domain.KindBrief] = true
		if _, err := e.svc.Workspace(e.ctx, e.mentor); !errors.Is(err, errStore) {
			t.Errorf("брифы: %v", err)
		}
	})

	t.Run("execute", func(t *testing.T) {
		fields := map[string]any{"intern_id": seed.InternID, "date": "2026-11-13", "reason": "Зачёт в университете"}
		for name, br := range map[string]breaker{
			"чек-лист": list(domain.KindChecklist), "шаг чек-листа": put(domain.KindChecklist), "сообщение": put(domain.KindMessage),
		} {
			t.Run(name, func(t *testing.T) {
				e := newEnv(t, nil)
				br(e.store)
				if _, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolCreateDayOff, fields, ""); !errors.Is(err, errStore) {
					t.Fatalf("ожидалась ошибка хранилища: %v", err)
				}
			})
		}
	})
}

func TestReviewAbsenceFlow(t *testing.T) {
	e := newEnv(t, nil)
	fields := map[string]any{"intern_id": seed.InternID, "date": "2026-11-20", "reason": "Зачёт"}
	_, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolCreateDayOff, fields, "")
	if err != nil {
		t.Fatalf("создание отгула: %v", err)
	}

	all, err := domain.Load[domain.AbsenceRequest](e.ctx, e.store, domain.KindAbsence, "")
	if err != nil || len(all) == 0 {
		t.Fatalf("нет сохранённых отсутствий: %v", err)
	}
	abs := all[0]

	// Intern cannot review
	if _, err := e.svc.ReviewAbsence(e.ctx, e.intern, abs.ID, "approved", ""); err == nil {
		t.Fatal("стажёр не может согласовать отгул")
	}

	// Lead invalid decision
	if _, err := e.svc.ReviewAbsence(e.ctx, e.lead, abs.ID, "unknown", ""); err == nil {
		t.Fatal("невалидное решение должно давать ошибку")
	}

	// Lead approves
	updated, err := e.svc.ReviewAbsence(e.ctx, e.lead, abs.ID, "approved", "Отлично, согласовано")
	if err != nil || updated.Status != "approved" {
		t.Fatalf("согласование: %v, %+v", err, updated)
	}

	// Check clickstream
	events := e.events("absence.reviewed")
	if len(events) != 1 || events[0].Payload["decision"] != "approved" {
		t.Fatalf("clickstream absence.reviewed: %+v", events)
	}

	// Check chat message added
	msgs, err := domain.Load[domain.ChatMessage](e.ctx, e.store, domain.KindMessage, seed.InternID)
	if err != nil {
		t.Fatalf("сообщения чата: %v", err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Text, "Руководитель согласовал твою заявку на отгул") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("системное уведомление не добавлено в чат стажёра")
	}

	// Lead rejects another request
	fields2 := map[string]any{"intern_id": seed.InternID, "date": "2026-11-21", "reason": "Конференция"}
	_, err = e.svc.Execute(e.ctx, e.intern, mcp.ToolCreateDayOff, fields2, "")
	if err != nil {
		t.Fatalf("создание второго отгула: %v", err)
	}
	all, _ = domain.Load[domain.AbsenceRequest](e.ctx, e.store, domain.KindAbsence, "")
	abs2 := all[1]
	rej, err := e.svc.ReviewAbsence(e.ctx, e.lead, abs2.ID, "rejected", "В этот день релиз")
	if err != nil || rej.Status != "rejected" {
		t.Fatalf("отклонение: %v, %+v", err, rej)
	}
}
