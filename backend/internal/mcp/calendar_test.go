package mcp

import (
	"context"
	"strings"
	"testing"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/seed"
)

func (f *fixture) planner(t *testing.T) []domain.PlannerItem {
	t.Helper()
	items, err := domain.Load[domain.PlannerItem](context.Background(), f.store.Store, domain.KindPlanner, seed.InternID)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func taskArgs() map[string]any {
	return map[string]any{"intern_id": seed.InternID, "title": "Настроить Go-окружение", "date": "2026-11-13", "time": "12:30", "description": "Go, линтер, pre-commit"}
}

func TestCalendarAddTask(t *testing.T) {
	f := newFixture(t)
	seeded := len(f.planner(t))
	if seeded == 0 {
		t.Fatal("в seed должен быть календарь первых недель")
	}

	// Стажёр: личная задача, без уведомлений.
	call := f.exec(f.intern(t), ToolAddTask, taskArgs())
	if !call.OK() || call.Result["mandatory"] != false || call.Result["kind"] != domain.PlannerTask || call.Result["all_day"] != false {
		t.Fatalf("личная задача: %+v", call)
	}
	if got := f.planner(t); len(got) != seeded+1 || got[len(got)-1].CreatedBy != seed.InternID || got[len(got)-1].StartsAt.Format("02.01 15:04") != "13.11 12:30" {
		t.Fatalf("задача в БД: %+v", got[len(got)-1])
	}
	if len(f.notifications(t, seed.InternID)) != 0 {
		t.Fatal("личная задача не должна порождать уведомлений")
	}

	// Наставник: обязательная задача на весь день + уведомление стажёру.
	args := map[string]any{"intern_id": seed.InternID, "title": "Пройти курс ИБ", "date": "2026-11-16"}
	call = f.exec(f.mentor(t), ToolAddTask, args)
	if !call.OK() || call.Result["mandatory"] != true || call.Result["all_day"] != true || call.Result["created_by_name"] != "Мария Кузнецова" {
		t.Fatalf("обязательная задача: %+v", call)
	}
	inbox := f.notifications(t, seed.InternID)
	if len(inbox) != 1 || inbox[0].Kind != domain.NotificationTaskAssigned || !strings.Contains(inbox[0].Body, "«Пройти курс ИБ»") || !strings.Contains(inbox[0].Body, "16.11") {
		t.Fatalf("уведомление стажёру: %+v", inbox)
	}
}

func TestCalendarAddTaskRejections(t *testing.T) {
	cases := []struct {
		name       string
		who        string
		mutate     func(map[string]any)
		wantStatus domain.ToolStatus
		wantErr    string
	}{
		{"дата в прошлом", seed.InternID, func(a map[string]any) { a["date"] = "2026-11-01" }, domain.ToolInvalid, "уже прошла"},
		{"несуществующая дата", seed.InternID, func(a map[string]any) { a["date"] = "2026-02-31" }, domain.ToolInvalid, "некорректная дата"},
		{"короткое название", seed.InternID, func(a map[string]any) { a["title"] = "ок" }, domain.ToolInvalid, "title"},
		{"название из пробелов", seed.InternID, func(a map[string]any) { a["title"] = "      " }, domain.ToolInvalid, "название"},
		{"формат времени", seed.InternID, func(a map[string]any) { a["time"] = "25:00" }, domain.ToolInvalid, "time"},
		{"задача другому стажёру", seed.InternID, func(a map[string]any) { a["intern_id"] = "u-intern-99" }, domain.ToolForbidden, "self_only"},
		{"неизвестный сотрудник", seed.MentorID, func(a map[string]any) { a["intern_id"] = "u-ghost" }, domain.ToolInvalid, "не найден"},
		{"сервисный аккаунт", seed.CronID, func(map[string]any) {}, domain.ToolForbidden, "default deny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			before := len(f.planner(t))
			args := taskArgs()
			tc.mutate(args)
			call := f.exec(f.principal(t, tc.who), ToolAddTask, args)
			if call.Status != tc.wantStatus || !strings.Contains(call.Error, tc.wantErr) {
				t.Fatalf("status=%s error=%q, want %s / %q", call.Status, call.Error, tc.wantStatus, tc.wantErr)
			}
			if len(f.planner(t)) != before {
				t.Fatal("отклонённая задача не должна сохраняться")
			}
		})
	}
	for _, kind := range []domain.Kind{domain.KindPlanner, domain.KindNotification} {
		f := newFixture(t)
		f.store.failPut[kind] = true
		if call := f.exec(f.mentor(t), ToolAddTask, taskArgs()); call.ErrorCode != CodeInternal {
			t.Fatalf("сбой записи %s: %+v", kind, call)
		}
	}
	f := newFixture(t)
	f.store.failGet[domain.KindIntern] = true
	if call := f.exec(f.intern(t), ToolAddTask, taskArgs()); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой чтения профиля: %+v", call)
	}
}

