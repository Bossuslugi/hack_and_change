// Package spec загружает и валидирует SDD-спецификации (OpenSpec) из директории /specs.
// Остальные пакеты получают поведение агента, контракты MCP и матрицу RBAC только отсюда.
package spec

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"aibuddy/specs"
)

// Metadata — общий заголовок OpenSpec-файла.
type Metadata struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title" json:"title,omitempty"`
	Version     string `yaml:"version" json:"version"`
	IsSynthetic bool   `yaml:"is_synthetic" json:"is_synthetic"`
}

// ContextVariable — контекстная переменная стажёра, доступная агенту.
type ContextVariable struct {
	Name    string `yaml:"name" json:"name"`
	Source  string `yaml:"source" json:"source"`
	Example string `yaml:"example" json:"example"`
}

// Intent — интент Smart Engine с триггерами и планом вызова инструмента.
type Intent struct {
	ID          string     `yaml:"id"`
	Scenario    string     `yaml:"scenario"`
	Priority    int        `yaml:"priority"`
	Description string     `yaml:"description"`
	Tool        string     `yaml:"tool"`
	ActionCard  string     `yaml:"action_card"`
	ConfirmTool string     `yaml:"confirm_tool"`
	Topics      []string   `yaml:"topics"`
	Triggers    [][]string `yaml:"triggers"`
	Suggestions []string   `yaml:"suggestions"`
}

// ChecklistEffect — правило автоматического обновления чек-листа.
type ChecklistEffect struct {
	Tool     string   `yaml:"tool"`
	DocsRead []string `yaml:"docs_read"`
	Item     string   `yaml:"item"`
	Status   string   `yaml:"status"`
}

// MetricsSpec — параметры оценки экономии времени наставника.
type MetricsSpec struct {
	MentorHoursBaselineMin int            `yaml:"mentor_hours_baseline_min"`
	MentorHoursBaselineMax int            `yaml:"mentor_hours_baseline_max"`
	MinutesSaved           map[string]int `yaml:"minutes_saved"`
}

// Scenario — приёмочный сценарий Given/When/Then.
type Scenario struct {
	ID    string   `yaml:"id"`
	Title string   `yaml:"title"`
	Given string   `yaml:"given"`
	When  string   `yaml:"when"`
	Then  []string `yaml:"then"`
}

// AgentSpec — спецификация поведения агента (01_agent_behavior).
type AgentSpec struct {
	OpenSpec string   `yaml:"openspec"`
	Kind     string   `yaml:"kind"`
	Metadata Metadata `yaml:"metadata"`
	Agent    struct {
		Name             string            `yaml:"name"`
		Language         string            `yaml:"language"`
		Tone             string            `yaml:"tone"`
		SystemPrompt     string            `yaml:"system_prompt"`
		ContextVariables []ContextVariable `yaml:"context_variables"`
		SyntheticData    struct {
			Disclaimer string   `yaml:"disclaimer"`
			Rules      []string `yaml:"rules"`
		} `yaml:"synthetic_data"`
		Engine struct {
			SmartDelayMS int `yaml:"smart_delay_ms"`
			LLM          struct {
				Protocol  string   `yaml:"protocol"`
				Env       []string `yaml:"env"`
				TimeoutMS int      `yaml:"timeout_ms"`
				Fallback  string   `yaml:"fallback"`
			} `yaml:"llm"`
		} `yaml:"engine"`
	} `yaml:"agent"`
	Intents          []Intent          `yaml:"intents"`
	FallbackIntent   Intent            `yaml:"fallback_intent"`
	ChecklistEffects []ChecklistEffect `yaml:"checklist_effects"`
	Responses        map[string]string `yaml:"responses"`
	Metrics          MetricsSpec       `yaml:"metrics"`
	Scenarios        []Scenario        `yaml:"scenarios"`
}

// Response возвращает шаблон ответа с подстановкой {ключ} → значение.
func (a *AgentSpec) Response(key string, vars map[string]string) string {
	text := strings.TrimSpace(a.Responses[key])
	for k, v := range vars {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}

// IntentByID ищет интент по идентификатору (включая fallback).
func (a *AgentSpec) IntentByID(id string) (Intent, bool) {
	for _, in := range a.Intents {
		if in.ID == id {
			return in, true
		}
	}
	if a.FallbackIntent.ID == id {
		return a.FallbackIntent, true
	}
	return Intent{}, false
}

// Server — описание MCP-сервера (имитации корпоративного сервиса).
type Server struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
}

