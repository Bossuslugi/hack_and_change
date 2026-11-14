// Package seed содержит синтетические демо-данные Гоффи.
// Все люди, команды, инструкции и задачи вымышлены; реальных ПДн и регламентов Авито здесь нет.
package seed

import (
	"time"

	"aibuddy/backend/internal/domain"
)

// Идентификаторы демо-пользователей и тестовые токены (тестовые учётные данные из README).
const (
	InternID    = "u-intern-01"
	MentorID    = "u-mentor-01"
	TeamLeadID  = "u-lead-01"
	CronID      = "svc-ai-cron"
	InternToken = "demo-intern-token"
	MentorToken = "demo-mentor-token"
	LeadToken   = "demo-lead-token"
	Team        = "Autoteka"
	TeamRepo    = "avito-autoteka-api"
)

const playbookSource = "Синтетическая адаптация по мотивам AvitoTech Playbook (github.com/avito-tech/playbook)"

func doc(id, title, summary string, topics, keywords, steps []string, audience ...domain.Role) domain.KnowledgeDoc {
	return domain.KnowledgeDoc{
		ID: id, Title: title, Summary: summary, Steps: steps, Topics: topics, Keywords: keywords,
		Team: Team, Audience: audience, Source: playbookSource,
		IsSynthetic: true, Disclaimer: domain.SyntheticDisclaimer,
	}
}

