package spec

import (
	"strings"
	"testing"
	"testing/fstest"

	"aibuddy/specs"
)

func TestLoadEmbeddedSpecs(t *testing.T) {
	b, err := Load()
	if err != nil {
		t.Fatalf("встроенные спецификации невалидны: %v", err)
	}
	if b.Agent.Kind != "AgentBehavior" || b.Tools.Kind != "McpTools" || b.RBAC.Kind != "RbacMatrix" {
		t.Fatalf("kinds: %s %s %s", b.Agent.Kind, b.Tools.Kind, b.RBAC.Kind)
	}
	if !b.Agent.Metadata.IsSynthetic || !b.Tools.Metadata.IsSynthetic || !b.RBAC.Metadata.IsSynthetic {
		t.Error("все спецификации должны быть помечены is_synthetic")
	}
	if got := b.Agent.Agent.SyntheticData.Disclaimer; got != "Тестовая инструкция (Синтетические данные)" {
		t.Errorf("дисклеймер = %q", got)
	}
	if b.Agent.Agent.Engine.SmartDelayMS != 300 {
		t.Errorf("smart_delay_ms = %d, want 300", b.Agent.Agent.Engine.SmartDelayMS)
	}
	for i := 1; i < len(b.Agent.Intents); i++ {
		if b.Agent.Intents[i-1].Priority < b.Agent.Intents[i].Priority {
			t.Fatalf("интенты должны быть отсортированы по убыванию приоритета: %s перед %s",
				b.Agent.Intents[i-1].ID, b.Agent.Intents[i].ID)
		}
	}
	if b.Agent.Intents[0].ID != "restricted_data_export" {
		t.Errorf("первым должен проверяться интент безопасности, а не %s", b.Agent.Intents[0].ID)
	}
	// Пять обязательных инструментов кейса.
	for _, name := range []string{
		"iam_access.request_repo_access", "db_admin.export_prod_pii", "playbook.search_docs",
		"absences.create_day_off", "mentor.send_1on1_brief",
	} {
		tool, ok := b.Tools.ToolByName(name)
		if !ok {
			t.Errorf("в спецификации нет инструмента %s", name)
			continue
		}
		if tool.InputSchema["type"] != "object" || tool.Description == "" {
			t.Errorf("инструмент %s: неполный контракт", name)
		}
	}
	if _, ok := b.Tools.ToolByName("нет.такого"); ok {
		t.Error("ToolByName нашёл несуществующий инструмент")
	}
	if tool, _ := b.Tools.ToolByName("absences.create_day_off"); !tool.RequiresConfirmation {
		t.Error("создание отгула должно требовать подтверждения")
	}
	if len(b.Agent.Scenarios) < 4 || len(b.Agent.ChecklistEffects) == 0 || len(b.Agent.Agent.ContextVariables) == 0 {
		t.Error("в спецификации агента должны быть сценарии, эффекты и контекстные переменные")
	}
	if b.Agent.Metrics.MinutesSaved["question_answered"] == 0 || b.Agent.Metrics.MentorHoursBaselineMax != 48 {
		t.Errorf("метрики: %+v", b.Agent.Metrics)
	}
}

func TestMustLoad(t *testing.T) {
	if MustLoad() == nil {
		t.Fatal("MustLoad вернул nil")
	}
}

func TestResponseAndIntentLookup(t *testing.T) {
	b := MustLoad()
	got := b.Agent.Response("not_found", map[string]string{"mentor": "Мария"})
	if !strings.Contains(got, "(Мария)") || strings.Contains(got, "{mentor}") || strings.Contains(got, "\n") {
		t.Errorf("Response = %q", got)
	}
	if b.Agent.Response("нет_шаблона", nil) != "" {
		t.Error("неизвестный шаблон должен давать пустую строку")
	}
	if in, ok := b.Agent.IntentByID("absence_day_off"); !ok || in.ConfirmTool != "absences.create_day_off" || in.ActionCard != "absence_form" {
		t.Errorf("absence_day_off = %+v", in)
	}
	if in, ok := b.Agent.IntentByID("knowledge_search"); !ok || in.Tool != "playbook.search_docs" {
		t.Errorf("fallback intent = %+v", in)
	}
	if _, ok := b.Agent.IntentByID("нет"); ok {
		t.Error("IntentByID нашёл несуществующий интент")
	}
}

