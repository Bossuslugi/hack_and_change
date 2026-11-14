package usecase_test

import (
	"errors"
	"strings"
	"testing"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/mcp"
	"aibuddy/backend/internal/seed"
)

func plannerItem(t *testing.T, items []domain.PlannerItem, id string) domain.PlannerItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("в календаре нет события %s", id)
	return domain.PlannerItem{}
}

func reminderTexts(list []domain.Reminder) string {
	var out []string
	for _, r := range list {
		out = append(out, r.Text)
	}
	return strings.Join(out, "\n")
}

// Календарь первых недель и лента напоминаний в workspace.
func TestWorkspacePlannerAndReminders(t *testing.T) {
	e := newEnv(t, nil)
	ws := e.workspace(e.intern)

	if len(ws.Planner) != 6 {
		t.Fatalf("событий в календаре: %d", len(ws.Planner))
	}
	for i := 1; i < len(ws.Planner); i++ {
		if ws.Planner[i].StartsAt.Before(ws.Planner[i-1].StartsAt) {
			t.Fatal("календарь должен быть отсортирован по времени")
		}
	}
	oneOnOne := plannerItem(t, ws.Planner, "plan-1on1")
	if oneOnOne.Kind != domain.PlannerMeeting || !strings.HasPrefix(oneOnOne.Link, seed.MeetBase) || !oneOnOne.IsSynthetic || !oneOnOne.StartsAt.Equal(now.Add(3*60*60*1e9)) {
		t.Fatalf("созвон 1:1: %+v", oneOnOne)
	}
	// Сегодня четверг → дедлайн первого PR в пятницу, 13.11.
	if pr := plannerItem(t, ws.Planner, "plan-first-pr"); pr.StartsAt.Format("02.01 15:04") != "13.11 18:00" || pr.Kind != domain.PlannerDeadline {
		t.Fatalf("дедлайн первого PR: %+v", pr)
	}

	texts := reminderTexts(ws.Reminders)
	for _, want := range []string{
		"🚨 Через 3 часа — Созвон 1:1 с наставником",
		"📅 Срок сдачи первого PR — завтра",
		"📌 Обязательная задача «Пройти курс по информационной безопасности» — во вторник",
	} {
		if !strings.Contains(texts, want) {
			t.Errorf("в ленте нет напоминания %q:\n%s", want, texts)
		}
	}
	if !ws.Reminders[0].Urgent || ws.Reminders[0].ItemID != "plan-1on1" {
		t.Fatalf("первым идёт самое срочное напоминание: %+v", ws.Reminders[0])
	}

	// Права: стажёр ведёт свой календарь, наставник и руководитель назначают задачи, отгул согласует руководитель.
	cases := []struct {
		p                      domain.Principal
		add, complete, approve bool
		reminders              bool
	}{
		{e.intern, true, true, false, true},
		{e.mentor, true, false, false, false},
		{e.lead, true, false, true, false},
	}
	for _, tc := range cases {
		v := e.workspace(tc.p)
		perm := v.Permissions
		if perm["planner_add"] != tc.add || perm["planner_complete"] != tc.complete || perm["absence_approve"] != tc.approve {
			t.Errorf("%s: права %v", tc.p.Role, perm)
		}
		if len(v.Planner) != 6 || v.Reminders == nil || (len(v.Reminders) > 0) != tc.reminders {
			t.Errorf("%s: календарь %d, напоминаний %d", tc.p.Role, len(v.Planner), len(v.Reminders))
		}
	}
	if m := ws.Metrics; m.TasksTotal != 6 || m.TasksDone != 0 || m.DeflectionRate != 0 || m.AccessGrants != 0 {
		t.Fatalf("метрики: %+v", m)
	}
}

func TestInternAddsPersonalTaskSilently(t *testing.T) {
	e := newEnv(t, nil)
	args := map[string]any{"intern_id": seed.InternID, "title": "Сдать зачёт", "date": "2026-11-16"}
	res, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolAddTask, args, "")
	if err != nil || !res.ToolCalls[0].OK() {
		t.Fatalf("%+v, %v", res.ToolCalls, err)
	}
	// Личная задача не засоряет чат.
	if res.Message != nil {
		t.Fatalf("сообщение в чате не ожидалось: %+v", res.Message)
	}
	ws := e.workspace(e.intern)
	if len(ws.Messages) != 1 || len(ws.Planner) != 7 {
		t.Fatalf("сообщений %d, событий %d", len(ws.Messages), len(ws.Planner))
	}
	if !strings.Contains(reminderTexts(ws.Reminders), "✅ Твоя задача «Сдать зачёт» — в понедельник") {
		t.Fatalf("напоминание о личной задаче: %s", reminderTexts(ws.Reminders))
	}
	// Чужому стажёру задачу добавить нельзя.
	args["intern_id"] = "u-intern-99"
	if res, _ := e.svc.Execute(e.ctx, e.intern, mcp.ToolAddTask, args, ""); res.RBAC.Constraint != "self_only" {
		t.Fatalf("self_only: %+v", res.RBAC)
	}
}