// Data возвращает исходное состояние демо.
func Data() domain.SeedData {
	return domain.SeedData{
		Employees: []domain.Employee{
			{ID: InternID, Name: "Алексей Смирнов", Role: domain.RoleInternBackend, RoleTitle: "Стажёр (Backend)", Team: Team, Token: InternToken, IsSynthetic: true},
			{ID: MentorID, Name: "Мария Кузнецова", Role: domain.RoleMentor, RoleTitle: "Наставник (buddy)", Team: Team, Token: MentorToken, IsSynthetic: true},
			{ID: TeamLeadID, Name: "Дмитрий Орлов", Role: domain.RoleTeamLead, RoleTitle: "Руководитель команды", Team: Team, Token: LeadToken, IsSynthetic: true},
			// Сервисный аккаунт крона: без токена, снаружи аутентифицироваться им нельзя.
			{ID: CronID, Name: "ИИ-крон", Role: domain.RoleSystemCron, RoleTitle: "ИИ-крон (сервисный аккаунт)", Team: Team, IsSynthetic: true},
		},
		Intern: domain.InternProfile{
			ID: InternID, Name: "Алексей Смирнов", Role: domain.RoleInternBackend,
			Team: Team, Stack: "Go", Stage: "Week 1", Program: "Стажировка Backend, 6 месяцев",
			StartDate: "2026-11-09", Repository: TeamRepo,
			MentorID: MentorID, MentorName: "Мария Кузнецова",
			TeamLeadID: TeamLeadID, TeamLeadName: "Дмитрий Орлов", IsSynthetic: true,
		},
		Checklist: []domain.ChecklistItem{
			{ID: "access_repo", Position: 1, Week: 1, Status: domain.StatusPending, Scenario: "S1",
				Title:       "Получить доступы к репозиторию команды и тестовому стенду",
				Description: "Доступ к avito-autoteka-api и стенду с синтетическими данными. Гоффи оформит через IAM."},
			{ID: "meet_team", Position: 2, Week: 1, Status: domain.StatusCompleted,
				Title:       "Познакомиться с командой и наставником",
				Description: "Вводная встреча с командой Autoteka, знакомство с наставником и руководителем.",
				Evidence:    "Вводная встреча проведена в первый день"},
			{ID: "local_env", Position: 3, Week: 1, Status: domain.StatusCompleted,
				Title:       "Настроить рабочее окружение",
				Description: "Go, Docker, IDE и корпоративный VPN на рабочем ноутбуке.",
				Evidence:    "Окружение настроено по инструкции"},
			{ID: "team_processes", Position: 4, Week: 1, Status: domain.StatusPending, Scenario: "S2",
				Title:       "Изучить процессы команды: фича-флаги и код-ревью",
				Description: "Как команда катит задачи под фича-флагами и как проходит ревью. Спроси Гоффи."},
			{ID: "first_task", Position: 5, Week: 1, Status: domain.StatusInProgress, Scenario: "S2",
				Title:       "Первая задача в спринте",
				Description: "AUT-1423: добавить поле mileage_verified в сводку отчёта под фича-флагом.",
				Evidence:    "Задача взята в работу"},
			{ID: "absence_rules", Position: 6, Week: 2, Status: domain.StatusPending, Scenario: "S3",
				Title:       "Разобраться с оформлением отсутствий",
				Description: "Отгул, отпуск, больничный: как оформить через сервис «Отсутствия» и кого предупредить."},
			{ID: "first_one_on_one", Position: 7, Week: 2, Status: domain.StatusPending, Scenario: "S2",
				Title:       "Провести первую встречу 1:1 с наставником",
				Description: "Обсудить прогресс и вопросы, требующие участия сеньора. Гоффи подготовит бриф."},
		},
		Task: domain.SprintTask{
			Key: "AUT-1423", InternID: InternID, Status: string(domain.StatusInProgress),
			Title:          "Первая задача в спринте",
			Summary:        "Добавить поле mileage_verified в ответ ручки /v1/report/summary под фича-флагом",
			Commits:        0,
			DaysInProgress: 3,
			Repository:     TeamRepo,
			ArchitectureQuestion: "Где в avito-autoteka-api проходит граница между слоем сборки отчёта и агрегатором источников: " +
				"на каком слое вычислять mileage_verified, чтобы не сломать контракт /v1/report/summary для существующих клиентов?",
		},
		Planner: Planner,
		Repositories: map[string]string{
			"avito-autoteka-api":      Team,
			"avito-autoteka-reports":  Team,
			"avito-payments-core":     "Payments",
			"avito-messenger-gateway": "Messenger",
		},
		Flags: []domain.FeatureFlag{
			{Key: "ai_buddy.proactive_cron", Enabled: true, Kind: domain.FlagRuntime, Description: "ИИ-крон: проактивные сообщения стажёру и бриф наставнику"},
			{Key: "ai_buddy.absence_action_card", Enabled: true, Kind: domain.FlagRuntime, Description: "Интерактивная карточка оформления отгула в чате"},
			{Key: "ai_buddy.llm_answers", Enabled: true, Kind: domain.FlagRuntime, Description: "Формулировка ответов внешней LLM (если задан LLM_API_KEY)"},
			{Key: "ai_buddy.mcp_jsonrpc", Enabled: true, Kind: domain.FlagCompileTime, Description: "MCP JSON-RPC endpoint POST /mcp"},
			{Key: "ai_buddy.synthetic_data_only", Enabled: true, Kind: domain.FlagCompileTime, Description: "Только синтетические данные и имитации сервисов"},
		},
		Docs: []domain.KnowledgeDoc{
			doc("pb-feature-flags", "Как катить задачу под фича-флагом",
				"Любое изменение поведения в команде выкатывается выключенным под runtime-флагом и включается постепенно.",
				[]string{"feature_flags"},
				[]string{"фича", "feature", "флаг", "фичефлаг", "фичафлаг", "катить", "выкат", "раскат", "деплой", "релиз", "зарелиз"},
				[]string{
					"Заведи runtime-флаг в сервисе Feature Flags: имя autoteka.<задача>, по умолчанию выключен.",
					"Оберни новое поведение в проверку флага; старый путь оставь без изменений и покрой оба пути тестами.",
					"Выкати код с выключенным флагом обычным релизом — на пользователей это не влияет.",
					"Включай флаг ступенями: тестовый стенд → 1% → 10% → 100%, между шагами смотри графики ошибок.",
					"Через две недели после 100% удали флаг и мёртвую ветку кода отдельным MR.",
				}),
			doc("pb-code-review", "Как проходить код-ревью",
				"Ревью — способ поделиться контекстом, а не экзамен. Маленькие MR проходят быстрее.",
				[]string{"code_review"},
				[]string{"ревью", "review", "merge request", "pull request", "мердж", "мерж", "аппрув", "mr$"},
				[]string{
					"Держи MR небольшим: до ~400 строк, одна логическая задача, в описании — ссылка на задачу и как проверить.",
					"Перед отправкой прогони линтер и тесты локально; CI должен быть зелёным.",
					"Назначь двух ревьюеров: наставника и дежурного ревьюера команды. Нужен минимум один аппрув, на первой неделе — аппрув наставника.",
					"На замечания отвечай в течение рабочего дня; спорные вопросы выноси в созвон на 10 минут, а не в длинный тред.",
					"После аппрува мержишь сам; флаг включаешь только после выкладки.",
				}),
			doc("pb-absences", "Отсутствия: отгул, отпуск, больничный",
				"Отгул оформляется заявкой в сервисе «Отсутствия»; отдельно писать руководителю не нужно — сервис уведомит его сам.",
				[]string{"absences"},
				[]string{"отгул", "отпуск", "больничн", "отсутств", "отпрос", "заболел", "зачет", "экзамен"},
				[]string{
					"Оформи заявку в сервисе «Отсутствия» минимум за один рабочий день: дата, время, причина.",
					"Сервис автоматически уведомит наставника и руководителя — отдельное письмо руководителю не требуется.",
					"Если из-за отгула пропускаешь встречу или дедлайн — коротко предупреди наставника в командном чате.",
					"Учебные причины (зачёт, экзамен) согласуются автоматически, отрабатывать часы не нужно.",
					"Больничный и отпуск оформляются в том же сервисе, но по своим формам.",
				}),
			doc("pb-access", "Доступы: репозитории и тестовые стенды",
				"Доступ к репозиториям своей команды и тестовому стенду стажёр получает автоматически через IAM.",
				[]string{"access"},
				[]string{"доступ", "репозитор", "стенд", "gitlab", "песочниц", "синтетическ", "vpn"},
				[]string{
					"Доступ к репозиториям команды запрашивается через IAM — Гоффи сделает это за тебя.",
					"Стажёру выдаётся уровень developer: ветки и MR, без прав на защищённые ветки.",
					"Для отладки используй тестовый стенд: на нём только синтетические данные.",
					"Доступ к репозиториям других команд запрашивает руководитель, а не стажёр.",
				}),
			doc("pb-first-task", "Первая задача: с чего начать",
				"Первая задача — про знакомство с кодовой базой и процессом, а не про скорость.",
				[]string{"first_task"},
				[]string{"задач", "таск", "спринт", "застрял", "декомпоз", "начать"},
				[]string{
					"Прочитай описание задачи и найди в коде ручку или модуль, которые она затрагивает.",
					"Разбей задачу на шаги и запиши план в комментарии к задаче — наставник увидит ход мысли.",
					"Сделай первый маленький коммит в первый же день: даже тест или заготовка помогают получить обратную связь.",
					"Если застрял дольше чем на час — сформулируй вопрос и принеси наставнику; Гоффи поможет собрать контекст.",
				}),
			doc("pb-one-on-one", "Встречи 1:1 с наставником",
				"1:1 — время для вопросов, где нужен опыт сеньора. Рутинные вопросы закрывает Гоффи.",
				[]string{"one_on_one"},
				[]string{"встреч", "one-on-one", "обратн", "фидбек"},
				[]string{
					"1:1 проходит раз в неделю, 30 минут. Повестку готовит стажёр.",
					"Перед встречей Гоффи отправляет наставнику бриф: что уже изучено и какой вопрос требует сеньора.",
					"Приноси архитектурные и карьерные вопросы — на них 1:1 даёт максимум пользы.",
				}),
			doc("pb-data-safety", "Работа с данными: прод и ПДн",
				"Прод-данные с персональной информацией стажёрам недоступны. Для отладки есть синтетические данные.",
				[]string{"security"},
				[]string{"данны", "пдн", "персональн", "прод$", "production", "безопасност", "дамп"},
				[]string{
					"Выгрузки production-баз с персональными данными запрещены для всех ролей Гоффи.",
					"Для отладки и тестов используй тестовый стенд с синтетическими данными.",
					"Если задаче нужны реалистичные данные — попроси наставника помочь с генератором синтетики.",
				}),
			doc("pb-mentor-guide", "Гайд наставника: ревью компетенций стажёра",
				"Внутренний материал для наставников и руководителей.",
				[]string{"mentoring"},
				[]string{"компетенц", "оценк", "экватор", "грейд"},
				[]string{
					"Ревью компетенций проводится на экваторе и перед выпуском.",
					"Оценку обсуждают наставник и руководитель; стажёр получает итог на встрече.",
				}, domain.RoleMentor, domain.RoleTeamLead),
		},
	}
}

