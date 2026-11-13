package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/seed"
)

func accessArgs(repo string) map[string]any {
	return map[string]any{"intern_id": seed.InternID, "repository": repo}
}

// Сценарий 1А на уровне инструмента.
func TestIAMRequestRepoAccess(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.intern(t), ToolRequestRepoAccess, accessArgs(seed.TeamRepo))
	if !call.OK() || call.RBAC.PolicyID != "POL-IAM-001" {
		t.Fatalf("доступ к репозиторию своей команды: %+v", call)
	}
	r := call.Result
	token, _ := r["token_masked"].(string)
	if r["status"] != "granted" || r["repository"] != seed.TeamRepo || r["access_level"] != "developer" ||
		r["stand"] != "autoteka-test-07" || r["is_synthetic"] != true || r["already_granted"] != false {
		t.Fatalf("результат: %v", r)
	}
	if !strings.HasPrefix(token, "glpat-synt-••••") || len([]rune(token)) != len("glpat-synt-")+8 {
		t.Fatalf("токен должен быть маскирован: %q", token)
	}
	grants, _ := domain.Load[domain.AccessGrant](context.Background(), f.store.Store, domain.KindAccessGrant, seed.InternID)
	if len(grants) != 1 || grants[0].TokenMasked != token || !grants[0].ExpiresAt.Equal(testNow.AddDate(0, 0, 90)) {
		t.Fatalf("запись о доступе в БД: %+v", grants)
	}
	if raw, _ := json.Marshal(grants[0]); strings.Count(string(raw), "glpat-synt-") != 1 || !strings.Contains(string(raw), "••••") {
		t.Fatalf("сырой токен не должен сохраняться: %s", raw)
	}

	// Повторный запрос идемпотентен: новый грант не создаётся.
	again := f.exec(f.intern(t), ToolRequestRepoAccess, accessArgs(seed.TeamRepo))
	if !again.OK() || again.Result["already_granted"] != true || again.Result["grant_id"] == "" {
		t.Fatalf("повторный запрос: %+v", again)
	}
	if grants, _ := domain.Load[domain.AccessGrant](context.Background(), f.store.Store, domain.KindAccessGrant, seed.InternID); len(grants) != 1 {
		t.Fatalf("дубликат гранта: %d", len(grants))
	}
}

func TestIAMVariants(t *testing.T) {
	cases := []struct {
		name       string
		who        string
		args       map[string]any
		wantStatus domain.ToolStatus
		check      func(*testing.T, domain.MCPToolCall)
	}{
		{"staging-стенд и уровень read", seed.InternID,
			map[string]any{"intern_id": seed.InternID, "repository": "avito-autoteka-reports", "stand": "staging", "access_level": "read"},
			domain.ToolSuccess, func(t *testing.T, c domain.MCPToolCall) {
				if c.Result["stand"] != "autoteka-staging-02" || c.Result["access_level"] != "read" {
					t.Errorf("результат: %v", c.Result)
				}
			}},
		{"без стенда", seed.InternID,
			map[string]any{"intern_id": seed.InternID, "repository": seed.TeamRepo, "stand": "none"},
			domain.ToolSuccess, func(t *testing.T, c domain.MCPToolCall) {
				if c.Result["stand"] != "" {
					t.Errorf("стенд не запрашивался: %v", c.Result["stand"])
				}
			}},
		{"репозиторий чужой команды", seed.InternID, accessArgs("avito-payments-core"),
			domain.ToolForbidden, func(t *testing.T, c domain.MCPToolCall) {
				if c.RBAC.Constraint != "team_scope" {
					t.Errorf("ожидался отказ по team_scope: %+v", c.RBAC)
				}
			}},
		{"несуществующий репозиторий неотличим от чужого", seed.InternID, accessArgs("avito-secret-vault"),
			domain.ToolForbidden, func(t *testing.T, c domain.MCPToolCall) {
				if c.RBAC.Constraint != "team_scope" {
					t.Errorf("%+v", c.RBAC)
				}
			}},
		{"доступ за другого сотрудника", seed.InternID,
			map[string]any{"intern_id": "u-intern-99", "repository": seed.TeamRepo},
			domain.ToolForbidden, func(t *testing.T, c domain.MCPToolCall) {
				if c.RBAC.Constraint != "self_only" {
					t.Errorf("%+v", c.RBAC)
				}
			}},
		{"наставник запрашивает стажёру", seed.MentorID, accessArgs(seed.TeamRepo), domain.ToolSuccess,
			func(t *testing.T, c domain.MCPToolCall) {
				if c.RBAC.PolicyID != "POL-IAM-002" {
					t.Errorf("%+v", c.RBAC)
				}
			}},
		{"наставник: неизвестный сотрудник", seed.MentorID,
			map[string]any{"intern_id": "u-ghost", "repository": seed.TeamRepo},
			domain.ToolInvalid, func(t *testing.T, c domain.MCPToolCall) {
				if !strings.Contains(c.Error, "не найден") {
					t.Errorf("ошибка: %s", c.Error)
				}
			}},
		{"некорректное имя репозитория", seed.MentorID,
			map[string]any{"intern_id": seed.InternID, "repository": "Bad Name!"},
			domain.ToolForbidden, nil}, // неизвестный репозиторий → нет команды → team_scope
		{"неизвестный стенд", seed.InternID,
			map[string]any{"intern_id": seed.InternID, "repository": seed.TeamRepo, "stand": "prod"},
			domain.ToolInvalid, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			call := f.exec(f.principal(t, tc.who), ToolRequestRepoAccess, tc.args)
			if call.Status != tc.wantStatus {
				t.Fatalf("status = %s (%s %s), want %s", call.Status, call.ErrorCode, call.Error, tc.wantStatus)
			}
			if tc.check != nil {
				tc.check(t, call)
			}
		})
	}
}

