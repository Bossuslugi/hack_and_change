// Package rbac — чистый движок проверки прав по матрице из specs/03_rbac_matrix.openspec.yaml.
// Модель: default deny, deny-overrides, ABAC-ограничения (self_only, team_scope) поверх ролей.
package rbac

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/spec"
)

// Идентификаторы ограничений и служебных политик.
const (
	ConstraintSelfOnly  = "self_only"
	ConstraintTeamScope = "team_scope"
	PolicyDefaultDeny   = "DEFAULT-DENY"
	PolicyUnknownRole   = "UNKNOWN-ROLE"
	effectAllow         = "allow"
	effectDeny          = "deny"
	wildcard            = "*"
)

// Resource — атрибуты ресурса для ABAC-ограничений.
type Resource struct {
	InternID string // владелец данных, над которыми выполняется операция
	Team     string // команда-владелец ресурса
}

// Engine — движок RBAC.
type Engine struct {
	roles      map[string]spec.Role
	roleOrder  []spec.Role
	policies   map[string][]spec.Policy
	operations []string
}

// New строит движок по спецификации.
func New(s spec.RBACSpec) *Engine {
	e := &Engine{roles: map[string]spec.Role{}, policies: map[string][]spec.Policy{}}
	for _, r := range s.Roles {
		e.roles[r.ID] = r
		e.roleOrder = append(e.roleOrder, r)
	}
	for _, p := range s.Policies {
		if _, seen := e.policies[p.Operation]; !seen {
			e.operations = append(e.operations, p.Operation)
		}
		e.policies[p.Operation] = append(e.policies[p.Operation], p)
	}
	return e
}

func appliesTo(p spec.Policy, role domain.Role) bool {
	for _, r := range p.Roles {
		if r == wildcard || r == string(role) {
			return true
		}
	}
	return false
}

func forbidden(p domain.Principal, op, policy, reason string) domain.RBACVerdict {
	return domain.RBACVerdict{
		Allowed: false, Code: domain.VerdictForbidden, HTTPStatus: http.StatusForbidden,
		Role: p.Role, Operation: op, PolicyID: policy, Reason: reason,
	}
}

// Check проверяет, может ли субъект выполнить операцию над ресурсом.
func (e *Engine) Check(p domain.Principal, operation string, res Resource) domain.RBACVerdict {
	role, known := e.roles[string(p.Role)]
	if !known {
		return forbidden(p, operation, PolicyUnknownRole, fmt.Sprintf("Роль %q не входит в ролевую модель", p.Role))
	}

	var allows []spec.Policy
	for _, pol := range e.policies[operation] {
		if !appliesTo(pol, p.Role) {
			continue
		}
		if pol.Effect == effectDeny {
			reason := pol.Reason
			if reason == "" {
				reason = fmt.Sprintf("Операция %s запрещена политикой %s", operation, pol.ID)
			}
			v := forbidden(p, operation, pol.ID, reason)
			v.Incident = pol.Incident
			v.SafeAlternative = pol.SafeAlternative
			return v
		}
		allows = append(allows, pol)
	}

	if len(allows) == 0 {
		return forbidden(p, operation, PolicyDefaultDeny,
			fmt.Sprintf("Для роли «%s» операция %s не разрешена (default deny)", role.Title, operation))
	}

	var failed domain.RBACVerdict
	for _, pol := range allows {
		constraint, reason := firstFailedConstraint(pol.Constraints, p, res)
		if constraint == "" {
			return domain.RBACVerdict{
				Allowed: true, Code: domain.VerdictAllowed, HTTPStatus: http.StatusOK,
				Role: p.Role, Operation: operation, PolicyID: pol.ID,
				Reason: allowReason(pol, role),
			}
		}
		failed = forbidden(p, operation, pol.ID, reason)
		failed.Constraint = constraint
	}
	return failed
}

func allowReason(pol spec.Policy, role spec.Role) string {
	if pol.Description != "" {
		return pol.Description
	}
	return fmt.Sprintf("Разрешено политикой %s для роли «%s»", pol.ID, role.Title)
}

func firstFailedConstraint(constraints []string, p domain.Principal, res Resource) (string, string) {
	for _, c := range constraints {
		switch c {
		case ConstraintSelfOnly:
			if res.InternID == "" || res.InternID != p.ID {
				return c, "Операцию можно выполнять только над собственными данными (self_only)"
			}
		case ConstraintTeamScope:
			if res.Team == "" || !strings.EqualFold(res.Team, p.Team) {
				return c, fmt.Sprintf("Ресурс не принадлежит команде %s (team_scope)", p.Team)
			}
		default:
			return c, fmt.Sprintf("Неизвестное ограничение %q — доступ запрещён", c)
		}
	}
	return "", ""
}

// Can сообщает, разрешена ли операция роли на уровне матрицы (без учёта ABAC-ограничений).
func (e *Engine) Can(role domain.Role, operation string) bool {
	allowed := false
	for _, pol := range e.policies[operation] {
		if !appliesTo(pol, role) {
			continue
		}
		if pol.Effect == effectDeny {
			return false
		}
		allowed = true
	}
	return allowed
}

// MatrixRow — строка матрицы «операция × роль» для отображения.
type MatrixRow struct {
	Operation string          `json:"operation"`
	Access    map[string]bool `json:"access"`
}

// Matrix возвращает развёрнутую матрицу прав для ролей-людей и сервисных аккаунтов.
func (e *Engine) Matrix() []MatrixRow {
	ops := append([]string{}, e.operations...)
	sort.Strings(ops)
	rows := make([]MatrixRow, 0, len(ops))
	for _, op := range ops {
		row := MatrixRow{Operation: op, Access: map[string]bool{}}
		for _, r := range e.roleOrder {
			row.Access[r.ID] = e.Can(domain.Role(r.ID), op)
		}
		rows = append(rows, row)
	}
	return rows
}

// Roles возвращает роли в порядке спецификации.
func (e *Engine) Roles() []spec.Role { return append([]spec.Role{}, e.roleOrder...) }

// RoleTitle возвращает человекочитаемое название роли.
func (e *Engine) RoleTitle(role domain.Role) string {
	if r, ok := e.roles[string(role)]; ok {
		return r.Title
	}
	return string(role)
}
