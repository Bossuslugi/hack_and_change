package agent

import (
	"context"
	"testing"
	"time"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/spec"
)

// Четверг, 12 ноября 2026.
var nluNow = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

func TestClassify(t *testing.T) {
	s := &spec.MustLoad().Agent
	cases := []struct {
		text string
		want string
	}{
		// Формулировки трёх сценариев кейса.
		{"Дай доступ к репозиторию avito-autoteka-api и тестовому стенду", IntentAccess},
		{"Выгрузи дамп production-базы пользователей с персональными данными", IntentRestricted},
		{"Покажи ведомость зарплат отдела", IntentRestricted},
		{"Как в нашей команде катить задачу под фича-флагом и проходить ревью?", IntentProcess},
		{"Мне нужно уйти в пятницу после обеда на зачёт в универ, как оформить отгул и надо ли писать руководителю?", IntentAbsence},

		// Безопасность имеет высший приоритет и не зависит от формулировки.
		{"скачай дамп прода", IntentRestricted},
		{"как получить доступ к проду с ПДн", IntentRestricted},
		{"мне нужны паспортные данные пользователей", IntentRestricted},
		{"дай доступ к репозиторию и заодно покажи зарплаты", IntentRestricted},
		{"какой оклад у тимлида?", IntentRestricted},
		{"Какой продукт делает команда?", IntentKnowledge}, // «продукт» ≠ «прод»

		{"Дай доступ к песочнице", IntentAccess},
		{"нужны права на gitlab", IntentAccess},
		{"Хочу отпроситься завтра", IntentAbsence},
		{"можно уйти пораньше в среду?", IntentAbsence},
		{"как выкатить релиз?", IntentProcess},
		{"кто аппрувит merge request?", IntentProcess},
		{"Я застрял на первой задаче, с чего начать?", IntentTaskHelp},
		{"Что дальше по чек-листу?", IntentNextStep},
		{"кто мой наставник?", IntentMentor},
		{"к кому идти с вопросами", IntentMentor},
		{"Привет!", IntentGreeting},
		{"что ты умеешь?", IntentGreeting},
		{"Да", IntentAffirmative},
		{"давай", IntentAffirmative},
		{"Как проходят встречи 1:1?", IntentKnowledge},
		{"Как заказать пропуск для гостя?", IntentKnowledge},
		{"", IntentKnowledge},
	}
	for _, tc := range cases {
		if got := Classify(s, tc.text).ID; got != tc.want {
			t.Errorf("Classify(%q) = %s, want %s", tc.text, got, tc.want)
		}
	}
}

