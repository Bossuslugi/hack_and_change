package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// testNow — четверг, 12 ноября 2026, 10:00.

func TestPlannerItemValidateAndComplete(t *testing.T) {
	valid := PlannerItem{ID: "p1", InternID: "u1", Kind: PlannerTask, Title: "Настроить Go-окружение", StartsAt: testNow.Add(time.Hour), Status: PlannerPlanned}
	cases := []struct {
		name    string
		mutate  func(*PlannerItem)
		wantErr bool
	}{
		{"корректная задача", func(*PlannerItem) {}, false},
		{"сегодня, но раньше текущего времени — допустимо", func(p *PlannerItem) { p.StartsAt = testNow.Add(-2 * time.Hour) }, false},
		{"нет сотрудника", func(p *PlannerItem) { p.InternID = " " }, true},
		{"короткое название", func(p *PlannerItem) { p.Title = "ок" }, true},
		{"длинное название", func(p *PlannerItem) { p.Title = strings.Repeat("я", 121) }, true},
		{"вчерашняя дата", func(p *PlannerItem) { p.StartsAt = testNow.AddDate(0, 0, -1) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := valid
			tc.mutate(&p)
			err := p.Validate(testNow)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("ожидался ErrValidation: %v", err)
			}
		})
	}

	done, err := valid.Complete("  Настроил, всё работает  ", testNow)
	if err != nil || done.Status != PlannerDone || done.Report != "Настроил, всё работает" || done.CompletedAt == nil || !done.CompletedAt.Equal(testNow) {
		t.Fatalf("Complete: %+v, %v", done, err)
	}
	if valid.Status != PlannerPlanned {
		t.Fatal("Complete не должен менять исходное значение")
	}
	if _, err := done.Complete("", testNow); !errors.Is(err, ErrValidation) {
		t.Fatalf("повторное выполнение: %v", err)
	}
	for kind, want := range map[string]bool{PlannerMeeting: true, PlannerReview: true, PlannerSync: true, PlannerDeadline: false, PlannerTask: false} {
		if got := (PlannerItem{Kind: kind}).IsEvent(); got != want {
			t.Errorf("IsEvent(%s) = %v", kind, got)
		}
	}
}

func TestRelativeWhen(t *testing.T) {
	cases := []struct {
		at       time.Time
		withTime bool
		want     string
	}{
		{testNow.Add(20 * time.Second), true, "прямо сейчас"},
		{testNow.Add(25 * time.Minute), true, "через 25 мин"},
		{testNow.Add(time.Hour), true, "через 1 час"},
		{testNow.Add(3 * time.Hour), true, "через 3 часа"},
		{testNow.Add(5 * time.Hour), true, "через 5 часов"},
		{testNow.Add(11 * time.Hour), true, "через 11 часов"},
		{testNow.Add(13 * time.Hour), true, "сегодня в 23:00"},
		{testNow.Add(-time.Hour), true, "сегодня в 09:00"},
		{testNow.Add(8 * time.Hour), false, "сегодня"},
		{testNow.AddDate(0, 0, 1).Add(5 * time.Hour), true, "завтра в 15:00"},
		{testNow.AddDate(0, 0, 1), false, "завтра"}, // пятница — уже «завтра»
		{testNow.AddDate(0, 0, 2), false, "в субботу"},
		{testNow.AddDate(0, 0, 5).Add(time.Hour), true, "во вторник в 11:00"},
		{testNow.AddDate(0, 0, 6), false, "в среду"},
		{testNow.AddDate(0, 0, 9), false, "21.11"},
		{testNow.AddDate(0, 0, -3), false, "09.11"},
	}
	for _, tc := range cases {
		if got := RelativeWhen(tc.at, testNow, tc.withTime); got != tc.want {
			t.Errorf("RelativeWhen(%s, time=%v) = %q, want %q", tc.at.Format("02.01 15:04"), tc.withTime, got, tc.want)
		}
	}
	if upperFirst("") != "" || upperFirst("через") != "Через" {
		t.Error("upperFirst")
	}
	forms := []string{pluralRu(1, "час", "часа", "часов"), pluralRu(3, "час", "часа", "часов"), pluralRu(11, "час", "часа", "часов"), pluralRu(21, "час", "часа", "часов"), pluralRu(7, "час", "часа", "часов")}
	if strings.Join(forms, ",") != "час,часа,часов,час,часов" {
		t.Errorf("pluralRu: %v", forms)
	}
}

