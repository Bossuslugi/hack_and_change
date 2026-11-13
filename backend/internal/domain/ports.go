package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// Kind — вид записи хранилища. Каждому виду соответствует таблица PostgreSQL.
type Kind string

// Виды записей.
const (
	KindIntern       Kind = "interns"
	KindChecklist    Kind = "checklist_items"
	KindSprintTask   Kind = "sprint_tasks"
	KindMessage      Kind = "chat_messages"
	KindToolCall     Kind = "mcp_tool_calls"
	KindAbsence      Kind = "absence_requests"
	KindAccessGrant  Kind = "access_grants"
	KindNotification Kind = "notifications"
	KindBrief        Kind = "mentor_briefs"
	KindEvent        Kind = "telemetry_events"
	KindDocRead      Kind = "doc_progress"
	KindOpenQuestion Kind = "open_questions"
)

// AllKinds — все виды записей в порядке создания таблиц.
var AllKinds = []Kind{
	KindIntern, KindChecklist, KindSprintTask, KindMessage, KindToolCall, KindAbsence,
	KindAccessGrant, KindNotification, KindBrief, KindEvent, KindDocRead, KindOpenQuestion,
}

// Record — запись хранилища: индексируемые поля + JSON-документ.
type Record struct {
	Kind      Kind
	ID        string
	OwnerID   string
	CreatedAt time.Time
	Payload   json.RawMessage
}

// ErrNotFound — запись не найдена.
var ErrNotFound = errors.New("запись не найдена")

// Store — порт хранилища состояния (PostgreSQL или in-memory fallback).
type Store interface {
	// Put создаёт или обновляет запись (upsert по Kind+ID), сохраняя исходный порядок.
	Put(ctx context.Context, rec Record) error
	// Get возвращает запись по Kind+ID или ErrNotFound.
	Get(ctx context.Context, kind Kind, id string) (Record, error)
	// List возвращает записи владельца в порядке вставки. Пустой ownerID — все записи вида.
	List(ctx context.Context, kind Kind, ownerID string) ([]Record, error)
	// Reset атомарно очищает хранилище и загружает seed-записи.
	Reset(ctx context.Context, seed []Record) error
	// Ping проверяет доступность.
	Ping(ctx context.Context) error
	// Name — имя реализации для диагностики (postgres | memory).
	Name() string
	Close() error
}

// Cache — порт кэша (Redis или in-memory fallback).
type Cache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Ping(ctx context.Context) error
	Name() string
	Close() error
}

// Clock — источник времени (подменяется в тестах).
type Clock interface{ Now() time.Time }

// SystemClock — системные часы.
type SystemClock struct{}

// Now возвращает текущее время.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock — фиксированные часы для тестов.
type FixedClock struct{ T time.Time }

// Now возвращает зафиксированное время.
func (c FixedClock) Now() time.Time { return c.T }

// IDGenerator — генератор идентификаторов.
type IDGenerator interface{ NewID(prefix string) string }

// RandomIDs — случайные идентификаторы вида prefix-1a2b3c4d5e6f.
type RandomIDs struct{}

// NewID возвращает случайный идентификатор.
func (RandomIDs) NewID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

// SequentialIDs — детерминированные идентификаторы для тестов (prefix-1, prefix-2…).
type SequentialIDs struct{ n atomic.Int64 }

// NewID возвращает следующий идентификатор последовательности.
func (s *SequentialIDs) NewID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, s.n.Add(1))
}

// NewRecord сериализует значение в запись хранилища.
func NewRecord(kind Kind, id, ownerID string, at time.Time, v any) (Record, error) {
	if id == "" {
		return Record{}, fmt.Errorf("%w: пустой идентификатор записи %s", ErrValidation, kind)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return Record{}, fmt.Errorf("сериализация %s/%s: %w", kind, id, err)
	}
	return Record{Kind: kind, ID: id, OwnerID: ownerID, CreatedAt: at, Payload: raw}, nil
}