func TestIAMStoreFailures(t *testing.T) {
	for _, kind := range []string{"get-intern", "list-grants", "put-grant"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			switch kind {
			case "get-intern":
				f.store.failGet[domain.KindIntern] = true
			case "list-grants":
				f.store.failList[domain.KindAccessGrant] = true
			case "put-grant":
				f.store.failPut[domain.KindAccessGrant] = true
			}
			call := f.exec(f.intern(t), ToolRequestRepoAccess, accessArgs(seed.TeamRepo))
			if call.Status != domain.ToolError || call.ErrorCode != CodeInternal {
				t.Fatalf("сбой хранилища должен давать INTERNAL: %+v", call)
			}
		})
	}
}

func TestPlaybookSearch(t *testing.T) {
	f := newFixture(t)
	pb := f.services.Playbook
	ids := func(docs []domain.KnowledgeDoc) string {
		var out []string
		for _, d := range docs {
			out = append(out, d.ID)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name   string
		role   domain.Role
		team   string
		query  string
		topics []string
		limit  int
		want   string
	}{
		{"сценарий 2: флаги и ревью", domain.RoleInternBackend, "Autoteka",
			"Как в нашей команде катить задачу под фича-флагом и проходить ревью?",
			[]string{"feature_flags", "code_review"}, 3, "pb-feature-flags,pb-code-review"},
		{"тема задана, вопрос только про ревью", domain.RoleInternBackend, "Autoteka",
			"как проходит ревью", []string{"feature_flags", "code_review"}, 3, "pb-code-review"},
		{"тема задана, слова не совпали — все документы темы", domain.RoleInternBackend, "Autoteka",
			"расскажи подробнее", []string{"absences"}, 3, "pb-absences"},
		{"свободный поиск", domain.RoleInternBackend, "Autoteka", "заболел, нужен больничный", nil, 0, "pb-absences"},
		{"слабые совпадения отсекаются", domain.RoleInternBackend, "Autoteka", "доступ к репозиторию и стенду для задачи", nil, 3, "pb-access"},
		{"лимит", domain.RoleInternBackend, "Autoteka", "катить фича-флаг ревью", []string{"feature_flags", "code_review"}, 1, "pb-feature-flags"},
		{"ничего не найдено", domain.RoleInternBackend, "Autoteka", "как заказать пропуск для гостя", nil, 3, ""},
		{"закрытый документ не виден стажёру", domain.RoleInternBackend, "Autoteka", "оценка компетенций на экваторе", nil, 3, ""},
		{"закрытый документ виден наставнику", domain.RoleMentor, "Autoteka", "оценка компетенций на экваторе", nil, 3, "pb-mentor-guide"},
		{"чужая команда не видит базу знаний", domain.RoleInternBackend, "Payments", "фича-флаг", nil, 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(pb.Search(tc.role, tc.team, tc.query, tc.topics, tc.limit)); got != tc.want {
				t.Fatalf("Search = %q, want %q", got, tc.want)
			}
		})
	}
	autotekaTotal := 0
	for _, d := range f.seed.Docs {
		if d.Team == "Autoteka" {
			autotekaTotal++
		}
	}
	if n := pb.VisibleCount(domain.RoleInternBackend, "Autoteka"); n != autotekaTotal-1 {
		t.Fatalf("стажёру видно %d документов из %d, ожидалось %d", n, autotekaTotal, autotekaTotal-1)
	}
	if pb.VisibleCount(domain.RoleMentor, "Autoteka") != autotekaTotal {
		t.Fatal("наставнику должны быть видны все документы команды")
	}
	if n := pb.VisibleCount(domain.RoleInternBackend, seed.Team2); n != 2 {
		t.Fatalf("ML-стажёру видно %d документов, ожидалось 2", n)
	}
	if d, ok := pb.Doc("pb-absences"); !ok || d.Title == "" {
		t.Fatal("Doc")
	}
	if _, ok := pb.Doc("нет"); ok {
		t.Fatal("Doc для несуществующего id")
	}
}