// MeetBase — адрес имитации сервиса видеовстреч (несуществующий домен .example).
const MeetBase = "https://meet.avito.example/"

// Planner возвращает календарь первых недель относительно момента загрузки демо:
// созвон 1:1 через три часа, синк и ревью на ближайшие дни, дедлайн первого PR в пятницу.
func Planner(now time.Time) []domain.PlannerItem {
	day := func(offset, hour, minute int) time.Time {
		d := now.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, now.Location())
	}
	oneOnOne := now.Add(3 * time.Hour).Truncate(time.Minute)
	friday := (int(time.Friday) - int(now.Weekday()) + 7) % 7
	if friday == 0 {
		friday = 7
	}
	item := func(id, kind, title, desc string, at time.Time, duration int, link string, mandatory bool, by, byName string) domain.PlannerItem {
		if link != "" {
			link = MeetBase + link
		}
		return domain.PlannerItem{
			ID: id, InternID: InternID, Kind: kind, Title: title, Description: desc, StartsAt: at, DurationMin: duration,
			AllDay: duration == 0 && kind != domain.PlannerDeadline, Link: link, Mandatory: mandatory,
			CreatedBy: by, CreatedByName: byName, Status: domain.PlannerPlanned, IsSynthetic: true,
		}
	}
	const mentor, lead = "Мария Кузнецова", "Дмитрий Орлов"
	return []domain.PlannerItem{
		item("plan-1on1", domain.PlannerMeeting, "Созвон 1:1 с наставником",
			"Первая встреча 1:1: прогресс по чек-листу и вопросы, где нужен опыт сеньора. Гоффи заранее отправит наставнику бриф.",
			oneOnOne, 30, "autoteka-1on1", true, MentorID, mentor),
		item("plan-sync", domain.PlannerSync, "Командный синк Autoteka",
			"Ежедневный синк команды: что сделано, что в работе, что мешает.",
			day(1, 11, 0), 15, "autoteka-daily", true, TeamLeadID, lead),
		item("plan-review", domain.PlannerReview, "Код-ревью задачи AUT-1423",
			"Разбор первого MR с наставником: фича-флаг, тесты, контракт ручки.",
			day(2, 15, 0), 45, "autoteka-review", true, MentorID, mentor),
		item("plan-first-pr", domain.PlannerDeadline, "Срок сдачи первого PR",
			"Первый PR по задаче AUT-1423 должен быть отправлен на ревью.",
			day(friday, 18, 0), 0, "", true, MentorID, mentor),
		item("plan-security-course", domain.PlannerTask, "Пройти курс по информационной безопасности",
			"Обязательный вводный курс: работа с данными, ПДн, доступы.",
			day(5, 0, 0), 0, "", true, TeamLeadID, lead),
		item("plan-feedback", domain.PlannerMeeting, "Созвон по обратной связи за первую неделю",
			"Руководитель и наставник: итоги недели, ожидания на следующую.",
			day(7, 16, 0), 30, "autoteka-feedback", true, TeamLeadID, lead),
	}
}