// ToolError — документированная ошибка инструмента.
type ToolError struct {
	Code        string `yaml:"code" json:"code"`
	Description string `yaml:"description" json:"description"`
}

// Tool — контракт MCP-инструмента.
type Tool struct {
	Name                 string         `yaml:"name" json:"name"`
	Server               string         `yaml:"server" json:"server"`
	Title                string         `yaml:"title" json:"title"`
	Description          string         `yaml:"description" json:"description"`
	SideEffects          string         `yaml:"side_effects" json:"side_effects"`
	RequiresConfirmation bool           `yaml:"requires_confirmation" json:"requires_confirmation"`
	InputSchema          map[string]any `yaml:"input_schema" json:"input_schema"`
	OutputSchema         map[string]any `yaml:"output_schema" json:"output_schema,omitempty"`
	Errors               []ToolError    `yaml:"errors" json:"errors,omitempty"`
}

// ToolsSpec — спецификация MCP-инструментов (02_mcp_tools).
type ToolsSpec struct {
	OpenSpec string   `yaml:"openspec"`
	Kind     string   `yaml:"kind"`
	Metadata Metadata `yaml:"metadata"`
	Servers  []Server `yaml:"servers"`
	Tools    []Tool   `yaml:"tools"`
}

// ToolByName ищет инструмент по имени.
func (t *ToolsSpec) ToolByName(name string) (Tool, bool) {
	for _, tool := range t.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Role — роль RBAC.
type Role struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	Kind  string `yaml:"kind" json:"kind"`
}

// Constraint — ABAC-ограничение поверх роли.
type Constraint struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
}

// Policy — правило матрицы RBAC.
type Policy struct {
	ID              string   `yaml:"id" json:"id"`
	Operation       string   `yaml:"operation" json:"operation"`
	Effect          string   `yaml:"effect" json:"effect"`
	Roles           []string `yaml:"roles" json:"roles"`
	Constraints     []string `yaml:"constraints" json:"constraints,omitempty"`
	Incident        bool     `yaml:"incident" json:"incident,omitempty"`
	Reason          string   `yaml:"reason" json:"reason,omitempty"`
	SafeAlternative string   `yaml:"safe_alternative" json:"safe_alternative,omitempty"`
	Description     string   `yaml:"description" json:"description,omitempty"`
}

// RBACSpec — матрица прав (03_rbac_matrix).
type RBACSpec struct {
	OpenSpec      string       `yaml:"openspec"`
	Kind          string       `yaml:"kind"`
	Metadata      Metadata     `yaml:"metadata"`
	DefaultEffect string       `yaml:"default_effect"`
	Roles         []Role       `yaml:"roles"`
	Constraints   []Constraint `yaml:"constraints"`
	Policies      []Policy     `yaml:"policies"`
}

// Bundle — все три спецификации.
type Bundle struct {
	Agent AgentSpec
	Tools ToolsSpec
	RBAC  RBACSpec
}

// Load читает спецификации, встроенные в бинарник.
func Load() (*Bundle, error) {
	return LoadFS(specs.FS)
}

// MustLoad — Load с паникой при ошибке (спеки вкомпилированы, ошибка = баг сборки).
func MustLoad() *Bundle {
	b, err := Load()
	if err != nil {
		panic(err)
	}
	return b
}