func TestPlaybookSearchTool(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.intern(t), ToolSearchDocs, map[string]any{
		"query": "как катить под фича-флагом", "team": "Payments", // стажёр не может выбрать чужую команду
		"topics": []any{"feature_flags"}, "limit": 2,
	})
	if !call.OK() {
		t.Fatalf("%+v", call)
	}
	refs := DocRefs(call.Result)
	if len(refs) != 1 || refs[0].ID != "pb-feature-flags" || call.Result["total"] != 1 {
		t.Fatalf("refs = %+v", refs)
	}
	// Жёсткое требование: явный флаг и дисклеймер у каждой инструкции.
	if !refs[0].IsSynthetic || refs[0].Disclaimer != "Тестовая инструкция (Синтетические данные)" || len(refs[0].Steps) == 0 || refs[0].Source == "" {
		t.Fatalf("документ без пометки синтетики: %+v", refs[0])
	}
	// Наставник может искать с явной командой; закрытый документ ему доступен.
	call = f.exec(f.mentor(t), ToolSearchDocs, map[string]any{"query": "оценка компетенций", "team": "Autoteka"})
	if refs := DocRefs(call.Result); len(refs) != 1 || refs[0].ID != "pb-mentor-guide" {
		t.Fatalf("поиск наставника: %+v", refs)
	}
	if call := f.exec(f.intern(t), ToolSearchDocs, map[string]any{"query": ""}); call.Status != domain.ToolInvalid {
		t.Fatalf("пустой запрос: %+v", call)
	}
}

func TestDocRefsTolerance(t *testing.T) {
	if got := DocRefs(nil); len(got) != 0 {
		t.Fatal("nil-результат")
	}
	got := DocRefs(map[string]any{"documents": []any{"мусор", map[string]any{"id": "d1", "title": "T"}}})
	if len(got) != 1 || got[0].ID != "d1" || got[0].Steps == nil || got[0].IsSynthetic {
		t.Fatalf("refs = %+v", got)
	}
}

func dayOffArgs() map[string]any {
	return map[string]any{
		"intern_id": seed.InternID, "date": "2026-11-13", "time_from": "14:00", "time_to": "18:00",
		"reason": "Зачёт в университете", "notify": []any{"mentor", "team_lead"},
	}
}

