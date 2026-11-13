package rbac

import (
	"net/http"
	"testing"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/spec"
)

var (
	intern = domain.Principal{ID: "u-intern-01", Name: "Стажёр", Role: domain.RoleInternBackend, Team: "Autoteka"}
	mentor = domain.Principal{ID: "u-mentor-01", Name: "Наставник", Role: domain.RoleMentor, Team: "Autoteka"}
	lead   = domain.Principal{ID: "u-lead-01", Name: "Руководитель", Role: domain.RoleTeamLead, Team: "Autoteka"}
	cron   = domain.Principal{ID: "svc-ai-cron", Name: "Крон", Role: domain.RoleSystemCron, Team: "Autoteka"}
)

func engine(t *testing.T) *Engine {
	t.Helper()
	return New(spec.MustLoad().RBAC)
}

// TestMatrixFromSpec проверяет матрицу прав из specs/03_rbac_matrix.openspec.yaml.
func TestMatrixFromSpec(t *testing.T) {
	e := engine(t)
	self := Resource{InternID: intern.ID, Team: "Autoteka"}
	cases := []struct {
		name       string
		p          domain.Principal
		op         string
		res        Resource
		allowed    bool
		policy     string
		constraint string
	}{
		{"стажёр: доступ к репо своей команды", intern, "iam_access.request_repo_access", self, true, "POL-IAM-001", ""},
		{"стажёр: репо чужой команды", intern, "iam_access.request_repo_access", Resource{InternID: intern.ID, Team: "Payments"}, false, "POL-IAM-001", ConstraintTeamScope},
		{"стажёр: неизвестный репозиторий (нет команды)", intern, "iam_access.request_repo_access", Resource{InternID: intern.ID}, false, "POL-IAM-001", ConstraintTeamScope},
		{"стажёр: доступ за другого сотрудника", intern, "iam_access.request_repo_access", Resource{InternID: "u-other", Team: "Autoteka"}, false, "POL-IAM-001", ConstraintSelfOnly},
		{"наставник: доступ стажёру своей команды", mentor, "iam_access.request_repo_access", self, true, "POL-IAM-002", ""},
		{"наставник: чужая команда", mentor, "iam_access.request_repo_access", Resource{InternID: intern.ID, Team: "Payments"}, false, "POL-IAM-002", ConstraintTeamScope},

		{"стажёр: выгрузка ПДн", intern, "db_admin.export_prod_pii", self, false, "POL-SEC-001", ""},
		{"наставник: выгрузка ПДн", mentor, "db_admin.export_prod_pii", Resource{}, false, "POL-SEC-001", ""},
		{"руководитель: выгрузка ПДн", lead, "db_admin.export_prod_pii", Resource{}, false, "POL-SEC-001", ""},
		{"крон: выгрузка ПДн", cron, "db_admin.export_prod_pii", Resource{}, false, "POL-SEC-001", ""},

		{"стажёр: поиск по базе знаний", intern, "playbook.search_docs", Resource{}, true, "POL-KB-001", ""},
		{"стажёр: отгул на себя", intern, "absences.create_day_off", self, true, "POL-ABS-001", ""},
		{"стажёр: отгул за другого", intern, "absences.create_day_off", Resource{InternID: "u-other"}, false, "POL-ABS-001", ConstraintSelfOnly},
		{"наставник: отгул за стажёра", mentor, "absences.create_day_off", self, false, PolicyDefaultDeny, ""},
		{"руководитель: отгул за стажёра", lead, "absences.create_day_off", self, false, PolicyDefaultDeny, ""},

		{"стажёр: отправка брифа", intern, "mentor.send_1on1_brief", self, false, PolicyDefaultDeny, ""},
		{"крон: отправка брифа", cron, "mentor.send_1on1_brief", self, true, "POL-MNT-001", ""},
		{"наставник: отправка брифа", mentor, "mentor.send_1on1_brief", self, true, "POL-MNT-001", ""},

		{"стажёр: уведомления", intern, "notifications.send", Resource{}, false, PolicyDefaultDeny, ""},
		{"крон: уведомления", cron, "notifications.send", Resource{}, true, "POL-NTF-001", ""},
		{"стажёр: своя задача", intern, "tracker.get_task_activity", self, true, "POL-TRK-001", ""},
		{"стажёр: чужая задача", intern, "tracker.get_task_activity", Resource{InternID: "u-other"}, false, "POL-TRK-001", ConstraintSelfOnly},
		{"крон: задача стажёра", cron, "tracker.get_task_activity", self, true, "POL-TRK-002", ""},

		{"стажёр: чат", intern, "api.chat.send", Resource{}, true, "POL-API-002", ""},
		{"наставник: чат", mentor, "api.chat.send", Resource{}, false, PolicyDefaultDeny, ""},
		{"наставник: история чата", mentor, "api.chat_history.read", Resource{}, false, PolicyDefaultDeny, ""},
		{"наставник: брифы", mentor, "api.briefs.read", Resource{}, true, "POL-API-004", ""},
		{"стажёр: брифы", intern, "api.briefs.read", Resource{}, false, PolicyDefaultDeny, ""},
		{"стажёр: флаги", intern, "api.feature_flags.write", Resource{}, false, PolicyDefaultDeny, ""},
		{"руководитель: флаги", lead, "api.feature_flags.write", Resource{}, true, "POL-API-008", ""},
		{"руководитель: инциденты", lead, "api.incidents.read", Resource{}, true, "POL-API-010", ""},
		{"наставник: инциденты", mentor, "api.incidents.read", Resource{}, false, PolicyDefaultDeny, ""},
		{"крон: workspace недоступен сервисному аккаунту", cron, "api.workspace.read", Resource{}, false, PolicyDefaultDeny, ""},

		{"неизвестная операция", intern, "ghost.operation", Resource{}, false, PolicyDefaultDeny, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := e.Check(tc.p, tc.op, tc.res)
			if v.Allowed != tc.allowed || v.PolicyID != tc.policy || v.Constraint != tc.constraint {
				t.Fatalf("verdict = %+v, want allowed=%v policy=%s constraint=%q", v, tc.allowed, tc.policy, tc.constraint)
			}
			if v.Role != tc.p.Role || v.Operation != tc.op || v.Reason == "" {
				t.Fatalf("вердикт должен содержать роль, операцию и причину: %+v", v)
			}
			wantCode, wantStatus := domain.VerdictForbidden, http.StatusForbidden
			if tc.allowed {
				wantCode, wantStatus = domain.VerdictAllowed, http.StatusOK
			}
			if v.Code != wantCode || v.HTTPStatus != wantStatus {
				t.Fatalf("code/status = %s/%d, want %s/%d", v.Code, v.HTTPStatus, wantCode, wantStatus)
			}
		})
	}
}