func TestCalendarCompleteItem(t *testing.T) {
	f := newFixture(t)
	args := map[string]any{"intern_id": seed.InternID, "item_id": "plan-1on1", "report": " Обсудили план на неделю "}
	call := f.exec(f.intern(t), ToolCompleteItem, args)
	if !call.OK() || call.Result["status"] != domain.PlannerDone || call.Result["report"] != "Обсудили план на неделю" || call.Result["title"] == "" {
		t.Fatalf("выполнение с отчётом: %+v", call)
	}
	for _, it := range f.planner(t) {
		if it.ID == "plan-1on1" && (it.Status != domain.PlannerDone || it.CompletedAt == nil) {
			t.Fatalf("событие в БД: %+v", it)
		}
	}
	// Повторно выполнить нельзя.
	if again := f.exec(f.intern(t), ToolCompleteItem, args); again.Status != domain.ToolInvalid || !strings.Contains(again.Error, "уже отмечено") {
		t.Fatalf("повтор: %+v", again)
	}
	cases := []struct {
		name       string
		who        string
		args       map[string]any
		wantStatus domain.ToolStatus
	}{
		{"несуществующее событие", seed.InternID, map[string]any{"intern_id": seed.InternID, "item_id": "plan-ghost"}, domain.ToolInvalid},
		{"за другого стажёра", seed.InternID, map[string]any{"intern_id": "u-intern-99", "item_id": "plan-sync"}, domain.ToolForbidden},
		{"наставник не может отметить за стажёра", seed.MentorID, map[string]any{"intern_id": seed.InternID, "item_id": "plan-sync"}, domain.ToolForbidden},
		{"слишком длинный отчёт", seed.InternID, map[string]any{"intern_id": seed.InternID, "item_id": "plan-sync", "report": strings.Repeat("я", 1001)}, domain.ToolInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if call := f.exec(f.principal(t, tc.who), ToolCompleteItem, tc.args); call.Status != tc.wantStatus {
				t.Fatalf("status = %s (%s), want %s", call.Status, call.Error, tc.wantStatus)
			}
		})
	}
	ok := map[string]any{"intern_id": seed.InternID, "item_id": "plan-sync"}
	f.store.failPut[domain.KindPlanner] = true
	if call := f.exec(f.intern(t), ToolCompleteItem, ok); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой записи: %+v", call)
	}
	f.store.failPut[domain.KindPlanner] = false
	f.store.failGet[domain.KindPlanner] = true
	if call := f.exec(f.intern(t), ToolCompleteItem, ok); call.ErrorCode != CodeInternal {
		t.Fatalf("сбой чтения: %+v", call)
	}
}

func TestAbsenceApproval(t *testing.T) {
	f := newFixture(t)
	lead := f.principal(t, seed.TeamLeadID)
	if call := f.exec(f.intern(t), ToolCreateDayOff, dayOffArgs()); !call.OK() {
		t.Fatalf("создание отгула: %+v", call)
	}
	args := map[string]any{"request_number": "#ABS-2026-84"}

	// Согласовать может только руководитель.
	for _, id := range []string{seed.InternID, seed.MentorID, seed.CronID} {
		if call := f.exec(f.principal(t, id), ToolApproveDayOff, args); call.Status != domain.ToolForbidden {
			t.Fatalf("%s не должен согласовывать отгул: %+v", id, call)
		}
	}
	call := f.exec(lead, ToolApproveDayOff, args)
	if !call.OK() || call.Result["status"] != domain.AbsenceStatusApproved || call.RBAC.PolicyID != "POL-ABS-002" {
		t.Fatalf("согласование: %+v", call)
	}
	saved, _ := domain.Load[domain.AbsenceRequest](context.Background(), f.store.Store, domain.KindAbsence, seed.InternID)
	if len(saved) != 1 || saved[0].Status != domain.AbsenceStatusApproved {
		t.Fatalf("статус заявки: %+v", saved)
	}
	inbox := f.notifications(t, seed.InternID)
	if len(inbox) != 1 || inbox[0].Kind != domain.NotificationAbsenceApproved || inbox[0].Body != "Руководитель согласовал твой отгул #ABS-2026-84 на 13.11.2026" {
		t.Fatalf("уведомление стажёру: %+v", inbox)
	}

	if again := f.exec(lead, ToolApproveDayOff, args); again.Status != domain.ToolInvalid || !strings.Contains(again.Error, "уже согласована") {
		t.Fatalf("повторное согласование: %+v", again)
	}
	if missing := f.exec(lead, ToolApproveDayOff, map[string]any{"request_number": "#ABS-2026-99"}); missing.Status != domain.ToolInvalid || !strings.Contains(missing.Error, "не найдена") {
		t.Fatalf("несуществующая заявка: %+v", missing)
	}
	if bad := f.exec(lead, ToolApproveDayOff, map[string]any{"request_number": "84"}); bad.Status != domain.ToolInvalid {
		t.Fatalf("формат номера: %+v", bad)
	}

	for _, br := range []func(*faultyStore){
		func(s *faultyStore) { s.failList[domain.KindAbsence] = true },
		func(s *faultyStore) { s.failPut[domain.KindAbsence] = true },
		func(s *faultyStore) { s.failPut[domain.KindNotification] = true },
	} {
		g := newFixture(t)
		if call := g.exec(g.intern(t), ToolCreateDayOff, dayOffArgs()); !call.OK() {
			t.Fatal(call.Error)
		}
		br(g.store)
		if call := g.exec(g.principal(t, seed.TeamLeadID), ToolApproveDayOff, args); call.ErrorCode != CodeInternal {
			t.Fatalf("сбой хранилища: %+v", call)
		}
	}
}
