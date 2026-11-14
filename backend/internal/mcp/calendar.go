package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aibuddy/backend/internal/domain"
)

// Имена инструментов календаря и согласования отгула.
const (
	ToolApproveDayOff = "absences.approve_day_off"
	ToolAddTask       = "calendar.add_task"
	ToolCompleteItem  = "calendar.complete_item"
)

// Calendar — имитация корпоративного календаря: встречи, дедлайны, задачи адаптации.
type Calendar struct {
	store         domain.Store
	ids           domain.IDGenerator
	notifications *Notifications
}

// AddTask добавляет задачу: личную (стажёр) или обязательную (наставник, руководитель).
func (s *Calendar) AddTask(ctx context.Context, c Call) (map[string]any, error) {
	internID := argString(c.Args, "intern_id")
	if _, err := loadIntern(ctx, s.store, internID); err != nil {
		return nil, err
	}
	date, clock := argString(c.Args, "date"), argString(c.Args, "time")
	allDay := clock == ""
	if allDay {
		clock = "00:00"
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, c.Now.Location())
	if err != nil {
		return nil, Errorf(CodeInvalidArgs, "некорректная дата %q", date)
	}
	item := domain.PlannerItem{
		ID: s.ids.NewID("plan"), InternID: internID, Kind: domain.PlannerTask,
		Title: strings.TrimSpace(argString(c.Args, "title")), Description: strings.TrimSpace(argString(c.Args, "description")),
		StartsAt: at, AllDay: allDay, Mandatory: c.Principal.Role != domain.RoleInternBackend,
		CreatedBy: c.Principal.ID, CreatedByName: c.Principal.Name, Status: domain.PlannerPlanned, IsSynthetic: true,
	}
	if err := item.Validate(c.Now); err != nil {
		return nil, err
	}
	if err := domain.Save(ctx, s.store, domain.KindPlanner, item.ID, internID, c.Now, item); err != nil {
		return nil, err
	}
	if item.Mandatory {
		body := fmt.Sprintf("%s назначил(а) обязательную задачу «%s» — срок %s", c.Principal.Name, item.Title, at.Format("02.01"))
		if _, err := s.notifications.Send(ctx, internID, "Новая обязательная задача", body, domain.NotificationTaskAssigned, c.Now); err != nil {
			return nil, err
		}
	}
	return toMap(item), nil
}

// CompleteItem отмечает событие выполненным и сохраняет отчёт стажёра.
func (s *Calendar) CompleteItem(ctx context.Context, c Call) (map[string]any, error) {
	internID := argString(c.Args, "intern_id")
	item, err := domain.LoadOne[domain.PlannerItem](ctx, s.store, domain.KindPlanner, argString(c.Args, "item_id"))
	if errors.Is(err, domain.ErrNotFound) || (err == nil && item.InternID != internID) {
		return nil, Errorf(CodeInvalidArgs, "событие не найдено")
	}
	if err != nil {
		return nil, err
	}
	done, err := item.Complete(argString(c.Args, "report"), c.Now)
	if err != nil {
		return nil, err
	}
	if err := domain.Save(ctx, s.store, domain.KindPlanner, done.ID, internID, c.Now, done); err != nil {
		return nil, err
	}
	return toMap(done), nil
}

// ApproveDayOff согласует заявку на отгул и уведомляет сотрудника.
func (s *Absences) ApproveDayOff(ctx context.Context, c Call) (map[string]any, error) {
	number := argString(c.Args, "request_number")
	all, err := domain.Load[domain.AbsenceRequest](ctx, s.store, domain.KindAbsence, "")
	if err != nil {
		return nil, err
	}
	for _, a := range all {
		if a.Number != number {
			continue
		}
		if a.Status == domain.AbsenceStatusApproved {
			return nil, Errorf(CodeInvalidArgs, "заявка %s уже согласована", number)
		}
		a.Status = domain.AbsenceStatusApproved
		if err := domain.Save(ctx, s.store, domain.KindAbsence, a.ID, a.InternID, c.Now, a); err != nil {
			return nil, err
		}
		body := fmt.Sprintf("Руководитель согласовал твой отгул %s на %s", a.Number, humanDate(a.Date))
		if _, err := s.notifications.Send(ctx, a.InternID, "Отгул согласован", body, domain.NotificationAbsenceApproved, c.Now); err != nil {
			return nil, err
		}
		return map[string]any{"request_number": a.Number, "status": a.Status, "intern_id": a.InternID}, nil
	}
	return nil, Errorf(CodeInvalidArgs, "заявка %s не найдена", number)
}