func TestDenyPolicyIsIncidentWithSafeAlternative(t *testing.T) {
	v := engine(t).Check(intern, "db_admin.export_prod_pii", Resource{InternID: intern.ID})
	if !v.Incident || v.SafeAlternative == "" {
		t.Fatalf("запрет выгрузки ПДн должен быть инцидентом с безопасной альтернативой: %+v", v)
	}
}

func TestUnknownRole(t *testing.T) {
	v := engine(t).Check(domain.Principal{ID: "x", Role: "contractor"}, "playbook.search_docs", Resource{})
	if v.Allowed || v.PolicyID != PolicyUnknownRole {
		t.Fatalf("неизвестная роль должна получать отказ: %+v", v)
	}
}

func TestCustomPolicies(t *testing.T) {
	e := New(spec.RBACSpec{
		Roles: []spec.Role{{ID: "intern_backend", Title: "Стажёр"}, {ID: "mentor", Title: "Наставник"}},
		Policies: []spec.Policy{
			{ID: "A-1", Operation: "op.mixed", Effect: "allow", Roles: []string{"*"}},
			{ID: "D-1", Operation: "op.mixed", Effect: "deny", Roles: []string{"intern_backend"}},
			{ID: "A-2", Operation: "op.unknown_constraint", Effect: "allow", Roles: []string{"mentor"}, Constraints: []string{"moon_phase"}},
			{ID: "A-3", Operation: "op.fallback", Effect: "allow", Roles: []string{"mentor"}, Constraints: []string{"self_only"}},
			{ID: "A-4", Operation: "op.fallback", Effect: "allow", Roles: []string{"mentor"}, Description: "Запасное правило без ограничений"},
			{ID: "A-5", Operation: "op.nodesc", Effect: "allow", Roles: []string{"mentor"}},
		},
	})
	// deny-overrides: запрет побеждает разрешение независимо от порядка политик.
	if v := e.Check(intern, "op.mixed", Resource{}); v.Allowed || v.PolicyID != "D-1" || v.Reason == "" || v.Incident {
		t.Fatalf("deny-overrides: %+v", v)
	}
	if v := e.Check(mentor, "op.mixed", Resource{}); !v.Allowed || v.PolicyID != "A-1" {
		t.Fatalf("wildcard allow: %+v", v)
	}
	// Неизвестное ограничение = отказ (fail closed).
	if v := e.Check(mentor, "op.unknown_constraint", Resource{}); v.Allowed || v.Constraint != "moon_phase" {
		t.Fatalf("неизвестное ограничение: %+v", v)
	}
	// Если первое разрешающее правило не прошло по ограничению, проверяется следующее.
	if v := e.Check(mentor, "op.fallback", Resource{InternID: "u-other"}); !v.Allowed || v.PolicyID != "A-4" || v.Reason != "Запасное правило без ограничений" {
		t.Fatalf("перебор разрешающих правил: %+v", v)
	}
	if v := e.Check(mentor, "op.nodesc", Resource{}); !v.Allowed || v.Reason == "" {
		t.Fatalf("причина по умолчанию: %+v", v)
	}
	if !e.Can(domain.RoleMentor, "op.mixed") || e.Can(domain.RoleInternBackend, "op.mixed") || e.Can(domain.RoleMentor, "op.none") {
		t.Fatal("Can работает неверно")
	}
}

func TestMatrixAndRoles(t *testing.T) {
	e := engine(t)
	roles := e.Roles()
	if len(roles) != 4 || roles[0].ID != "intern_backend" {
		t.Fatalf("roles = %+v", roles)
	}
	rows := e.Matrix()
	if len(rows) < 10 {
		t.Fatalf("в матрице слишком мало операций: %d", len(rows))
	}
	byOp := map[string]MatrixRow{}
	for i, r := range rows {
		byOp[r.Operation] = r
		if i > 0 && rows[i-1].Operation > r.Operation {
			t.Fatal("строки матрицы должны быть отсортированы")
		}
	}
	for role, allowed := range byOp["db_admin.export_prod_pii"].Access {
		if allowed {
			t.Errorf("роль %s не должна иметь доступ к выгрузке ПДн", role)
		}
	}
	abs := byOp["absences.create_day_off"].Access
	if !abs["intern_backend"] || abs["mentor"] || abs["team_lead"] || abs["system_cron"] {
		t.Errorf("матрица отгула: %v", abs)
	}
	if e.RoleTitle(domain.RoleMentor) != "Наставник (buddy)" || e.RoleTitle("ghost") != "ghost" {
		t.Error("RoleTitle")
	}
}