func TestBuildReminders(t *testing.T) {
	at := func(d time.Duration) time.Time { return testNow.Add(d) }
	items := []PlannerItem{
		{ID: "1on1", Kind: PlannerMeeting, Title: "Созвон 1:1 с наставником", StartsAt: at(3 * time.Hour)},
		{ID: "pr", Kind: PlannerDeadline, Title: "Срок сдачи первого PR", StartsAt: testNow.AddDate(0, 0, 2).Add(8 * time.Hour)},
		{ID: "review", Kind: PlannerReview, Title: "Код-ревью", StartsAt: testNow.AddDate(0, 0, 2).Add(5 * time.Hour)},
		{ID: "course", Kind: PlannerTask, Title: "Пройти курс ИБ", StartsAt: testNow.AddDate(0, 0, 5), AllDay: true, Mandatory: true},
		{ID: "own", Kind: PlannerTask, Title: "Сдать зачёт", StartsAt: testNow.AddDate(0, 0, 4), AllDay: true},
		{ID: "late", Kind: PlannerTask, Title: "Настроить VPN", StartsAt: testNow.AddDate(0, 0, -1), AllDay: true, Mandatory: true},
		{ID: "today", Kind: PlannerTask, Title: "Прочитать онбординг", StartsAt: dayStart(testNow), AllDay: true, Mandatory: true},
		{ID: "past-meeting", Kind: PlannerSync, Title: "Вчерашний синк", StartsAt: at(-20 * time.Hour)},
		{ID: "done", Kind: PlannerMeeting, Title: "Вводная встреча", StartsAt: at(time.Hour), Status: PlannerDone},
		{ID: "far", Kind: PlannerMeeting, Title: "Экватор", StartsAt: testNow.AddDate(0, 0, 30)},
	}
	notes := []Notification{
		{ID: "n1", Kind: NotificationAbsenceApproved, Body: "Руководитель согласовал твой отгул #ABS-2026-84 на 13.11.2026", CreatedAt: at(-time.Minute)},
		{ID: "n2", Kind: "brief", Body: "не для ленты", CreatedAt: testNow},
		{ID: "n3", Kind: NotificationTaskAssigned, Body: "Мария назначила задачу", CreatedAt: testNow},
	}
	got := BuildReminders(items, notes, testNow)
	texts := map[string]Reminder{}
	for _, r := range got {
		texts[r.ID] = r
	}
	want := map[string]struct {
		text   string
		urgent bool
	}{
		"rem-1on1":   {"🚨 Через 3 часа — Созвон 1:1 с наставником", true},
		"rem-pr":     {"📅 Срок сдачи первого PR — в субботу", false},
		"rem-review": {"🗓 В субботу в 15:00 — Код-ревью", false},
		"rem-course": {"📌 Обязательная задача «Пройти курс ИБ» — во вторник", false},
		"rem-own":    {"✅ Твоя задача «Сдать зачёт» — в понедельник", false},
		"rem-late":   {"⏰ Просрочено: Настроить VPN", true},
		"rem-today":  {"📌 Обязательная задача «Прочитать онбординг» — сегодня", true},
		"rem-n1":     {"🔔 Руководитель согласовал твой отгул #ABS-2026-84 на 13.11.2026", true},
		"rem-n3":     {"🔔 Мария назначила задачу", false},
	}
	if len(got) != MaxReminders {
		t.Fatalf("лента ограничена %d напоминаниями, получено %d", MaxReminders, len(got))
	}
	found := 0
	for id, w := range want {
		r, ok := texts[id]
		if !ok {
			continue // вытеснено лимитом
		}
		found++
		if r.Text != w.text || r.Urgent != w.urgent {
			t.Errorf("%s = %q (urgent=%v), want %q (urgent=%v)", id, r.Text, r.Urgent, w.text, w.urgent)
		}
	}
	if found != MaxReminders {
		t.Fatalf("в ленте есть лишние напоминания: %+v", got)
	}
	for _, id := range []string{"rem-past-meeting", "rem-done", "rem-far", "rem-n2"} {
		if _, ok := texts[id]; ok {
			t.Errorf("напоминание %s не должно попадать в ленту", id)
		}
	}
	// Срочные идут первыми, внутри группы — по времени.
	for i := 1; i < len(got); i++ {
		if got[i].Urgent && !got[i-1].Urgent {
			t.Fatalf("срочные напоминания должны идти первыми: %+v", got)
		}
		if got[i].Urgent == got[i-1].Urgent && got[i].At.Before(got[i-1].At) {
			t.Fatalf("нарушен порядок по времени: %+v", got)
		}
	}
	if r := texts["rem-1on1"]; r.ItemID != "1on1" || r.Kind != PlannerMeeting {
		t.Errorf("связь с событием: %+v", r)
	}
	if BuildReminders(nil, nil, testNow) != nil {
		t.Error("без событий лента пуста")
	}
}

func TestDeflectionRate(t *testing.T) {
	ev := func(key string) TelemetryEvent {
		return TelemetryEvent{Kind: EventKindAgent, Payload: map[string]any{"saving_key": key}}
	}
	m := ComputeMetrics(MetricsInput{
		Events:        []TelemetryEvent{ev(SavingQuestion), ev(SavingQuestion), ev(SavingQuestion)},
		OpenQuestions: 1, AccessGrants: 2, TasksDone: 1, TasksTotal: 6,
	})
	if m.DeflectionRate != 75 || m.AccessGrants != 2 || m.TasksDone != 1 || m.TasksTotal != 6 {
		t.Fatalf("метрики: %+v", m)
	}
	if ComputeMetrics(MetricsInput{}).DeflectionRate != 0 {
		t.Fatal("без вопросов показатель самостоятельности равен 0")
	}
}