// Сценарий 3 на уровне инструмента.
func TestAbsencesCreateDayOff(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.intern(t), ToolCreateDayOff, dayOffArgs())
	if !call.OK() {
		t.Fatalf("создание отгула: %+v", call)
	}
	r := call.Result
	if r["request_number"] != "#ABS-2026-84" || r["status"] != domain.AbsenceStatusPending || r["is_synthetic"] != true {
		t.Fatalf("результат: %v", r)
	}
	if notified, _ := r["notified"].([]any); len(notified) != 2 || notified[0] != "Мария Кузнецова" || notified[1] != "Дмитрий Орлов" {
		t.Fatalf("уведомлены: %v", r["notified"])
	}
	saved, _ := domain.Load[domain.AbsenceRequest](context.Background(), f.store.Store, domain.KindAbsence, seed.InternID)
	if len(saved) != 1 || saved[0].Number != "#ABS-2026-84" || saved[0].InternName != "Алексей Смирнов" || !saved[0].IsSynthetic {
		t.Fatalf("заявка в БД: %+v", saved)
	}
	mentorInbox, leadInbox := f.notifications(t, seed.MentorID), f.notifications(t, seed.TeamLeadID)
	if len(mentorInbox) != 1 || len(leadInbox) != 1 {
		t.Fatalf("уведомления: наставник %d, руководитель %d", len(mentorInbox), len(leadInbox))
	}
	for _, n := range []domain.Notification{mentorInbox[0], leadInbox[0]} {
		if n.Kind != "absence" || !strings.Contains(n.Body, "#ABS-2026-84") || !strings.Contains(n.Body, "13.11.2026, 14:00–18:00") {
			t.Fatalf("уведомление: %+v", n)
		}
	}
	if !strings.Contains(leadInbox[0].Body, "ожидает согласования") {
		t.Fatalf("уведомление руководителю: %s", leadInbox[0].Body)
	}

	// Дубликат на ту же дату.
	dup := f.exec(f.intern(t), ToolCreateDayOff, dayOffArgs())
	if dup.Status != domain.ToolError || dup.ErrorCode != CodeDuplicate || !strings.Contains(dup.Error, "#ABS-2026-84") {
		t.Fatalf("дубликат: %+v", dup)
	}
	// Следующая заявка получает следующий номер; уведомляем только наставника; без времени.
	args := map[string]any{"intern_id": seed.InternID, "date": "2026-11-20", "reason": "Экзамен", "notify": []any{"mentor"}}
	next := f.exec(f.intern(t), ToolCreateDayOff, args)
	if !next.OK() || next.Result["request_number"] != "#ABS-2026-85" {
		t.Fatalf("вторая заявка: %+v", next)
	}
	if len(f.notifications(t, seed.MentorID)) != 2 || len(f.notifications(t, seed.TeamLeadID)) != 1 {
		t.Fatal("во второй заявке уведомляется только наставник")
	}
	if body := f.notifications(t, seed.MentorID)[1].Body; strings.Contains(body, "–") {
		t.Fatalf("для отгула на весь день время не указывается: %s", body)
	}
}

func TestAbsencesRejections(t *testing.T) {
	cases := []struct {
		name       string
		who        string
		mutate     func(map[string]any)
		wantStatus domain.ToolStatus
		wantErr    string
	}{
		{"дата в прошлом", seed.InternID, func(a map[string]any) { a["date"] = "2026-11-01" }, domain.ToolInvalid, "уже прошла"},
		{"конец раньше начала", seed.InternID, func(a map[string]any) { a["time_from"], a["time_to"] = "18:00", "14:00" }, domain.ToolInvalid, "позже начала"},
		{"короткая причина", seed.InternID, func(a map[string]any) { a["reason"] = "ок" }, domain.ToolInvalid, "reason"},
		{"неизвестный получатель", seed.InternID, func(a map[string]any) { a["notify"] = []any{"hr"} }, domain.ToolInvalid, "notify"},
		{"за другого сотрудника", seed.InternID, func(a map[string]any) { a["intern_id"] = "u-intern-99" }, domain.ToolForbidden, "self_only"},
		{"наставник за стажёра", seed.MentorID, func(map[string]any) {}, domain.ToolForbidden, "default deny"},
		{"руководитель за стажёра", seed.TeamLeadID, func(map[string]any) {}, domain.ToolForbidden, "default deny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			args := dayOffArgs()
			tc.mutate(args)
			call := f.exec(f.principal(t, tc.who), ToolCreateDayOff, args)
			if call.Status != tc.wantStatus || !strings.Contains(call.Error, tc.wantErr) {
				t.Fatalf("status=%s error=%q, want %s / %q", call.Status, call.Error, tc.wantStatus, tc.wantErr)
			}
			if saved, _ := domain.Load[domain.AbsenceRequest](context.Background(), f.store.Store, domain.KindAbsence, ""); len(saved) != 0 {
				t.Fatal("отклонённая заявка не должна сохраняться")
			}
		})
	}
}