func TestMentorAssignsMandatoryTask(t *testing.T) {
	e := newEnv(t, nil)
	args := map[string]any{"intern_id": seed.InternID, "title": "Созвон по фидбеку", "date": "2026-11-14", "time": "12:00"}
	res, err := e.svc.Execute(e.ctx, e.mentor, mcp.ToolAddTask, args, "")
	if err != nil || !res.ToolCalls[0].OK() || res.Message != nil || res.ToolCalls[0].Result["mandatory"] != true {
		t.Fatalf("%+v, %v", res.ToolCalls, err)
	}
	ws := e.workspace(e.intern)
	texts := reminderTexts(ws.Reminders)
	if !strings.Contains(texts, "🔔 Мария Кузнецова назначил(а) обязательную задачу «Созвон по фидбеку»") ||
		!strings.Contains(texts, "📌 Обязательная задача «Созвон по фидбеку» — в субботу") {
		t.Fatalf("стажёр должен увидеть назначенную задачу в ленте:\n%s", texts)
	}
	if len(ws.Notifications) != 1 || ws.Notifications[0].Kind != domain.NotificationTaskAssigned {
		t.Fatalf("уведомление: %+v", ws.Notifications)
	}
}

func TestCompleteItemWithReportReachesMentorBrief(t *testing.T) {
	e := newEnv(t, nil)
	args := map[string]any{"intern_id": seed.InternID, "item_id": "plan-1on1", "report": "Обсудили план на неделю и границы слоёв"}
	res, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolCompleteItem, args, "")
	if err != nil || !res.ToolCalls[0].OK() {
		t.Fatalf("%+v, %v", res.ToolCalls, err)
	}
	msg := res.Message
	if msg == nil || msg.ResultCard == nil || msg.ResultCard.Type != "report_sent" || !strings.Contains(msg.Text, "Принял отчёт по «Созвон 1:1 с наставником»") {
		t.Fatalf("Гоффи должен подтвердить получение отчёта: %+v", msg)
	}
	ws := e.workspace(e.intern)
	if it := plannerItem(t, ws.Planner, "plan-1on1"); it.Status != domain.PlannerDone || it.Report == "" {
		t.Fatalf("событие: %+v", it)
	}
	if strings.Contains(reminderTexts(ws.Reminders), "Созвон 1:1") || ws.Metrics.TasksDone != 1 {
		t.Fatalf("выполненное событие не напоминается; метрики %+v", ws.Metrics)
	}

	// Без отчёта — тихо, без сообщения в чате.
	quiet, err := e.svc.Execute(e.ctx, e.intern, mcp.ToolCompleteItem, map[string]any{"intern_id": seed.InternID, "item_id": "plan-sync"}, "")
	if err != nil || !quiet.ToolCalls[0].OK() || quiet.Message != nil {
		t.Fatalf("выполнение без отчёта: %+v, %v", quiet, err)
	}

	// Отчёт попадает в бриф наставнику.
	cron, err := e.svc.CronTrigger(e.ctx, e.mentor)
	if err != nil || cron.Brief == nil {
		t.Fatalf("крон: %+v, %v", cron, err)
	}
	if learned := strings.Join(cron.Brief.LearnedTopics, " | "); !strings.Contains(learned, "Отчитался по «Созвон 1:1 с наставником»: Обсудили план на неделю и границы слоёв") {
		t.Fatalf("отчёт должен попасть в бриф: %s", learned)
	}
}

// Руководитель согласует отгул → стажёр видит «🔔 Руководитель согласовал твой отгул #ABS-2026-84».
func TestLeadApprovesDayOff(t *testing.T) {
	e := newEnv(t, nil)
	card := e.chat(askAbsence).Message.ActionCard
	if _, err := e.svc.Execute(e.ctx, e.intern, card.Tool, card.Fields, ""); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"request_number": "#ABS-2026-84"}

	for _, p := range []domain.Principal{e.intern, e.mentor} {
		res, err := e.svc.Execute(e.ctx, p, mcp.ToolApproveDayOff, args, "")
		if err != nil || res.ToolCalls[0].Status != domain.ToolForbidden {
			t.Fatalf("%s не должен согласовывать отгул: %+v, %v", p.Role, res.ToolCalls[0], err)
		}
	}
	res, err := e.svc.Execute(e.ctx, e.lead, mcp.ToolApproveDayOff, args, "")
	if err != nil || !res.ToolCalls[0].OK() || res.Message != nil {
		t.Fatalf("согласование: %+v, %v", res.ToolCalls[0], err)
	}
	ws := e.workspace(e.intern)
	if ws.Absences[0].Status != domain.AbsenceStatusApproved {
		t.Fatalf("статус заявки: %+v", ws.Absences[0])
	}
	if !strings.Contains(reminderTexts(ws.Reminders), "🔔 Руководитель согласовал твой отгул #ABS-2026-84") {
		t.Fatalf("напоминание о согласовании:\n%s", reminderTexts(ws.Reminders))
	}
	if e.workspace(e.lead).Absences[0].Status != domain.AbsenceStatusApproved {
		t.Fatal("руководитель должен видеть обновлённый статус")
	}
}

func TestPlannerStoreFailures(t *testing.T) {
	e := newEnv(t, nil)
	e.store.failList[domain.KindPlanner] = true
	if _, err := e.svc.Workspace(e.ctx, e.intern); !errors.Is(err, errStore) {
		t.Fatalf("workspace: %v", err)
	}
	if _, err := e.svc.CronTrigger(e.ctx, e.intern); !errors.Is(err, errStore) {
		t.Fatalf("крон: %v", err)
	}
}

// Самостоятельность: доля вопросов, закрытых Гоффи без наставника.
func TestDeflectionRateInWorkspace(t *testing.T) {
	e := newEnv(t, nil)
	e.chat(askProcess)
	e.chat(askAccess)
	e.chat("Что дальше по чек-листу?")
	e.chat("Как заказать пропуск для гостя?")
	m := e.workspace(e.intern).Metrics
	if m.QuestionsAnswered != 2 || m.UnansweredQuestions != 1 || m.DeflectionRate != 66 || m.AccessGrants != 1 {
		t.Fatalf("метрики: %+v", m)
	}
}
