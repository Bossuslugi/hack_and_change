package seed

import (
	"testing"

	"aibuddy/backend/internal/domain"
)

func TestSeedIsSyntheticAndConsistent(t *testing.T) {
	d := Data()

	if !d.Intern.IsSynthetic || d.Intern.ID != InternID || d.Intern.Role != domain.RoleInternBackend {
		t.Fatalf("профиль стажёра: %+v", d.Intern)
	}
	if d.Intern.Team != "Autoteka" || d.Intern.Stack != "Go" || d.Intern.Stage != "Week 1" {
		t.Fatalf("контекст стажёра должен соответствовать сценарию 2: %+v", d.Intern)
	}

	tokens, ids := map[string]bool{}, map[string]domain.Employee{}
	for _, e := range d.Employees {
		if !e.IsSynthetic {
			t.Errorf("сотрудник %s должен быть синтетическим", e.ID)
		}
		if e.Token != "" && tokens[e.Token] {
			t.Errorf("повтор токена %s", e.Token)
		}
		tokens[e.Token] = true
		ids[e.ID] = e
	}
	if ids[d.Intern.MentorID].Role != domain.RoleMentor || ids[d.Intern.TeamLeadID].Role != domain.RoleTeamLead {
		t.Fatal("наставник и руководитель стажёра должны быть в справочнике с верными ролями")
	}
	if cron := ids[CronID]; cron.Role != domain.RoleSystemCron || cron.Token != "" {
		t.Fatalf("сервисный аккаунт крона не должен иметь токена: %+v", cron)
	}
}

func TestSeedDocsCarryDisclaimer(t *testing.T) {
	d := Data()
	seen := map[string]bool{}
	restricted := 0
	for _, doc := range d.Docs {
		if seen[doc.ID] {
			t.Errorf("повтор документа %s", doc.ID)
		}
		seen[doc.ID] = true
		// Жёсткое требование кейса: явный флаг и дисклеймер у каждой инструкции.
		if !doc.IsSynthetic || doc.Disclaimer != "Тестовая инструкция (Синтетические данные)" {
			t.Errorf("документ %s: нет флага is_synthetic или дисклеймера", doc.ID)
		}
		if doc.Title == "" || doc.Summary == "" || len(doc.Steps) == 0 || len(doc.Topics) == 0 || len(doc.Keywords) == 0 || doc.Source == "" {
			t.Errorf("документ %s заполнен не полностью", doc.ID)
		}
		if len(doc.Audience) > 0 {
			restricted++
		}
	}
	for _, id := range []string{"pb-feature-flags", "pb-code-review", "pb-absences"} {
		if !seen[id] {
			t.Errorf("нет документа %s, нужного сценариям", id)
		}
	}
	if restricted == 0 {
		t.Error("нужен хотя бы один документ с ограниченной аудиторией (проверка прав на знания)")
	}
}

func TestSeedChecklistAndTask(t *testing.T) {
	d := Data()
	positions, byID := map[int]bool{}, map[string]domain.ChecklistItem{}
	for _, it := range d.Checklist {
		if positions[it.Position] || !it.Status.Valid() || it.Title == "" {
			t.Errorf("шаг %s: позиция/статус/название некорректны", it.ID)
		}
		positions[it.Position] = true
		byID[it.ID] = it
	}
	if it := byID["access_repo"]; it.Position != 1 || it.Status != domain.StatusPending {
		t.Errorf("шаг №1 чек-листа — доступы в статусе pending: %+v", it)
	}
	if it := byID["first_task"]; it.Title != "Первая задача в спринте" || it.Status != domain.StatusInProgress {
		t.Errorf("первая задача: %+v", it)
	}
	for _, id := range []string{"team_processes", "absence_rules", "first_one_on_one"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("нет шага %s, на который ссылается спецификация", id)
		}
	}
	if !d.Task.Stalled() || d.Task.ArchitectureQuestion == "" || d.Task.InternID != InternID {
		t.Errorf("задача должна «висеть» без коммитов и иметь архитектурный вопрос: %+v", d.Task)
	}
	if d.Repositories[TeamRepo] != Team || d.Repositories["avito-payments-core"] == Team {
		t.Errorf("каталог репозиториев: %v", d.Repositories)
	}
	kinds := map[string]int{}
	for _, f := range d.Flags {
		kinds[f.Kind]++
	}
	if kinds[domain.FlagRuntime] == 0 || kinds[domain.FlagCompileTime] == 0 {
		t.Errorf("нужны и runtime, и compile-time флаги: %v", kinds)
	}
}
