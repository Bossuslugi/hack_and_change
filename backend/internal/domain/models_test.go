package domain

import (
	"errors"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) // четверг

func TestChecklistStatusValid(t *testing.T) {
	cases := map[ChecklistStatus]bool{
		StatusPending: true, StatusInProgress: true, StatusCompleted: true, "done": false, "": false,
	}
	for status, want := range cases {
		if got := status.Valid(); got != want {
			t.Errorf("Valid(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestChecklistItemAdvance(t *testing.T) {
	cases := []struct {
		name        string
		from, to    ChecklistStatus
		wantStatus  ChecklistStatus
		wantChanged bool
		wantErr     bool
	}{
		{"pending → in_progress", StatusPending, StatusInProgress, StatusInProgress, true, false},
		{"pending → completed", StatusPending, StatusCompleted, StatusCompleted, true, false},
		{"in_progress → completed", StatusInProgress, StatusCompleted, StatusCompleted, true, false},
		{"повтор статуса — no-op", StatusCompleted, StatusCompleted, StatusCompleted, false, false},
		{"откат запрещён", StatusCompleted, StatusInProgress, StatusCompleted, false, true},
		{"откат в pending запрещён", StatusInProgress, StatusPending, StatusInProgress, false, true},
		{"неизвестный статус", StatusPending, "done", StatusPending, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := ChecklistItem{ID: "x", Status: tc.from, Evidence: "старое"}
			got, changed, err := item.Advance(tc.to, "новое", testNow)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("ожидался ErrInvalidTransition, получено %v", err)
			}
			if got.Status != tc.wantStatus || changed != tc.wantChanged {
				t.Fatalf("status=%s changed=%v, want %s/%v", got.Status, changed, tc.wantStatus, tc.wantChanged)
			}
			if changed {
				if got.Evidence != "новое" || !got.UpdatedAt.Equal(testNow) {
					t.Errorf("evidence/updated_at не обновлены: %+v", got)
				}
				if (got.CompletedAt != nil) != (tc.to == StatusCompleted) {
					t.Errorf("completed_at выставлен неверно: %v", got.CompletedAt)
				}
			}
		})
	}
	// Пустое подтверждение не затирает прежнее.
	item, _, _ := ChecklistItem{Status: StatusPending, Evidence: "старое"}.Advance(StatusInProgress, "", testNow)
	if item.Evidence != "старое" {
		t.Errorf("evidence затёрт: %q", item.Evidence)
	}
}

func TestComputeProgressAndNextStep(t *testing.T) {
	items := []ChecklistItem{
		{ID: "c", Position: 3, Status: StatusPending},
		{ID: "a", Position: 1, Status: StatusCompleted},
		{ID: "b", Position: 2, Status: StatusInProgress},
		{ID: "d", Position: 4, Status: StatusCompleted},
	}
	p := ComputeProgress(items)
	if p.Total != 4 || p.Completed != 2 || p.InProgress != 1 || p.Percent != 50 {
		t.Fatalf("progress = %+v", p)
	}
	if next := NextStep(items); next == nil || next.ID != "b" {
		t.Fatalf("next = %+v, want b", next)
	}
	if p := ComputeProgress(nil); p.Percent != 0 || p.Total != 0 {
		t.Fatalf("пустой чек-лист: %+v", p)
	}
	done := []ChecklistItem{{ID: "a", Status: StatusCompleted}}
	if NextStep(done) != nil {
		t.Fatal("для выполненного чек-листа следующий шаг должен быть nil")
	}
}

func TestAbsenceRequestValidate(t *testing.T) {
	valid := AbsenceRequest{InternID: "u1", Date: "2026-11-13", TimeFrom: "14:00", TimeTo: "18:00", Reason: "Зачёт"}
	cases := []struct {
		name    string
		mutate  func(*AbsenceRequest)
		wantErr bool
	}{
		{"корректная заявка", func(*AbsenceRequest) {}, false},
		{"сегодня допустимо", func(a *AbsenceRequest) { a.Date = "2026-11-12" }, false},
		{"без времени — весь день", func(a *AbsenceRequest) { a.TimeFrom, a.TimeTo = "", "" }, false},
		{"нет сотрудника", func(a *AbsenceRequest) { a.InternID = " " }, true},
		{"формат даты", func(a *AbsenceRequest) { a.Date = "13.11.2026" }, true},
		{"несуществующая дата", func(a *AbsenceRequest) { a.Date = "2026-02-31" }, true},
		{"дата в прошлом", func(a *AbsenceRequest) { a.Date = "2026-11-11" }, true},
		{"только начало", func(a *AbsenceRequest) { a.TimeTo = "" }, true},
		{"формат времени", func(a *AbsenceRequest) { a.TimeFrom = "25:00" }, true},
		{"конец раньше начала", func(a *AbsenceRequest) { a.TimeFrom, a.TimeTo = "18:00", "14:00" }, true},
		{"короткая причина", func(a *AbsenceRequest) { a.Reason = "ок" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := valid
			tc.mutate(&a)
			err := a.Validate(testNow)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("ожидался ErrValidation, получено %v", err)
			}
		})
	}
}