// Save сохраняет типизированное значение.
func Save(ctx context.Context, s Store, kind Kind, id, ownerID string, at time.Time, v any) error {
	rec, err := NewRecord(kind, id, ownerID, at, v)
	if err != nil {
		return err
	}
	return s.Put(ctx, rec)
}

// Load читает типизированные значения владельца.
func Load[T any](ctx context.Context, s Store, kind Kind, ownerID string) ([]T, error) {
	recs, err := s.List(ctx, kind, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(recs))
	for _, r := range recs {
		var v T
		if err := json.Unmarshal(r.Payload, &v); err != nil {
			return nil, fmt.Errorf("десериализация %s/%s: %w", kind, r.ID, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// LoadOne читает одно типизированное значение по идентификатору.
func LoadOne[T any](ctx context.Context, s Store, kind Kind, id string) (T, error) {
	var v T
	rec, err := s.Get(ctx, kind, id)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(rec.Payload, &v); err != nil {
		return v, fmt.Errorf("десериализация %s/%s: %w", kind, id, err)
	}
	return v, nil
}

// ChecklistRecordID — идентификатор записи шага чек-листа.
func ChecklistRecordID(internID, itemID string) string { return internID + ":" + itemID }

// LoadChecklist читает чек-лист стажёра, отсортированный по позиции.
func LoadChecklist(ctx context.Context, s Store, internID string) ([]ChecklistItem, error) {
	items, err := Load[ChecklistItem](ctx, s, KindChecklist, internID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Position < items[j].Position })
	return items, nil
}

// SaveChecklistItem сохраняет шаг чек-листа.
func SaveChecklistItem(ctx context.Context, s Store, internID string, item ChecklistItem) error {
	return Save(ctx, s, KindChecklist, ChecklistRecordID(internID, item.ID), internID, item.UpdatedAt, item)
}

// SeedData — синтетические демо-данные.
type SeedData struct {
	Employees []Employee
	Intern    InternProfile
	Checklist []ChecklistItem
	Task      SprintTask
	Docs      []KnowledgeDoc
	Flags     []FeatureFlag
	// Repositories — каталог репозиториев: имя → команда-владелец.
	Repositories map[string]string
}

// Records превращает seed в записи хранилища.
func (d SeedData) Records(now time.Time) ([]Record, error) {
	var out []Record
	rec, err := NewRecord(KindIntern, d.Intern.ID, d.Intern.ID, now, d.Intern)
	if err != nil {
		return nil, err
	}
	out = append(out, rec)
	for _, it := range d.Checklist {
		it.UpdatedAt = now
		rec, err := NewRecord(KindChecklist, ChecklistRecordID(d.Intern.ID, it.ID), d.Intern.ID, now, it)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	rec, err = NewRecord(KindSprintTask, d.Intern.ID, d.Intern.ID, now, d.Task)
	if err != nil {
		return nil, err
	}
	return append(out, rec), nil
}

// Directory — имитация корпоративного справочника сотрудников и авторизации.
type Directory struct {
	byID    map[string]Employee
	byToken map[string]Employee
	order   []Employee
}

// NewDirectory строит справочник.
func NewDirectory(employees []Employee) *Directory {
	d := &Directory{byID: map[string]Employee{}, byToken: map[string]Employee{}}
	for _, e := range employees {
		d.byID[e.ID] = e
		if e.Token != "" {
			d.byToken[e.Token] = e
		}
		d.order = append(d.order, e)
	}
	return d
}

// ByToken находит сотрудника по демо-токену.
func (d *Directory) ByToken(token string) (Employee, bool) {
	e, ok := d.byToken[token]
	return e, ok
}

// ByID находит сотрудника по идентификатору.
func (d *Directory) ByID(id string) (Employee, bool) {
	e, ok := d.byID[id]
	return e, ok
}

// ByRole возвращает первого сотрудника с ролью.
func (d *Directory) ByRole(role Role) (Employee, bool) {
	for _, e := range d.order {
		if e.Role == role {
			return e, true
		}
	}
	return Employee{}, false
}
