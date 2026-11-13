package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"aibuddy/backend/internal/cache"
	"aibuddy/backend/internal/domain"
	"aibuddy/backend/internal/rbac"
	"aibuddy/backend/internal/seed"
	"aibuddy/backend/internal/spec"
	"aibuddy/backend/internal/storage/memory"
)

// Четверг, 12 ноября 2026: «в пятницу» = 2026-11-13.
var testNow = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

var errStore = errors.New("хранилище недоступно")

// faultyStore — in-memory хранилище с управляемыми сбоями.
type faultyStore struct {
	*memory.Store
	failPut  map[domain.Kind]bool
	failList map[domain.Kind]bool
	failGet  map[domain.Kind]bool
}

func (s *faultyStore) Put(ctx context.Context, rec domain.Record) error {
	if s.failPut[rec.Kind] {
		return errStore
	}
	return s.Store.Put(ctx, rec)
}

func (s *faultyStore) List(ctx context.Context, kind domain.Kind, owner string) ([]domain.Record, error) {
	if s.failList[kind] {
		return nil, errStore
	}
	return s.Store.List(ctx, kind, owner)
}

func (s *faultyStore) Get(ctx context.Context, kind domain.Kind, id string) (domain.Record, error) {
	if s.failGet[kind] {
		return domain.Record{}, errStore
	}
	return s.Store.Get(ctx, kind, id)
}

type fixture struct {
	store    *faultyStore
	cache    *cache.Memory
	reg      *Registry
	services *Services
	gw       *Gateway
	bundle   *spec.Bundle
	seed     domain.SeedData
	dir      *domain.Directory
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	bundle := spec.MustLoad()
	data := seed.Data()
	store := &faultyStore{
		Store:   memory.New(),
		failPut: map[domain.Kind]bool{}, failList: map[domain.Kind]bool{}, failGet: map[domain.Kind]bool{},
	}
	recs, err := data.Records(testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	clock, ids, kv := domain.FixedClock{T: testNow}, &domain.SequentialIDs{}, cache.NewMemory()
	reg := NewRegistry(bundle.Tools)
	services := NewServices(Deps{Store: store, Cache: kv, Clock: clock, IDs: ids, Seed: data})
	if err := services.BindAll(reg); err != nil {
		t.Fatal(err)
	}
	gw := NewGateway(reg, rbac.New(bundle.RBAC), NewTelemetry(store, clock, ids), store, clock, ids)
	return &fixture{store: store, cache: kv, reg: reg, services: services, gw: gw, bundle: bundle, seed: data, dir: domain.NewDirectory(data.Employees)}
}

func (f *fixture) principal(t *testing.T, id string) domain.Principal {
	t.Helper()
	e, ok := f.dir.ByID(id)
	if !ok {
		t.Fatalf("нет сотрудника %s", id)
	}
	return e.Principal()
}

func (f *fixture) intern(t *testing.T) domain.Principal { return f.principal(t, seed.InternID) }
func (f *fixture) mentor(t *testing.T) domain.Principal { return f.principal(t, seed.MentorID) }
func (f *fixture) cron(t *testing.T) domain.Principal   { return f.principal(t, seed.CronID) }

func (f *fixture) exec(p domain.Principal, tool string, args map[string]any) domain.MCPToolCall {
	return f.gw.Execute(context.Background(), p, seed.InternID, tool, args)
}

func (f *fixture) events(t *testing.T, name string) []domain.TelemetryEvent {
	t.Helper()
	all, err := domain.Load[domain.TelemetryEvent](context.Background(), f.store.Store, domain.KindEvent, seed.InternID)
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.TelemetryEvent
	for _, e := range all {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

func (f *fixture) notifications(t *testing.T, recipient string) []domain.Notification {
	t.Helper()
	list, err := domain.Load[domain.Notification](context.Background(), f.store.Store, domain.KindNotification, recipient)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// failCache — кэш, который всегда отвечает ошибкой.
type failCache struct{}

func (failCache) Get(context.Context, string) (string, bool, error)        { return "", false, errStore }
func (failCache) Set(context.Context, string, string, time.Duration) error { return errStore }
func (failCache) Del(context.Context, ...string) error                     { return errStore }
func (failCache) Ping(context.Context) error                               { return errStore }
func (failCache) Name() string                                             { return "fail" }
func (failCache) Close() error                                             { return nil }