func readSpec(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := specs.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadFSErrors(t *testing.T) {
	agent, tools, rbac := readSpec(t, specs.AgentBehaviorFile), readSpec(t, specs.MCPToolsFile), readSpec(t, specs.RBACMatrixFile)
	cases := []struct {
		name string
		fs   fstest.MapFS
		want string
	}{
		{"нет файла", fstest.MapFS{specs.AgentBehaviorFile: {Data: agent}}, "чтение"},
		{"битый YAML", fstest.MapFS{
			specs.AgentBehaviorFile: {Data: []byte("kind: [unclosed")},
			specs.MCPToolsFile:      {Data: tools}, specs.RBACMatrixFile: {Data: rbac},
		}, "разбор"},
		{"неверный kind", fstest.MapFS{
			specs.AgentBehaviorFile: {Data: tools},
			specs.MCPToolsFile:      {Data: tools}, specs.RBACMatrixFile: {Data: rbac},
		}, "kind"},
		{"невалидное содержимое", fstest.MapFS{
			specs.AgentBehaviorFile: {Data: []byte("kind: AgentBehavior\n")},
			specs.MCPToolsFile:      {Data: tools}, specs.RBACMatrixFile: {Data: rbac},
		}, "невалидны"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFS(tc.fs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, ожидалось упоминание %q", err, tc.want)
			}
		})
	}
	if _, err := LoadFS(fstest.MapFS{
		specs.AgentBehaviorFile: {Data: agent}, specs.MCPToolsFile: {Data: tools}, specs.RBACMatrixFile: {Data: rbac},
	}); err != nil {
		t.Fatalf("корректные спецификации из MapFS: %v", err)
	}
}

func TestValidateCrossReferences(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Bundle)
		want   string
	}{
		{"пустой промпт", func(b *Bundle) { b.Agent.Agent.SystemPrompt = " " }, "system_prompt"},
		{"пустой дисклеймер", func(b *Bundle) { b.Agent.Agent.SyntheticData.Disclaimer = "" }, "disclaimer"},
		{"дубль инструмента", func(b *Bundle) { b.Tools.Tools = append(b.Tools.Tools, b.Tools.Tools[0]) }, "объявлен дважды"},
		{"неизвестный сервер", func(b *Bundle) { b.Tools.Tools[0].Server = "ghost" }, "неизвестный сервер"},
		{"имя без префикса сервера", func(b *Bundle) { b.Tools.Tools[0].Name = "request_repo_access" }, "должно начинаться"},
		{"схема не object", func(b *Bundle) { b.Tools.Tools[0].InputSchema = map[string]any{"type": "string"} }, "input_schema"},
		{"нет инструментов", func(b *Bundle) { b.Tools.Tools = nil }, "ни одного MCP-инструмента"},
		{"интент без id", func(b *Bundle) { b.Agent.Intents[0].ID = "" }, "интент без id"},
		{"дубль интента", func(b *Bundle) { b.Agent.Intents = append(b.Agent.Intents, b.Agent.Intents[0]) }, "интент restricted_data_export объявлен дважды"},
		{"интент с неизвестным инструментом", func(b *Bundle) { b.Agent.Intents[0].Tool = "ghost.tool" }, "неизвестный инструмент"},
		{"эффект с неизвестным инструментом", func(b *Bundle) { b.Agent.ChecklistEffects[0].Tool = "ghost.tool" }, "checklist_effects"},
		{"эффект без шага", func(b *Bundle) { b.Agent.ChecklistEffects[0].Item = "" }, "пустые item/status"},
		{"default allow запрещён", func(b *Bundle) { b.RBAC.DefaultEffect = "allow" }, "default_effect"},
		{"политика без id", func(b *Bundle) { b.RBAC.Policies[0].ID = "" }, "пустым или повторным id"},
		{"неверный effect", func(b *Bundle) { b.RBAC.Policies[0].Effect = "maybe" }, "effect"},
		{"неизвестная роль", func(b *Bundle) { b.RBAC.Policies[0].Roles = []string{"root"} }, "неизвестная роль"},
		{"неизвестное ограничение", func(b *Bundle) { b.RBAC.Policies[0].Constraints = []string{"magic"} }, "неизвестное ограничение"},
		{"неизвестная операция", func(b *Bundle) { b.RBAC.Policies[0].Operation = "ghost.tool" }, "неизвестная операция"},
		{"инструмент без политики", func(b *Bundle) {
			var kept []Policy
			for _, p := range b.RBAC.Policies {
				if p.Operation != "feature_flags.list" {
					kept = append(kept, p)
				}
			}
			b.RBAC.Policies = kept
		}, "нет ни одной политики"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := MustLoad() // свежая копия: мутации не влияют на другие тесты
			tc.mutate(b)
			err := b.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, ожидалось упоминание %q", err, tc.want)
			}
		})
	}
}
