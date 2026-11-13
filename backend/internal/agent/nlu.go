// Package agent — оркестратор ИИ-помощника: детерминированный Smart Engine по спецификации
// (specs/01_agent_behavior.openspec.yaml) и опциональная OpenAI-совместимая LLM с fallback.
package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/spec"
)

// Идентификаторы интентов (совпадают со спецификацией).
const (
	IntentRestricted  = "restricted_data_export"
	IntentAbsence     = "absence_day_off"
	IntentAccess      = "access_request"
	IntentProcess     = "process_question"
	IntentTaskHelp    = "task_help"
	IntentNextStep    = "next_step"
	IntentMentor      = "mentor_contact"
	IntentGreeting    = "greeting"
	IntentAffirmative = "affirmative"
	IntentKnowledge   = "knowledge_search"
)

// Classify определяет интент: первый по приоритету интент, у которого сработал триггер.
// Интенты в спецификации отсортированы загрузчиком по убыванию приоритета.
func Classify(s *spec.AgentSpec, text string) spec.Intent {
	for _, in := range s.Intents {
		for _, trigger := range in.Triggers {
			if domain.MatchTrigger(trigger, text) {
				return in
			}
		}
	}
	return s.FallbackIntent
}

var repoRe = regexp.MustCompile(`[a-z][a-z0-9]*(?:-[a-z0-9]+)+`)

// ExtractRepository достаёт имя репозитория из текста (предпочитая имена с avito-).
func ExtractRepository(text string) string {
	matches := repoRe.FindAllString(strings.ToLower(text), -1)
	for _, m := range matches {
		if strings.HasPrefix(m, "avito-") {
			return m
		}
	}
	for _, m := range matches {
		if len(m) >= 6 && m != "merge-request" && m != "pull-request" && m != "one-on-one" {
			return m
		}
	}
	return ""
}

// AbsenceDraft — черновик заявки на отгул, извлечённый из текста.
type AbsenceDraft struct {
	Date      string
	TimeFrom  string
	TimeTo    string
	Reason    string
	DateFound bool
}

var (
	explicitDateRe = regexp.MustCompile(`(?:^|\D)(\d{1,2})[./](\d{1,2})(?:[./](\d{4}))?(?:\D|$)`)
	timeRangeRe    = regexp.MustCompile(`с\s*(\d{1,2})(?::(\d{2}))?\s*(?:до|по)\s*(\d{1,2})(?::(\d{2}))?`)
	weekdays       = []struct {
		stem string
		day  time.Weekday
	}{
		{"понедельник", time.Monday}, {"вторник", time.Tuesday}, {"сред", time.Wednesday},
		{"четверг", time.Thursday}, {"пятниц", time.Friday}, {"суббот", time.Saturday}, {"воскресен", time.Sunday},
	}
	weekdayShort = []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
)

// ParseAbsence извлекает дату, время и причину отгула из свободного текста.
func ParseAbsence(text string, now time.Time) AbsenceDraft {
	normalized, tokens := domain.Normalize(text), domain.Tokens(text)
	d := AbsenceDraft{TimeFrom: "10:00", TimeTo: "19:00", Reason: "Личные обстоятельства"}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	setDate := func(t time.Time) {
		d.Date, d.DateFound = t.Format("2006-01-02"), true
	}
	switch m := explicitDateRe.FindStringSubmatch(normalized); {
	case m != nil:
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		year := today.Year()
		if m[3] != "" {
			year, _ = strconv.Atoi(m[3])
		}
		t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
		if t.Day() == day && int(t.Month()) == month { // отсекаем 31.02 и подобное
			if m[3] == "" && t.Before(today) {
				t = t.AddDate(1, 0, 0)
			}
			setDate(t)
		}
	case domain.MatchTerm("послезавтра$", normalized, tokens):
		setDate(today.AddDate(0, 0, 2))
	case domain.MatchTerm("завтра$", normalized, tokens):
		setDate(today.AddDate(0, 0, 1))
	case domain.MatchTerm("сегодня$", normalized, tokens):
		setDate(today)
	default:
		for _, w := range weekdays {
			if domain.MatchTerm(w.stem, normalized, tokens) {
				ahead := (int(w.day) - int(today.Weekday()) + 7) % 7
				if ahead == 0 {
					ahead = 7
				}
				setDate(today.AddDate(0, 0, ahead))
				break
			}
		}
	}

	switch m := timeRangeRe.FindStringSubmatch(normalized); {
	case m != nil:
		from, to := clock(m[1], m[2]), clock(m[3], m[4])
		if from != "" && to != "" && to > from {
			d.TimeFrom, d.TimeTo = from, to
		}
	case strings.Contains(normalized, "после обеда") || strings.Contains(normalized, "второй половин"):
		d.TimeFrom, d.TimeTo = "14:00", "18:00"
	case strings.Contains(normalized, "до обеда") || strings.Contains(normalized, "первой половин") || domain.MatchTerm("утром$", normalized, tokens):
		d.TimeFrom, d.TimeTo = "10:00", "14:00"
	}

	switch {
	case domain.MatchGroup("зачет", normalized, tokens):
		d.Reason = "Зачёт в университете"
	case domain.MatchGroup("экзамен|сесси", normalized, tokens):
		d.Reason = "Экзамен в университете"
	case domain.MatchGroup("врач|поликлиник|стоматолог|больниц", normalized, tokens):
		d.Reason = "Визит к врачу"
	case domain.MatchGroup("универ|учеб|пары$|диплом", normalized, tokens):
		d.Reason = "Учёба в университете"
	}
	return d
}

func clock(h, m string) string {
	hour, err := strconv.Atoi(h)
	if err != nil || hour > 23 {
		return ""
	}
	minute := 0
	if m != "" {
		minute, _ = strconv.Atoi(m)
		if minute > 59 {
			return ""
		}
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// HumanDay форматирует дату YYYY-MM-DD как «пт, 13.11».
func HumanDay(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%s, %s", weekdayShort[t.Weekday()], t.Format("02.01"))
}

func truncate(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max])
}