func TestAbsenceNumber(t *testing.T) {
	if got := AbsenceNumber(2026, 0); got != "#ABS-2026-84" {
		t.Fatalf("первая заявка = %s, want #ABS-2026-84", got)
	}
	if got := AbsenceNumber(2027, 3); got != "#ABS-2027-87" {
		t.Fatalf("got %s", got)
	}
}

func TestMaskToken(t *testing.T) {
	cases := map[string]string{
		"glpat-synt-0123456789abcdef": "glpat-synt-••••cdef",
		"abcdefgh":                    "••••efgh",
		"abc":                         "•••",
		"":                            "",
	}
	for in, want := range cases {
		if got := MaskToken(in); got != want {
			t.Errorf("MaskToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKnowledgeDocVisibility(t *testing.T) {
	open := KnowledgeDoc{ID: "d1", Title: "T", Team: "Autoteka", Steps: []string{"a"}, IsSynthetic: true, Disclaimer: SyntheticDisclaimer}
	restricted := KnowledgeDoc{ID: "d2", Team: "Autoteka", Audience: []Role{RoleMentor, RoleTeamLead}}
	cases := []struct {
		doc  KnowledgeDoc
		role Role
		team string
		want bool
	}{
		{open, RoleInternBackend, "Autoteka", true},
		{open, RoleInternBackend, "autoteka", true},
		{open, RoleInternBackend, "Payments", false},
		{open, RoleInternBackend, "", true},
		{restricted, RoleInternBackend, "Autoteka", false},
		{restricted, RoleMentor, "Autoteka", true},
		{restricted, RoleTeamLead, "Payments", false},
	}
	for i, tc := range cases {
		if got := tc.doc.VisibleTo(tc.role, tc.team); got != tc.want {
			t.Errorf("case %d: VisibleTo(%s, %s) = %v, want %v", i, tc.role, tc.team, got, tc.want)
		}
	}
	ref := open.Ref()
	if ref.ID != "d1" || !ref.IsSynthetic || ref.Disclaimer != SyntheticDisclaimer || len(ref.Steps) != 1 {
		t.Fatalf("ref = %+v", ref)
	}
	ref.Steps[0] = "изменено"
	if open.Steps[0] != "a" {
		t.Fatal("Ref должен копировать шаги")
	}
}

func TestSprintTaskStalled(t *testing.T) {
	cases := []struct {
		task SprintTask
		want bool
	}{
		{SprintTask{Status: "in_progress", Commits: 0, DaysInProgress: 3}, true},
		{SprintTask{Status: "in_progress", Commits: 0, DaysInProgress: StalledAfterDays}, true},
		{SprintTask{Status: "in_progress", Commits: 0, DaysInProgress: 1}, false},
		{SprintTask{Status: "in_progress", Commits: 2, DaysInProgress: 5}, false},
		{SprintTask{Status: "completed", Commits: 0, DaysInProgress: 5}, false},
	}
	for i, tc := range cases {
		if got := tc.task.Stalled(); got != tc.want {
			t.Errorf("case %d: Stalled() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestProfileHelpers(t *testing.T) {
	p := InternProfile{Name: "Алексей Смирнов", Team: "Autoteka", Stack: "Go", Stage: "Week 1"}
	if p.FirstName() != "Алексей" {
		t.Errorf("FirstName = %q", p.FirstName())
	}
	if (InternProfile{}).FirstName() != "коллега" {
		t.Error("для пустого имени ожидается обращение «коллега»")
	}
	if got := p.ContextLine(); got != "команда Autoteka · стек Go · этап Week 1" {
		t.Errorf("ContextLine = %q", got)
	}
	e := Employee{ID: "u1", Name: "N", Role: RoleMentor, Team: "T"}
	if pr := e.Principal(); pr.ID != "u1" || pr.Role != RoleMentor || pr.Team != "T" || pr.Name != "N" {
		t.Errorf("Principal = %+v", pr)
	}
	if !(MCPToolCall{Status: ToolSuccess}).OK() || (MCPToolCall{Status: ToolForbidden}).OK() {
		t.Error("MCPToolCall.OK работает неверно")
	}
}

func TestComputeMetrics(t *testing.T) {
	weights := map[string]int{SavingQuestion: 10, SavingAccess: 15, SavingAbsence: 10, SavingBrief: 20, SavingBlocked: 5}
	ev := func(kind string, payload map[string]any) TelemetryEvent {
		return TelemetryEvent{Kind: kind, Payload: payload}
	}
	events := []TelemetryEvent{
		ev(EventKindAgent, map[string]any{"saving_key": SavingQuestion, "latency_ms": 2.0}),
		ev(EventKindAgent, map[string]any{"saving_key": SavingQuestion, "latency_ms": 4.0}),
		ev(EventKindAgent, map[string]any{"saving_key": SavingAccess}),
		ev(EventKindAgent, map[string]any{"saving_key": SavingAbsence}),
		ev(EventKindAgent, map[string]any{"saving_key": SavingBlocked}),
		ev(EventKindCron, map[string]any{"saving_key": SavingBrief}),
		ev(EventKindMCP, map[string]any{"tool": "x", "latency_ms": 100.0}),
		ev(EventKindMCP, nil),
		ev(EventKindClickstream, map[string]any{"saving_key": "unknown"}),
	}
	m := ComputeMetrics(MetricsInput{
		Events: events, MinutesSaved: weights, BaselineHoursMin: 40, BaselineHoursMax: 48,
		Progress: Progress{Percent: 42}, DocsRead: 2, DocsTotal: 7, OpenQuestions: 1,
	})
	if m.QuestionsAnswered != 2 || m.ActionsAutomated != 2 || m.RBACBlocks != 1 || m.BriefsPrepared != 1 {
		t.Fatalf("счётчики: %+v", m)
	}
	if m.MentorMinutesSaved != 10+10+15+10+5+20 {
		t.Fatalf("минуты = %d", m.MentorMinutesSaved)
	}
	if m.ToolCalls != 2 || m.AvgResponseMS != 3.0 || m.DocsRead != 2 || m.DocsTotal != 7 || m.ChecklistPercent != 42 || m.UnansweredQuestions != 1 {
		t.Fatalf("прочие поля: %+v", m)
	}
	// 70 минут от среднего базового уровня 44 ч = 2640 мин → 2.7%
	if m.SavingPercent != 2.7 {
		t.Fatalf("SavingPercent = %v, want 2.7", m.SavingPercent)
	}
	empty := ComputeMetrics(MetricsInput{})
	if empty.SavingPercent != 0 || empty.AvgResponseMS != 0 {
		t.Fatalf("пустые метрики: %+v", empty)
	}
}