func TestAbsencesDefaultsAndFailures(t *testing.T) {
	// Без notify уведомляются оба; без времени — отгул на весь день.
	f := newFixture(t)
	call := f.exec(f.intern(t), ToolCreateDayOff, map[string]any{"intern_id": seed.InternID, "date": "2026-11-13", "reason": "Личные обстоятельства"})
	if notified, _ := call.Result["notified"].([]any); !call.OK() || len(notified) != 2 {
		t.Fatalf("уведомления по умолчанию: %+v", call)
	}

	for _, kind := range []string{"get-intern", "list-absences", "put-notification", "put-absence"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			switch kind {
			case "get-intern":
				f.store.failGet[domain.KindIntern] = true
			case "list-absences":
				f.store.failList[domain.KindAbsence] = true
			case "put-notification":
				f.store.failPut[domain.KindNotification] = true
			case "put-absence":
				f.store.failPut[domain.KindAbsence] = true
			}
			if call := f.exec(f.intern(t), ToolCreateDayOff, dayOffArgs()); call.Status != domain.ToolError || call.ErrorCode != CodeInternal {
				t.Fatalf("сбой хранилища: %+v", call)
			}
		})
	}
	if humanDate("не дата") != "не дата" || humanDate("2026-11-13") != "13.11.2026" {
		t.Fatal("humanDate")
	}
}

func briefArgs() map[string]any {
	return map[string]any{
		"intern_id": seed.InternID, "mentor_id": seed.MentorID, "task_key": "AUT-1423",
		"learned_topics":        []any{"Изучил инструкцию «Как проходить код-ревью»"},
		"architecture_question": "На каком слое вычислять mileage_verified?",
		"signals":               []any{"Коммитов: 0"},
	}
}

func TestMentorSendBrief(t *testing.T) {
	f := newFixture(t)
	call := f.exec(f.cron(t), ToolSendBrief, briefArgs())
	if !call.OK() || call.Result["delivered_to"] != "Мария Кузнецова" {
		t.Fatalf("отправка брифа кроном: %+v", call)
	}
	briefs, _ := domain.Load[domain.MentorBrief](context.Background(), f.store.Store, domain.KindBrief, seed.InternID)
	if len(briefs) != 1 || briefs[0].ID != call.Result["brief_id"] || briefs[0].TaskKey != "AUT-1423" ||
		len(briefs[0].LearnedTopics) != 1 || briefs[0].OpenQuestions == nil || briefs[0].InternName == "" {
		t.Fatalf("бриф в БД: %+v", briefs)
	}
	inbox := f.notifications(t, seed.MentorID)
	if len(inbox) != 1 || inbox[0].Kind != "brief" || !strings.Contains(inbox[0].Body, "mileage_verified") {
		t.Fatalf("уведомление наставнику: %+v", inbox)
	}

	// Стажёр не может отправить бриф сам.
	if call := f.exec(f.intern(t), ToolSendBrief, briefArgs()); call.Status != domain.ToolForbidden {
		t.Fatalf("стажёр: %+v", call)
	}
	// Бриф можно отправить только наставнику этого стажёра.
	wrong := briefArgs()
	wrong["mentor_id"] = seed.TeamLeadID
	if call := f.exec(f.cron(t), ToolSendBrief, wrong); call.Status != domain.ToolInvalid || !strings.Contains(call.Error, "не является наставником") {
		t.Fatalf("чужой наставник: %+v", call)
	}
	short := briefArgs()
	short["architecture_question"] = "почему?"
	if call := f.exec(f.cron(t), ToolSendBrief, short); call.Status != domain.ToolInvalid {
		t.Fatalf("слишком короткий вопрос: %+v", call)
	}

	for _, kind := range []domain.Kind{domain.KindBrief, domain.KindNotification} {
		g := newFixture(t)
		g.store.failPut[kind] = true
		if call := g.exec(g.cron(t), ToolSendBrief, briefArgs()); call.ErrorCode != CodeInternal {
			t.Fatalf("сбой записи %s: %+v", kind, call)
		}
	}
	g := newFixture(t)
	g.store.failGet[domain.KindIntern] = true
	if call := g.exec(g.cron(t), ToolSendBrief, briefArgs()); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой чтения профиля: %+v", call)
	}
}