// LoadFS читает и валидирует спецификации из произвольной файловой системы.
func LoadFS(fsys fs.FS) (*Bundle, error) {
	var b Bundle
	files := []struct {
		name string
		kind string
		dst  any
	}{
		{specs.AgentBehaviorFile, "AgentBehavior", &b.Agent},
		{specs.MCPToolsFile, "McpTools", &b.Tools},
		{specs.RBACMatrixFile, "RbacMatrix", &b.RBAC},
	}
	for _, f := range files {
		raw, err := fs.ReadFile(fsys, f.name)
		if err != nil {
			return nil, fmt.Errorf("spec: чтение %s: %w", f.name, err)
		}
		if err := yaml.Unmarshal(raw, f.dst); err != nil {
			return nil, fmt.Errorf("spec: разбор %s: %w", f.name, err)
		}
	}
	if b.Agent.Kind != "AgentBehavior" || b.Tools.Kind != "McpTools" || b.RBAC.Kind != "RbacMatrix" {
		return nil, fmt.Errorf("spec: неверный kind (%q, %q, %q)", b.Agent.Kind, b.Tools.Kind, b.RBAC.Kind)
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	sort.SliceStable(b.Agent.Intents, func(i, j int) bool {
		return b.Agent.Intents[i].Priority > b.Agent.Intents[j].Priority
	})
	return &b, nil
}

// Validate проверяет целостность и перекрёстные ссылки между спецификациями.
func (b *Bundle) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if strings.TrimSpace(b.Agent.Agent.SystemPrompt) == "" {
		add("agent.system_prompt пуст")
	}
	if strings.TrimSpace(b.Agent.Agent.SyntheticData.Disclaimer) == "" {
		add("agent.synthetic_data.disclaimer пуст")
	}

	servers := map[string]bool{}
	for _, s := range b.Tools.Servers {
		servers[s.ID] = true
	}
	tools := map[string]bool{}
	for _, t := range b.Tools.Tools {
		if tools[t.Name] {
			add("инструмент %s объявлен дважды", t.Name)
		}
		tools[t.Name] = true
		if !servers[t.Server] {
			add("инструмент %s ссылается на неизвестный сервер %q", t.Name, t.Server)
		}
		if !strings.HasPrefix(t.Name, t.Server+".") {
			add("имя инструмента %s должно начинаться с %s.", t.Name, t.Server)
		}
		if t.InputSchema == nil || t.InputSchema["type"] != "object" {
			add("инструмент %s: input_schema.type должен быть object", t.Name)
		}
	}
	if len(tools) == 0 {
		add("не объявлено ни одного MCP-инструмента")
	}

	intents := map[string]bool{}
	for _, in := range append(append([]Intent{}, b.Agent.Intents...), b.Agent.FallbackIntent) {
		if in.ID == "" {
			add("интент без id")
			continue
		}
		if intents[in.ID] {
			add("интент %s объявлен дважды", in.ID)
		}
		intents[in.ID] = true
		for _, ref := range []string{in.Tool, in.ConfirmTool} {
			if ref != "" && !tools[ref] {
				add("интент %s ссылается на неизвестный инструмент %q", in.ID, ref)
			}
		}
	}
	for _, e := range b.Agent.ChecklistEffects {
		if e.Tool != "" && !tools[e.Tool] {
			add("checklist_effects: неизвестный инструмент %q", e.Tool)
		}
		if e.Item == "" || e.Status == "" {
			add("checklist_effects: пустые item/status")
		}
	}

	roles := map[string]bool{}
	for _, r := range b.RBAC.Roles {
		roles[r.ID] = true
	}
	constraints := map[string]bool{}
	for _, c := range b.RBAC.Constraints {
		constraints[c.ID] = true
	}
	if b.RBAC.DefaultEffect != "deny" {
		add("rbac.default_effect должен быть deny")
	}
	covered := map[string]bool{}
	ids := map[string]bool{}
	for _, p := range b.RBAC.Policies {
		if p.ID == "" || ids[p.ID] {
			add("rbac: политика с пустым или повторным id %q", p.ID)
		}
		ids[p.ID] = true
		if p.Effect != "allow" && p.Effect != "deny" {
			add("rbac: политика %s: effect %q", p.ID, p.Effect)
		}
		for _, r := range p.Roles {
			if r != "*" && !roles[r] {
				add("rbac: политика %s: неизвестная роль %q", p.ID, r)
			}
		}
		for _, c := range p.Constraints {
			if !constraints[c] {
				add("rbac: политика %s: неизвестное ограничение %q", p.ID, c)
			}
		}
		if !strings.HasPrefix(p.Operation, "api.") && !tools[p.Operation] {
			add("rbac: политика %s: неизвестная операция %q", p.ID, p.Operation)
		}
		covered[p.Operation] = true
	}
	for name := range tools {
		if !covered[name] {
			add("rbac: для инструмента %s нет ни одной политики", name)
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("spec: спецификации невалидны:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}