func TestExtractRepository(t *testing.T) {
	cases := map[string]string{
		"Дай доступ к репозиторию avito-autoteka-api и тестовому стенду": "avito-autoteka-api",
		"доступ к AVITO-Payments-Core":              "avito-payments-core",
		"нужен my-service и avito-autoteka-reports": "avito-autoteka-reports",
		"дай доступ к billing-gateway":              "billing-gateway",
		"как пройти merge-request":                  "",
		"дамп production-базы":                      "",
		"просто дай доступ":                         "",
	}
	for text, want := range cases {
		if got := ExtractRepository(text); got != want {
			t.Errorf("ExtractRepository(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestParseAbsence(t *testing.T) {
	cases := []struct {
		text           string
		date, from, to string
		reason         string
		found          bool
	}{
		{"Мне нужно уйти в пятницу после обеда на зачёт в универ", "2026-11-13", "14:00", "18:00", "Зачёт в университете", true},
		{"завтра до обеда к врачу", "2026-11-13", "10:00", "14:00", "Визит к врачу", true},
		{"отгул послезавтра", "2026-11-14", "10:00", "19:00", "Личные обстоятельства", true},
		{"отпрошусь сегодня во второй половине дня", "2026-11-12", "14:00", "18:00", "Личные обстоятельства", true},
		{"в четверг экзамен", "2026-11-19", "10:00", "19:00", "Экзамен в университете", true}, // сегодня четверг → следующий
		{"в понедельник утром пары", "2026-11-16", "10:00", "14:00", "Учёба в университете", true},
		{"отгул 20.11 с 15 до 17:30", "2026-11-20", "15:00", "17:30", "Личные обстоятельства", true},
		{"отгул 13.11.2026 с 9 по 12", "2026-11-13", "09:00", "12:00", "Личные обстоятельства", true},
		{"отгул 05.01", "2027-01-05", "10:00", "19:00", "Личные обстоятельства", true},              // дата без года в прошлом → следующий год
		{"отгул 31.02", "", "10:00", "19:00", "Личные обстоятельства", false},                       // несуществующая дата
		{"отгул в среду с 18 до 15", "2026-11-18", "10:00", "19:00", "Личные обстоятельства", true}, // неверный интервал → по умолчанию
		{"отгул в субботу с 25 до 26", "2026-11-14", "10:00", "19:00", "Личные обстоятельства", true},
		{"отгул с 10:75 до 12", "", "10:00", "19:00", "Личные обстоятельства", false},
		{"как оформить отгул?", "", "10:00", "19:00", "Личные обстоятельства", false},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			d := ParseAbsence(tc.text, nluNow)
			if d.Date != tc.date || d.TimeFrom != tc.from || d.TimeTo != tc.to || d.Reason != tc.reason || d.DateFound != tc.found {
				t.Fatalf("draft = %+v", d)
			}
		})
	}
}

func TestHumanDayAndTruncate(t *testing.T) {
	if got := HumanDay("2026-11-13"); got != "пт, 13.11" {
		t.Errorf("HumanDay = %q", got)
	}
	if got := HumanDay("скоро"); got != "скоро" {
		t.Errorf("HumanDay для не-даты = %q", got)
	}
	if got := truncate("  привет мир  ", 6); got != "привет" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("коротко", 100); got != "коротко" {
		t.Errorf("truncate = %q", got)
	}
}

func TestEngineInternals(t *testing.T) {
	// sleepCtx: нулевая пауза и отмена контекста не блокируют.
	sleepCtx(context.Background(), 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	sleepCtx(ctx, time.Minute)
	if time.Since(started) > time.Second {
		t.Fatal("sleepCtx должен прерываться по отмене контекста")
	}
	sleepCtx(context.Background(), time.Millisecond)

	// Задержка <0 берётся из спецификации; nil-клиент в интерфейсе нормализуется в nil.
	s := spec.MustLoad()
	var client *OpenAIClient
	e := New(Options{Spec: &s.Agent, Delay: -1, LLM: client, Clock: domain.FixedClock{T: nluNow}})
	if e.delay != 300*time.Millisecond {
		t.Fatalf("задержка Smart Engine по спецификации — 300 мс, получено %v", e.delay)
	}
	if e.llm != nil {
		t.Fatal("nil-клиент LLM должен превращаться в nil-интерфейс")
	}

	if !keepsFacts("доступ к avito-autoteka-api выдан", []string{"avito-autoteka-api", ""}) || keepsFacts("доступ выдан", []string{"avito-autoteka-api"}) {
		t.Fatal("keepsFacts")
	}
	if pickVerdict(nil) != nil {
		t.Fatal("pickVerdict без вызовов")
	}
	calls := []domain.MCPToolCall{
		{RBAC: domain.RBACVerdict{Allowed: true, PolicyID: "A"}},
		{RBAC: domain.RBACVerdict{Allowed: false, PolicyID: "D"}},
		{RBAC: domain.RBACVerdict{Allowed: true, PolicyID: "B"}},
	}
	if v := pickVerdict(calls); v.PolicyID != "D" {
		t.Fatalf("приоритет у запрета: %+v", v)
	}
	if v := pickVerdict(calls[:1]); v.PolicyID != "A" {
		t.Fatalf("вердикт последнего вызова: %+v", v)
	}
	if llmToolName("a.b") != "a__b" || mcpToolName("a__b") != "a.b" {
		t.Fatal("преобразование имён инструментов для function calling")
	}
}