func TestNotificationsTool(t *testing.T) {
	f := newFixture(t)
	args := map[string]any{"recipient_id": seed.InternID, "title": "Привет", "body": "Чем помочь?", "kind": "proactive"}
	call := f.exec(f.cron(t), ToolSendNotification, args)
	if !call.OK() || call.Result["notification_id"] == "" {
		t.Fatalf("%+v", call)
	}
	if inbox := f.notifications(t, seed.InternID); len(inbox) != 1 || inbox[0].Kind != "proactive" {
		t.Fatalf("входящие: %+v", inbox)
	}
	if call := f.exec(f.intern(t), ToolSendNotification, args); call.Status != domain.ToolForbidden {
		t.Fatalf("стажёр не может рассылать уведомления: %+v", call)
	}
	// Вид по умолчанию — info.
	n, err := f.services.Notifications.Send(context.Background(), "u1", "t", "b", "", testNow)
	if err != nil || n.Kind != "info" {
		t.Fatalf("Send: %+v, %v", n, err)
	}
	f.store.failPut[domain.KindNotification] = true
	if call := f.exec(f.cron(t), ToolSendNotification, args); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой хранилища: %+v", call)
	}
}

func TestTrackerTaskActivity(t *testing.T) {
	f := newFixture(t)
	self := map[string]any{"intern_id": seed.InternID}
	call := f.exec(f.intern(t), ToolTaskActivity, self)
	if !call.OK() || call.Result["key"] != "AUT-1423" || call.Result["stalled"] != true || call.Result["commits"] != float64(0) {
		t.Fatalf("активность по задаче: %+v", call)
	}
	if call := f.exec(f.cron(t), ToolTaskActivity, self); !call.OK() {
		t.Fatalf("крон: %+v", call)
	}
	if call := f.exec(f.intern(t), ToolTaskActivity, map[string]any{"intern_id": "u-intern-99"}); call.Status != domain.ToolForbidden {
		t.Fatalf("чужая задача: %+v", call)
	}
	if call := f.exec(f.mentor(t), ToolTaskActivity, map[string]any{"intern_id": "u-intern-99"}); call.Status != domain.ToolInvalid {
		t.Fatalf("нет задач: %+v", call)
	}
	f.store.failGet[domain.KindSprintTask] = true
	if call := f.exec(f.cron(t), ToolTaskActivity, self); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой хранилища: %+v", call)
	}
}

func TestFeatureFlags(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	flags := f.services.Flags

	list := flags.List(ctx)
	if len(list) != len(f.seed.Flags) || !flags.Enabled(ctx, FlagProactiveCron) || !flags.Enabled(ctx, FlagAbsenceCard) || !flags.Enabled(ctx, FlagLLMAnswers) {
		t.Fatalf("значения по умолчанию: %+v", list)
	}
	if flags.Enabled(ctx, "ai_buddy.unknown") {
		t.Fatal("неизвестный флаг должен быть выключен")
	}

	flag, err := flags.Set(ctx, FlagProactiveCron, false)
	if err != nil || flag.Enabled || flags.Enabled(ctx, FlagProactiveCron) {
		t.Fatalf("выключение runtime-флага: %+v, %v", flag, err)
	}
	if v, ok, _ := f.cache.Get(ctx, "ff:"+FlagProactiveCron); !ok || v != "0" {
		t.Fatalf("runtime-значение должно лежать в кэше (Redis): %q, %v", v, ok)
	}
	if flag, err := flags.Set(ctx, FlagProactiveCron, true); err != nil || !flag.Enabled {
		t.Fatalf("включение: %+v, %v", flag, err)
	}
	if _, err := flags.Set(ctx, "ai_buddy.mcp_jsonrpc", false); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("compile-time флаг нельзя менять: %v", err)
	}
	if _, err := flags.Set(ctx, "ai_buddy.unknown", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("неизвестный флаг: %v", err)
	}

	_, _ = flags.Set(ctx, FlagAbsenceCard, false)
	if err := flags.Reset(ctx); err != nil || !flags.Enabled(ctx, FlagAbsenceCard) {
		t.Fatalf("Reset должен вернуть значения по умолчанию: %v", err)
	}

	call := f.exec(f.intern(t), ToolListFlags, map[string]any{})
	if got, _ := call.Result["flags"].([]any); !call.OK() || len(got) != len(f.seed.Flags) {
		t.Fatalf("feature_flags.list: %+v", call)
	}

	// Сбой кэша: чтение откатывается на значения по умолчанию, запись возвращает ошибку.
	broken := NewFlags(failCache{}, f.seed.Flags)
	if !broken.Enabled(ctx, FlagProactiveCron) {
		t.Fatal("при сбое кэша должны действовать значения по умолчанию")
	}
	if _, err := broken.Set(ctx, FlagProactiveCron, false); !errors.Is(err, errStore) {
		t.Fatalf("ошибка записи должна пробрасываться: %v", err)
	}
	if err := broken.Reset(ctx); !errors.Is(err, errStore) {
		t.Fatalf("ошибка сброса должна пробрасываться: %v", err)
	}
}

