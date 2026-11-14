package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Виды и статусы элементов планировщика.
const (
	PlannerMeeting  = "meeting"
	PlannerReview   = "review"
	PlannerSync     = "sync"
	PlannerDeadline = "deadline"
	PlannerTask     = "task"

	PlannerPlanned = "planned"
	PlannerDone    = "done"

	NotificationAbsenceApproved = "absence_approved"
	NotificationAbsenceRejected = "absence_rejected"
	NotificationTaskAssigned    = "task_assigned"
)

// PlannerItem — событие календаря или задача адаптации стажёра.
type PlannerItem struct {
	ID            string     `json:"id"`
	InternID      string     `json:"intern_id"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	StartsAt      time.Time  `json:"starts_at"`
	DurationMin   int        `json:"duration_min"`
	AllDay        bool       `json:"all_day"`
	Link          string     `json:"link"`
	Mandatory     bool       `json:"mandatory"`
	CreatedBy     string     `json:"created_by"`
	CreatedByName string     `json:"created_by_name"`
	Status        string     `json:"status"`
	Report        string     `json:"report"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	IsSynthetic   bool       `json:"is_synthetic"`
}

// IsEvent сообщает, что элемент — встреча (а не задача или дедлайн).
func (p PlannerItem) IsEvent() bool {
	return p.Kind == PlannerMeeting || p.Kind == PlannerReview || p.Kind == PlannerSync
}

// Validate проверяет новый элемент: название и дата не в прошлом.
func (p PlannerItem) Validate(now time.Time) error {
	if strings.TrimSpace(p.InternID) == "" {
		return fmt.Errorf("%w: не указан сотрудник", ErrValidation)
	}
	if n := len([]rune(strings.TrimSpace(p.Title))); n < 3 || n > 120 {
		return fmt.Errorf("%w: название должно быть от 3 до 120 символов", ErrValidation)
	}
	if dayStart(p.StartsAt).Before(dayStart(now)) {
		return fmt.Errorf("%w: дата уже прошла", ErrValidation)
	}
	return nil
}

// Complete отмечает элемент выполненным и сохраняет отчёт.
func (p PlannerItem) Complete(report string, now time.Time) (PlannerItem, error) {
	if p.Status == PlannerDone {
		return p, fmt.Errorf("%w: «%s» уже отмечено выполненным", ErrValidation, p.Title)
	}
	p.Status, p.Report = PlannerDone, strings.TrimSpace(report)
	t := now
	p.CompletedAt = &t
	return p, nil
}

// Reminder — напоминание для ленты и всплывающих уведомлений.
type Reminder struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
	ItemID string    `json:"item_id,omitempty"`
	Urgent bool      `json:"urgent"`
}

// MaxReminders — размер ленты напоминаний.
const MaxReminders = 8

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func pluralRu(n int, one, few, many string) string {
	n100, n10 := n%100, n%10
	switch {
	case n100 > 10 && n100 < 20:
		return many
	case n10 == 1:
		return one
	case n10 > 1 && n10 < 5:
		return few
	}
	return many
}

var weekdayAccusative = []string{"в воскресенье", "в понедельник", "во вторник", "в среду", "в четверг", "в пятницу", "в субботу"}

// RelativeWhen описывает момент относительно «сейчас»: «через 3 часа», «завтра в 15:00», «в пятницу».
func RelativeWhen(at, now time.Time, withTime bool) string {
	d := at.Sub(now)
	days := int(dayStart(at).Sub(dayStart(now)).Hours() / 24)
	clock := ""
	if withTime {
		clock = " в " + at.Format("15:04")
	}
	switch {
	case withTime && d >= 0 && d < time.Hour:
		m := int(d.Minutes())
		if m < 1 {
			return "прямо сейчас"
		}
		return fmt.Sprintf("через %d мин", m)
	case withTime && d >= 0 && d < 12*time.Hour:
		h := int((d + 30*time.Minute).Hours())
		return fmt.Sprintf("через %d %s", h, pluralRu(h, "час", "часа", "часов"))
	case days == 0:
		return "сегодня" + clock
	case days == 1:
		return "завтра" + clock
	case days > 1 && days < 7:
		return weekdayAccusative[at.Weekday()] + clock
	}
	return at.Format("02.01") + clock
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// BuildReminders формирует ленту напоминаний из календаря и уведомлений стажёра.
func BuildReminders(items []PlannerItem, notes []Notification, now time.Time) []Reminder {
	var out []Reminder
	horizon := now.Add(7 * 24 * time.Hour)
	for _, it := range items {
		if it.Status == PlannerDone || it.StartsAt.After(horizon) {
			continue
		}
		deadline := it.StartsAt
		if it.AllDay {
			deadline = dayStart(it.StartsAt).Add(24*time.Hour - time.Minute)
		}
		r := Reminder{ID: "rem-" + it.ID, Kind: it.Kind, At: it.StartsAt, ItemID: it.ID}
		soon := deadline.Sub(now) < 24*time.Hour
		switch {
		case deadline.Before(now) && it.IsEvent():
			continue // прошедшие встречи не напоминаем
		case deadline.Before(now):
			r.Text, r.Urgent = fmt.Sprintf("⏰ Просрочено: %s", it.Title), true
		case it.IsEvent() && soon:
			r.Text, r.Urgent = fmt.Sprintf("🚨 %s — %s", upperFirst(RelativeWhen(it.StartsAt, now, true)), it.Title), true
		case it.IsEvent():
			r.Text = fmt.Sprintf("🗓 %s — %s", upperFirst(RelativeWhen(it.StartsAt, now, true)), it.Title)
		case it.Kind == PlannerDeadline:
			r.Text, r.Urgent = fmt.Sprintf("📅 %s — %s", it.Title, RelativeWhen(it.StartsAt, now, false)), soon
		case it.Mandatory:
			r.Text, r.Urgent = fmt.Sprintf("📌 Обязательная задача «%s» — %s", it.Title, RelativeWhen(it.StartsAt, now, false)), soon
		default:
			r.Text = fmt.Sprintf("✅ Твоя задача «%s» — %s", it.Title, RelativeWhen(it.StartsAt, now, false))
		}
		out = append(out, r)
	}
	for _, n := range notes {
		if n.Kind != NotificationAbsenceApproved && n.Kind != NotificationAbsenceRejected && n.Kind != NotificationTaskAssigned {
			continue
		}
		out = append(out, Reminder{ID: "rem-" + n.ID, Kind: n.Kind, Text: "🔔 " + n.Body, At: n.CreatedAt, Urgent: n.Kind != NotificationTaskAssigned})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Urgent != out[j].Urgent {
			return out[i].Urgent
		}
		return out[i].At.Before(out[j].At)
	})
	if len(out) > MaxReminders {
		out = out[:MaxReminders]
	}
	return out
}