func rpc(t *testing.T, f *fixture, p domain.Principal, body string) *RPCResponse {
	t.Helper()
	return f.gw.HandleRPC(context.Background(), p, seed.InternID, []byte(body))
}

func TestJSONRPC(t *testing.T) {
	f := newFixture(t)
	intern := f.intern(t)

	resp := rpc(t, f, intern, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	result, _ := resp.Result.(map[string]any)
	if resp.Error != nil || result["protocolVersion"] != ProtocolVersion || string(resp.ID) != "1" || resp.JSONRPC != "2.0" {
		t.Fatalf("initialize: %+v", resp)
	}
	if resp := rpc(t, f, intern, `{"jsonrpc":"2.0","id":"p","method":"ping"}`); resp.Error != nil || string(resp.ID) != `"p"` {
		t.Fatalf("ping: %+v", resp)
	}
	if resp := rpc(t, f, intern, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp != nil {
		t.Fatalf("уведомление не должно иметь ответа: %+v", resp)
	}

	resp = rpc(t, f, intern, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools, _ := resp.Result.(map[string]any)["tools"].([]map[string]any)
	if len(tools) != len(f.bundle.Tools.Tools) || tools[0]["name"] != ToolRequestRepoAccess || tools[0]["inputSchema"] == nil || tools[0]["outputSchema"] == nil {
		t.Fatalf("tools/list: %d инструментов", len(tools))
	}

	resp = rpc(t, f, intern, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"iam_access.request_repo_access","arguments":{"intern_id":"u-intern-01","repository":"avito-autoteka-api"}}}`)
	result, _ = resp.Result.(map[string]any)
	structured, _ := result["structuredContent"].(map[string]any)
	if result["isError"] != false || structured["status"] != "granted" {
		t.Fatalf("tools/call (успех): %+v", resp)
	}
	content, _ := result["content"].([]map[string]any)
	if len(content) != 1 || !strings.Contains(content[0]["text"].(string), "avito-autoteka-api") {
		t.Fatalf("content: %+v", content)
	}

	resp = rpc(t, f, intern, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"db_admin.export_prod_pii","arguments":{"dataset":"payroll"}}}`)
	result, _ = resp.Result.(map[string]any)
	content, _ = result["content"].([]map[string]any)
	if result["isError"] != true || !strings.HasPrefix(content[0]["text"].(string), "FORBIDDEN") {
		t.Fatalf("tools/call (запрет): %+v", resp)
	}
	if meta, _ := result["_meta"].(map[string]any); meta["rbac"].(domain.RBACVerdict).PolicyID != "POL-SEC-001" {
		t.Fatalf("_meta.rbac: %+v", result["_meta"])
	}

	errCases := []struct {
		name string
		body string
		code int
	}{
		{"некорректный JSON", `{не json`, rpcParseError},
		{"не JSON-RPC 2.0", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, rpcInvalidRequest},
		{"нет метода", `{"jsonrpc":"2.0","id":1}`, rpcInvalidRequest},
		{"неизвестный метод", `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, rpcMethodNotFound},
		{"tools/call без имени", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`, rpcInvalidParams},
		{"tools/call с битыми params", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"x"}`, rpcInvalidParams},
		{"неизвестный инструмент", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ghost.tool"}}`, rpcInvalidParams},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := rpc(t, f, intern, tc.body)
			if resp == nil || resp.Error == nil || resp.Error.Code != tc.code || len(resp.ID) == 0 {
				t.Fatalf("resp = %+v, want code %d", resp, tc.code)
			}
			if _, err := json.Marshal(resp); err != nil {
				t.Fatalf("ответ должен сериализоваться: %v", err)
			}
		})
	}
}
